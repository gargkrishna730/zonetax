// Package collector wires together scrape.DiscoverAgents, scrape.ScrapeAll, and
// costengine.Compute into a periodic collection loop, holding the latest costed Summary in
// memory for the REST API (internal/api) to serve. Kept deliberately simple (single latest
// snapshot, no history/TSDB) — see ROADMAP.md M2 non-goals.
package collector

import (
	"context"
	"errors"
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
	// HistoryFile, if set, is where History is saved (every SaveInterval and on shutdown)
	// and loaded from on startup, so history survives collector restarts.
	HistoryFile  string
	SaveInterval time.Duration
}

// Store holds the most recently computed cost Summary, safe for concurrent read (API handlers)
// and write (the collection loop). It also accumulates an hourly-bucketed cost/traffic History
// (see history.go) for as long as this process has been running — NOT a persistent time-series
// database, and it makes no claim to data from before the collector started. StartedAt and
// History together let the API/UI state honestly how much real history exists rather than
// implying a longer window than was actually observed.
type Store struct {
	mu      sync.RWMutex
	latest  costengine.Summary
	updated time.Time
	// lastErr surfaces the most recent collection-cycle-level error (e.g. agent discovery
	// failure) so /healthz and the API can report staleness without crashing the process — a
	// transient k8s API hiccup shouldn't take the collector down.
	lastErr        error
	startedAt      time.Time
	scrapeInterval time.Duration
	history        *History
	acc            *accumulator
}

// maxHistoryHours bounds History's memory to 7 days of hourly buckets — the longest range this
// dashboard's UI offers (see internal/api's supported ?range values). Buckets older than this
// are evicted, not silently kept forever.
const maxHistoryHours = 7 * 24

// Latest returns the most recent Summary, when it was computed, and any error from the most
// recent collection attempt (which may be nil even if Summary is stale-but-valid from an
// earlier successful cycle).
func (s *Store) Latest() (costengine.Summary, time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest, s.updated, s.lastErr
}

// StartedAt returns when this Store recorded its first successful collection cycle — i.e. since
// when both the "tracked" totals (Latest()'s Summary) and History have been accumulating.
// Returns the zero time.Time if no successful cycle has completed yet. This is the honest
// answer to "since when is this data real" — deliberately NOT wall-clock process start, since a
// process can be up but never successfully complete a scrape (e.g. agent discovery failing).
func (s *Store) StartedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.startedAt
}

// ScrapeInterval returns the collection loop's configured interval, set once by Run(). Zero
// until Run() has started. Exposed via the API so the UI can state its actual data-freshness
// cadence instead of a hardcoded guess.
func (s *Store) ScrapeInterval() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scrapeInterval
}

// History returns the Store's hourly time-bucketed cost/traffic history. Never nil — lazily
// created on first access so a zero-value Store (as used directly in tests, e.g.
// internal/api's api_test.go) works without an explicit constructor.
func (s *Store) History() *History {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureHistoryLocked()
}

func (s *Store) ensureHistoryLocked() *History {
	if s.history == nil {
		s.history = NewHistory(maxHistoryHours)
	}
	return s.history
}

func (s *Store) setScrapeInterval(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scrapeInterval = d
}

func (s *Store) set(summary costengine.Summary, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = err
	if err == nil {
		now := time.Now()
		if s.startedAt.IsZero() {
			s.startedAt = now
		}
		s.latest = summary
		s.updated = now
	}
}

// observe folds each successfully scraped agent into the observed-traffic accumulator and
// records the result in History. Failed agents are simply absent from perAgent.
func (s *Store) observe(perAgent map[string]costengine.Summary, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.acc == nil {
		s.acc = newAccumulator()
	}
	for name, sum := range perAgent {
		s.acc.observe(name, sum, now)
	}
	s.acc.prune(now)
	cost, gb, same, entries := s.acc.totals()
	s.ensureHistoryLocked().Record(now, cost, gb, same, entries)
}

// loadHistory restores saved history from path and continues the observed totals from its
// last snapshot. Downtime since that snapshot is marked as a gap (shown as no data).
func (s *Store) loadHistory(path string) error {
	h := s.History()
	if err := h.Load(path, time.Now()); err != nil {
		return err
	}
	h.MarkGap()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acc = newAccumulator()
	if last, ok := h.last(); ok {
		s.acc.seed(last)
	}
	return nil
}

// Run starts the periodic collection loop, blocking until ctx is cancelled. Each cycle:
// discover agent pods -> scrape /metrics from each -> merge -> price -> store. A single cycle's
// failure (or a single unreachable agent) logs and is retried next tick rather than crashing.
func Run(ctx context.Context, clientset kubernetes.Interface, table *pricing.Table, cfg Config, store *Store) {
	store.setScrapeInterval(cfg.ScrapeInterval)

	var saveC <-chan time.Time
	if cfg.HistoryFile != "" {
		switch err := store.loadHistory(cfg.HistoryFile); {
		case err == nil:
			log.Printf("collector: restored history from %s (since %s)", cfg.HistoryFile, store.History().EarliestSnapshot().UTC().Format(time.RFC3339))
		case errors.Is(err, ErrNoHistoryFile):
			log.Printf("collector: no saved history at %s, starting fresh", cfg.HistoryFile)
		default:
			log.Printf("collector: could not load history (%v), starting fresh", err)
		}
		every := cfg.SaveInterval
		if every <= 0 {
			every = 5 * time.Minute
		}
		t := time.NewTicker(every)
		defer t.Stop()
		saveC = t.C
		defer saveHistory(store, cfg.HistoryFile)
	}

	ticker := time.NewTicker(cfg.ScrapeInterval)
	defer ticker.Stop()

	collectOnce(ctx, clientset, table, cfg, store)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectOnce(ctx, clientset, table, cfg, store)
		case <-saveC:
			saveHistory(store, cfg.HistoryFile)
		}
	}
}

func saveHistory(store *Store, path string) {
	if err := store.History().Save(path); err != nil {
		log.Printf("collector: save history to %s: %v", path, err)
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

	perAgent := make(map[string]costengine.Summary, len(results))
	for _, r := range results {
		if r.Err != nil {
			continue
		}
		if s, err := costengine.Compute(r.Metrics, table, cfg.Cloud, cfg.Region); err == nil {
			perAgent[r.Target.PodName] = s
		}
	}
	store.observe(perAgent, time.Now())

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
