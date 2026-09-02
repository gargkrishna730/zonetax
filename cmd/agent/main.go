// Command agent runs as a Kubernetes DaemonSet, sampling conntrack flows on its node and
// exposing cross-AZ / same-AZ byte-count metrics for the collector to scrape. It intentionally
// does no cost math — see internal/pricing and cmd/collector for that.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/gargkrishna730/zonetax/internal/aggregator"
	"github.com/gargkrishna730/zonetax/internal/conntrack"
	"github.com/gargkrishna730/zonetax/internal/metrics"
	"github.com/gargkrishna730/zonetax/internal/podindex"
)

const (
	// conntrackPath is where the Linux kernel exposes the connection tracking table. The
	// container must mount the host's /proc (or run with hostNetwork + hostPID as appropriate)
	// for this path to reflect the node's real conntrack state — see the Helm chart.
	conntrackPath = "/proc/net/nf_conntrack"

	defaultSampleInterval = 15 * time.Second
)

func main() {
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		log.Println("warning: NODE_NAME env var not set; run via the Helm chart's DaemonSet template")
	}

	sampleInterval := defaultSampleInterval
	if v := os.Getenv("SAMPLE_INTERVAL_SECONDS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			sampleInterval = time.Duration(secs) * time.Second
		} else {
			log.Printf("warning: ignoring invalid SAMPLE_INTERVAL_SECONDS=%q", v)
		}
	}

	reg := prometheus.NewRegistry()
	metrics.MustRegister(reg)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	store, err := buildPodIndex(ctx)
	if err != nil {
		// A missing/unsynced pod index means every flow is unattributable. Fail loudly rather
		// than silently emitting empty metrics that would look like "zero cross-AZ traffic".
		log.Fatalf("failed to build pod/node index: %v", err)
	}

	go runSampleLoop(ctx, store, sampleInterval)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	addr := ":9090"
	srv := &http.Server{Addr: addr, Handler: mux}
	log.Printf("zonetax-agent starting on %s (node=%s, sample_interval=%s)", addr, nodeName, sampleInterval)

	go func() {
		<-ctx.Done()
		log.Println("shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// buildPodIndex constructs the in-cluster pod/node index via the in-cluster kubeconfig. Kept as
// its own function so main stays readable and so a future local-dev mode (out-of-cluster
// kubeconfig) has an obvious single place to branch.
func buildPodIndex(ctx context.Context) (*podindex.Store, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	// 10 minute informer resync as a safety net against missed watch events; event handlers
	// keep the store current in near-real-time in the common case.
	return podindex.NewInformerStore(ctx, clientset, 10*time.Minute)
}

// runSampleLoop periodically reads conntrack, aggregates flows by AZ pair, and records results
// into the Prometheus metrics. Runs until ctx is cancelled.
func runSampleLoop(ctx context.Context, store *podindex.Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := sampleOnce(store); err != nil {
				log.Printf("sample error: %v", err)
			}
		}
	}
}

func sampleOnce(store *podindex.Store) error {
	timer := prometheus.NewTimer(metrics.SampleDurationSeconds)
	defer timer.ObserveDuration()

	f, err := os.Open(conntrackPath)
	if err != nil {
		return err
	}
	defer f.Close()

	flows, err := conntrack.ParseReader(f)
	if err != nil {
		return err
	}

	out := aggregator.Aggregate(flows, store.Lookup)
	if out.Unresolved > 0 {
		metrics.UnresolvedFlowsTotal.Add(float64(out.Unresolved))
	}

	for _, r := range out.Results {
		if r.CrossAZ() {
			metrics.CrossAZBytesTotal.WithLabelValues(r.SrcZone, r.DstZone, r.SrcNamespace, r.SrcWorkload).Add(float64(r.Bytes))
		} else {
			metrics.SameAZBytesTotal.WithLabelValues(r.SrcZone, r.SrcNamespace, r.SrcWorkload).Add(float64(r.Bytes))
		}
	}
	return nil
}
