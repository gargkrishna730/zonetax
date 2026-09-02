// Command collector aggregates cross-AZ traffic metrics scraped from agent DaemonSet pods,
// applies the pricing table, and serves the REST API + UI.
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gargkrishna730/zonetax/internal/pricing"
)

func main() {
	table, err := pricing.LoadDefault()
	if err != nil {
		log.Fatalf("load pricing table: %v", err)
	}
	log.Printf("loaded pricing table v%d (%d cloud providers)", table.Version, len(table.Pricing))

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	// TODO(M2): /api/v1/costs endpoint aggregating scraped agent metrics through the pricing table.
	// TODO(M3): serve the static UI (ui/) via embed.FS at "/".

	addr := ":8080"
	log.Printf("zonetax-collector starting on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
