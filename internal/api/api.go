// Package api exposes the collector's costed traffic Summary over HTTP as JSON, for the CLI
// (M5) and UI (M3) to consume. Kept intentionally small — one read endpoint plus health/status.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gargkrishna730/zonetax/internal/collector"
)

// Handler serves the collector's REST API, reading from a *collector.Store.
type Handler struct {
	store *collector.Store
}

// NewHandler returns an api.Handler backed by store.
func NewHandler(store *collector.Store) *Handler {
	return &Handler{store: store}
}

// costsResponse is the wire format for GET /api/v1/costs.
type costsResponse struct {
	UpdatedAt string  `json:"updated_at,omitempty"`
	StaleErr  string  `json:"stale_error,omitempty"`
	Cloud     string  `json:"cloud"`
	Region    string  `json:"region"`
	Entries   []entry `json:"entries"`
	Totals    totals  `json:"totals"`
}

type entry struct {
	SrcZone      string  `json:"src_zone"`
	DstZone      string  `json:"dst_zone"`
	SrcNamespace string  `json:"src_namespace"`
	SrcWorkload  string  `json:"src_workload"`
	GB           float64 `json:"gb"`
	CostUSD      float64 `json:"cost_usd"`
}

type totals struct {
	CrossAZGB     float64 `json:"cross_az_gb"`
	CrossAZUSD    float64 `json:"cross_az_cost_usd"`
	SameAZGB      float64 `json:"same_az_gb"`
	PricePerGBUSD float64 `json:"price_per_gb_usd"`
}

// Costs handles GET /api/v1/costs, returning the most recently computed cost summary. Responds
// 200 with the latest known-good summary even if the most recent collection cycle errored
// (StaleErr is populated in that case) — a transient scrape failure shouldn't make callers treat
// otherwise-valid recent data as unavailable.
func (h *Handler) Costs(w http.ResponseWriter, r *http.Request) {
	summary, updatedAt, lastErr := h.store.Latest()

	resp := costsResponse{
		Cloud:  summary.Cloud,
		Region: summary.Region,
		Totals: totals{
			CrossAZGB:     summary.TotalCrossAZGB,
			CrossAZUSD:    summary.TotalCrossAZCost,
			SameAZGB:      summary.TotalSameAZGB,
			PricePerGBUSD: summary.PricePerGB,
		},
	}
	if !updatedAt.IsZero() {
		resp.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	}
	if lastErr != nil {
		resp.StaleErr = lastErr.Error()
	}
	for _, e := range summary.Entries {
		resp.Entries = append(resp.Entries, entry{
			SrcZone: e.SrcZone, DstZone: e.DstZone,
			SrcNamespace: e.SrcNamespace, SrcWorkload: e.SrcWorkload,
			GB: e.GB, CostUSD: e.CostUSD,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Top handles GET /api/v1/top, returning cross-AZ entries sorted by cost descending, optionally
// limited via ?n=. Kept as a separate endpoint (rather than a query param on Costs) since it's
// the primary shape the CLI's `zonetax top` (M5) and the UI's offenders table (M3) both want.
func (h *Handler) Top(w http.ResponseWriter, r *http.Request) {
	summary, updatedAt, lastErr := h.store.Latest()

	n := 10
	if v := r.URL.Query().Get("n"); v != "" {
		if parsed, err := parsePositiveInt(v); err == nil {
			n = parsed
		}
	}

	entries := make([]entry, 0, len(summary.Entries))
	for _, e := range summary.Entries {
		entries = append(entries, entry{
			SrcZone: e.SrcZone, DstZone: e.DstZone,
			SrcNamespace: e.SrcNamespace, SrcWorkload: e.SrcWorkload,
			GB: e.GB, CostUSD: e.CostUSD,
		})
	}
	sortByCostDesc(entries)
	if n < len(entries) {
		entries = entries[:n]
	}

	resp := costsResponse{
		Cloud:   summary.Cloud,
		Region:  summary.Region,
		Entries: entries,
		Totals: totals{
			CrossAZGB:     summary.TotalCrossAZGB,
			CrossAZUSD:    summary.TotalCrossAZCost,
			SameAZGB:      summary.TotalSameAZGB,
			PricePerGBUSD: summary.PricePerGB,
		},
	}
	if !updatedAt.IsZero() {
		resp.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	}
	if lastErr != nil {
		resp.StaleErr = lastErr.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func sortByCostDesc(entries []entry) {
	// Simple insertion sort: entry counts are small (bounded by distinct AZ-pair/workload
	// combos in one cluster), so O(n^2) is fine and avoids pulling in sort.Slice's closure
	// allocation for what's typically a handful of rows.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].CostUSD > entries[j-1].CostUSD; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func parsePositiveInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errNotANumber
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return 0, errNotANumber
	}
	return n, nil
}

var errNotANumber = errInvalidInt("not a positive integer")

type errInvalidInt string

func (e errInvalidInt) Error() string { return string(e) }
