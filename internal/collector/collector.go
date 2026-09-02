// Package collector wires together scrape.DiscoverAgents, scrape.ScrapeAll, and
// costengine.Compute into a periodic collection loop, holding the latest costed Summary in
// memory for the REST API (internal/api) to serve. Kept deliberately simple (single latest
// snapshot, no history/TSDB) — see ROADMAP.md M2 non-goals.
package collector

import (
	"context"
	"log"
	"sync"
	"time"

	dto "github.com/prometheus/client_model/go"
	"k8s.io/client-go/kubernetes"

	"github.com/gargkrishna730/zonetax/internal/costengine"
	"github.com/gargkrishna730/zonetax/internal/pricing"
	"github.com/gargkrishna730/zonetax/internal/scrape"
)

// Config controls how the collector discovers and scrapes agents and prices their traffic.
type Config struct {
	Namespace      string
	LabelSelector  string
	ScrapeInterval time.Duration
	ScrapeTimeout  time.Duration
	Cloud          string
	Region         string
}

// Store holds the most recently computed cost Summary, safe for concurrent read (API handlers)
// and write (the collection loop).
type Store struct {
	mu      sync.RWMutex
	latest  costengine.Summary
	updated time.Time
	// lastErr surfaces the most recent collection-cycle-level error (e.g. agent discovery
	// failure) so /healthz and the API can report staleness without crashing the process — a
	// transient k8s API hiccup shouldn't take the collector down.
	lastErr error
}

// Latest returns the most recent Summary, when it was computed, and any error from the most
// recent collection attempt (which may be nil even if Summary is stale-but-valid from an
// earlier successful cycle).
func (s *Store) Latest() (costengine.Summary, time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest, s.updated, s.lastErr
}

func (s *Store) set(summary costengine.Summary, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = err
	if err == nil {
		s.latest = summary
		s.updated = time.Now()
	}
}

// Run starts the periodic collection loop, blocking until ctx is cancelled. Each cycle:
// discover agent pods -> scrape /metrics from each -> merge -> price -> store. A single cycle's
// failure (or a single unreachable agent) logs and is retried next tick rather than crashing.
func Run(ctx context.Context, clientset kubernetes.Interface, table *pricing.Table, cfg Config, store *Store) {
	ticker := time.NewTicker(cfg.ScrapeInterval)
	defer ticker.Stop()

	collectOnce(ctx, clientset, table, cfg, store)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectOnce(ctx, clientset, table, cfg, store)
		}
	}
}

func collectOnce(ctx context.Context, clientset kubernetes.Interface, table *pricing.Table, cfg Config, store *Store) {
	targets, err := scrape.DiscoverAgents(ctx, clientset, cfg.Namespace, cfg.LabelSelector)
	if err != nil {
		log.Printf("collector: discover agents: %v", err)
		store.set(costengine.Summary{}, err)
		return
	}
	if len(targets) == 0 {
		log.Printf("collector: no agent pods found (namespace=%s selector=%s)", cfg.Namespace, cfg.LabelSelector)
	}

	results := scrape.ScrapeAll(ctx, targets, cfg.ScrapeTimeout)
	merged := mergeScrapeResults(results)

	summary, err := costengine.Compute(merged, table, cfg.Cloud, cfg.Region)
	if err != nil {
		log.Printf("collector: compute cost: %v", err)
		store.set(costengine.Summary{}, err)
		return
	}

	store.set(summary, nil)
	log.Printf("collector: cycle complete: %d agents, %d cross-AZ entries, $%.4f total",
		len(targets), len(summary.Entries), summary.TotalCrossAZCost)
}

// mergeScrapeResults logs (without failing the cycle) any per-agent scrape errors and merges
// the successfully-scraped agents' metric families via costengine.MergeFamilies.
func mergeScrapeResults(results []scrape.Result) map[string]*dto.MetricFamily {
	var ok []map[string]*dto.MetricFamily
	for _, r := range results {
		if r.Err != nil {
			log.Printf("collector: scrape %s (%s) failed: %v", r.Target.PodName, r.Target.NodeName, r.Err)
			continue
		}
		ok = append(ok, r.Metrics)
	}
	return costengine.MergeFamilies(ok)
}
