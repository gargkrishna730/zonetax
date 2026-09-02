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
	"github.com/gargkrishna730/zonetax/internal/collector"
	"github.com/gargkrishna730/zonetax/internal/pricing"
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
		Cloud:          envOr("CLOUD_PROVIDER", "aws"),
		Region:         envOr("CLOUD_REGION", "us-east-1"),
	}

	clientset, err := inClusterClientset()
	if err != nil {
		log.Fatalf("build k8s client: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	store := &collector.Store{}
	go collector.Run(ctx, clientset, table, cfg, store)

	handler := api.NewHandler(store)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/api/v1/costs", handler.Costs)
	mux.HandleFunc("/api/v1/top", handler.Top)

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
