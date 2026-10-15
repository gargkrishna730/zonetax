// Command collector aggregates cross-AZ traffic metrics scraped from agent DaemonSet pods,
// applies the pricing table, and serves the REST API.
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

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/gargkrishna730/zonetax/internal/api"
	"github.com/gargkrishna730/zonetax/internal/clouddetect"
	"github.com/gargkrishna730/zonetax/internal/collector"
	"github.com/gargkrishna730/zonetax/internal/pricing"
	"github.com/gargkrishna730/zonetax/ui"
)

const (
	defaultScrapeInterval = 30 * time.Second
	defaultScrapeTimeout  = 5 * time.Second
	defaultLabelSelector  = "app.kubernetes.io/component=agent"
)

func main() {
	table, err := pricing.LoadDefault()
	if err != nil {
		log.Fatalf("load pricing table: %v", err)
	}
	log.Printf("loaded pricing table v%d (%d cloud providers)", table.Version, len(table.Pricing))

	cfg := collector.Config{
		Namespace:      envOr("AGENT_NAMESPACE", currentNamespace()),
		LabelSelector:  envOr("AGENT_LABEL_SELECTOR", defaultLabelSelector),
		ScrapeInterval: envDurationOr("SCRAPE_INTERVAL_SECONDS", defaultScrapeInterval),
		ScrapeTimeout:  envDurationOr("SCRAPE_TIMEOUT_SECONDS", defaultScrapeTimeout),
		HistoryFile:    os.Getenv("HISTORY_FILE"),
	}

	clientset, err := inClusterClientset()
	if err != nil {
		log.Fatalf("build k8s client: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Resolve which cloud/region this cluster actually runs in. Previously these defaulted to
	// a hard-coded cloud and region, so an install anywhere else silently priced and displayed
	// the wrong region. Auto-detect from node labels/providerID, and let an explicit env var
	// win so operators can still override (e.g. an unusual or unlabelled cluster).
	cfg.Cloud, cfg.Region = resolveCloudRegion(ctx, clientset)

	store := &collector.Store{}
	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		collector.Run(ctx, clientset, table, cfg, store)
	}()

	handler := api.NewHandler(store)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/api/v1/costs", handler.Costs)
	mux.HandleFunc("/api/v1/top", handler.Top)
	mux.HandleFunc("/api/v1/history", handler.History)
	mux.HandleFunc("/api/v1/map", handler.Map)
	mux.Handle("/", http.FileServer(http.FS(ui.FS())))

	addr := ":8080"
	srv := &http.Server{Addr: addr, Handler: mux}
	log.Printf("zonetax-collector starting on %s (namespace=%s selector=%q cloud=%s region=%s interval=%s)",
		addr, cfg.Namespace, cfg.LabelSelector, cfg.Cloud, cfg.Region, cfg.ScrapeInterval)

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
	// Let Run finish its final history save before the process exits.
	select {
	case <-runDone:
	case <-time.After(10 * time.Second):
		log.Println("timed out waiting for final history save")
	}
}

func inClusterClientset() (*kubernetes.Clientset, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

// currentNamespace reads the namespace this pod is running in from the standard
// service-account-token projection, falling back to "zonetax" (the Helm chart's default
// install namespace) when running outside a cluster or the file is unreadable.
func currentNamespace() string {
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "zonetax"
	}
	return string(data)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// resolveCloudRegion decides which cloud/region to price against, in priority order:
//
//  1. Explicit CLOUD_PROVIDER / CLOUD_REGION env vars (an operator override always wins).
//  2. Auto-detection from node labels and spec.providerID.
//  3. A last-resort cloud default, with the region left empty.
//
// Step 3 deliberately leaves the region blank rather than guessing one: the pricing table
// falls back to the cloud-level default rate, and an empty region shows up in the API/UI as
// "unknown" instead of asserting a region the cluster may not be in. Silently claiming the
// wrong region is worse than admitting we don't know it.
func resolveCloudRegion(ctx context.Context, clientset kubernetes.Interface) (cloud, region string) {
	envCloud := os.Getenv("CLOUD_PROVIDER")
	envRegion := os.Getenv("CLOUD_REGION")

	detected, err := clouddetect.Detect(ctx, clientset)
	if err != nil {
		log.Printf("cloud/region auto-detection failed: %v", err)
	} else {
		log.Printf("detected cloud=%q region=%q across %d node(s), zones=%v",
			detected.Cloud, detected.Region, detected.NodesSeen, detected.Zones)
		if len(detected.Zones) == 1 {
			log.Printf("note: all nodes are in a single zone (%s); a single-zone cluster "+
				"cannot incur cross-AZ transfer cost", detected.Zones[0])
		}
	}

	cloud = firstNonEmpty(envCloud, detected.Cloud, "aws")
	region = firstNonEmpty(envRegion, detected.Region)

	switch {
	case envRegion != "" && detected.Region != "" && envRegion != detected.Region:
		log.Printf("WARNING: CLOUD_REGION=%q overrides detected region %q — cross-AZ traffic "+
			"will be priced using %q", envRegion, detected.Region, envRegion)
	case region == "":
		log.Printf("WARNING: region unknown; using the %s cloud-level default rate and "+
			"reporting the region as unknown. Set CLOUD_REGION to price a specific region.", cloud)
	}

	log.Printf("pricing cross-AZ traffic as cloud=%q region=%q", cloud, region)
	return cloud, region
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func envDurationOr(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs <= 0 {
		log.Printf("warning: ignoring invalid %s=%q", key, v)
		return fallback
	}
	return time.Duration(secs) * time.Second
}
