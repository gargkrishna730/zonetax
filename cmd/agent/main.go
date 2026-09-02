// Command agent runs as a Kubernetes DaemonSet, sampling conntrack flows on its node and
// exposing raw cross-AZ byte-count metrics for the collector to scrape. It intentionally does
// no cost math — see internal/pricing and cmd/collector for that.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gargkrishna730/zonetax/internal/azmap"
)

func main() {
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		log.Println("warning: NODE_NAME env var not set; run via the Helm chart's DaemonSet template")
	}

	// TODO(M1): wire up client-go informer to resolve pod IP -> pod -> node -> AZ, and start
	// sampling /proc/net/nf_conntrack on an interval. Placeholder resolver keeps `go build`
	// green and gives a concrete extension point.
	_ = azmap.NewStaticResolver(nil)

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		// TODO(M1): replace with real Prometheus metrics (promhttp.Handler()).
		fmt.Fprintln(w, "# zonetax_agent metrics not yet implemented (see ROADMAP.md M1)")
	})

	addr := ":9090"
	log.Printf("zonetax-agent starting on %s (node=%s)", addr, nodeName)
	log.Fatal(http.ListenAndServe(addr, nil))
}
