// Package api exposes the collector's costed traffic Summary over HTTP as JSON, for the CLI
// (M5) and UI (M3) to consume. Kept intentionally small — one read endpoint plus health/status.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gargkrishna730/zonetax/internal/collector"
	"github.com/gargkrishna730/zonetax/internal/costengine"
)

// Handler serves the collector's REST API, reading from a *collector.Store.
type Handler struct {
	store *collector.Store
}

// NewHandler returns an api.Handler backed by store.
func NewHandler(store *collector.Store) *Handler {
	return &Handler{store: store}
}

// costsResponse is the wire format for GET /api/v1/costs and GET /api/v1/top.
type costsResponse struct {
	UpdatedAt string  `json:"updated_at,omitempty"`
	StaleErr  string  `json:"stale_error,omitempty"`
	Cloud     string  `json:"cloud"`
	Region    string  `json:"region"`
	Entries   []entry `json:"entries"`
	Totals    totals  `json:"totals"`
	// ServerTimeUTC is this response's own generation time, in RFC3339 UTC — lets the UI show
	// an exact, unambiguous "as of" timestamp (and detect client/server clock skew) rather than
	// only a vague client-side "updated Ns ago" derived from UpdatedAt.
	ServerTimeUTC string `json:"server_time_utc"`
	// CollectorStartedAtUTC is when this collector process completed its first successful
	// collection cycle (RFC3339 UTC), empty if none has completed yet. This is what "tracked"
	// totals and Cross-AZ spend are measured SINCE — a session/process metric, explicitly not a
	// calendar day, and distinct from the deeper HistoryStartUTC below.
	CollectorStartedAtUTC string `json:"collector_started_at_utc,omitempty"`
	// ScrapeIntervalSeconds is the collector's configured collection cadence — how often
	// Totals/Entries can change, i.e. the real "data freshness" bound (not the UI's own poll
	// interval, which is a separate, unrelated number).
	ScrapeIntervalSeconds int `json:"scrape_interval_seconds"`
}

type entry struct {
	SrcZone      string  `json:"src_zone"`
	DstZone      string  `json:"dst_zone"`
	SrcNamespace string  `json:"src_namespace"`
	SrcWorkload  string  `json:"src_workload"`
	DstNamespace string  `json:"dst_namespace"`
	DstWorkload  string  `json:"dst_workload"`
	GB           float64 `json:"gb"`
	CostUSD      float64 `json:"cost_usd"`
}

type totals struct {
	CrossAZGB  float64 `json:"cross_az_gb"`
	CrossAZUSD float64 `json:"cross_az_cost_usd"`
	SameAZGB   float64 `json:"same_az_gb"`
	// PricePerGBUSD is the EFFECTIVE $/GB applied to CrossAZUSD — i.e. already includes AWS's
	// bill-both-directions behavior (see costengine.crossAZBillingMultiplier), so
	// CrossAZGB * PricePerGBUSD == CrossAZUSD. PricePerGBDirectionUSD is the raw published
	// per-direction rate, included so the UI/CLI can show "why" without recomputing it.
	PricePerGBUSD          float64 `json:"price_per_gb_usd"`
	PricePerGBDirectionUSD float64 `json:"price_per_gb_direction_usd"`
}

// buildResponse converts a costengine.Summary (+ Store metadata) into the wire response shared
// by Costs and Top, so the two handlers can't drift out of sync on which fields get populated —
// exactly the kind of duplication that caused Top to previously ship without Totals set.
func buildResponse(summary costengine.Summary, updatedAt time.Time, lastErr error, entries []entry, startedAt time.Time, scrapeInterval time.Duration) costsResponse {
	resp := costsResponse{
		Cloud:                 summary.Cloud,
		Region:                summary.Region,
		Entries:               entries,
		ServerTimeUTC:         time.Now().UTC().Format(time.RFC3339),
		ScrapeIntervalSeconds: int(scrapeInterval.Seconds()),
		Totals: totals{
			CrossAZGB:              summary.TotalCrossAZGB,
			CrossAZUSD:             summary.TotalCrossAZCost,
			SameAZGB:               summary.TotalSameAZGB,
			PricePerGBUSD:          summary.EffectivePricePerGB,
			PricePerGBDirectionUSD: summary.PricePerGBDirection,
		},
	}
	if !updatedAt.IsZero() {
		resp.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	}
	if !startedAt.IsZero() {
		resp.CollectorStartedAtUTC = startedAt.UTC().Format(time.RFC3339)
	}
	if lastErr != nil {
		resp.StaleErr = lastErr.Error()
	}
	return resp
}

func toEntries(src []costengine.Entry) []entry {
	out := make([]entry, 0, len(src))
	for _, e := range src {
		out = append(out, entry{
			SrcZone: e.SrcZone, DstZone: e.DstZone,
			SrcNamespace: e.SrcNamespace, SrcWorkload: e.SrcWorkload,
			DstNamespace: e.DstNamespace, DstWorkload: e.DstWorkload,
			GB: e.GB, CostUSD: e.CostUSD,
		})
	}
	return out
}

// Costs handles GET /api/v1/costs, returning the most recently computed cost summary with all
// entries (unsorted, unfiltered). Responds 200 with the latest known-good summary even if the
// most recent collection cycle errored (StaleErr is populated in that case) — a transient scrape
// failure shouldn't make callers treat otherwise-valid recent data as unavailable.
func (h *Handler) Costs(w http.ResponseWriter, r *http.Request) {
	summary, updatedAt, lastErr := h.store.Latest()
	resp := buildResponse(summary, updatedAt, lastErr, toEntries(summary.Entries), h.store.StartedAt(), h.store.ScrapeInterval())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Top handles GET /api/v1/top, returning cross-AZ entries sorted by cost descending, optionally
// limited via ?n= (default 10). Kept as a separate endpoint (rather than a query param on Costs)
// since it's the primary shape the CLI's `zonetax top` (M5) and the UI's offenders table (M3)
// both want.
func (h *Handler) Top(w http.ResponseWriter, r *http.Request) {
	summary, updatedAt, lastErr := h.store.Latest()

	n := 10
	if v := r.URL.Query().Get("n"); v != "" {
		if parsed, err := parsePositiveInt(v); err == nil {
			n = parsed
		}
	}

	entries := toEntries(summary.Entries)
	sortByCostDesc(entries)
	if n < len(entries) {
		entries = entries[:n]
	}

	resp := buildResponse(summary, updatedAt, lastErr, entries, h.store.StartedAt(), h.store.ScrapeInterval())

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

// historyResponse is the wire format for GET /api/v1/history.
type historyResponse struct {
	// RangeRequested/RangeStartUTC/RangeEndUTC describe the window the caller asked for
	// (?range=1h|6h|24h|7d), in RFC3339 UTC — always present so the UI can label a chart's axis
	// with real, non-invented timestamps rather than a vague "last N hours".
	RangeRequested string `json:"range_requested"`
	RangeStartUTC  string `json:"range_start_utc"`
	RangeEndUTC    string `json:"range_end_utc"`
	ServerTimeUTC  string `json:"server_time_utc"`
	// HistoryStartUTC is when this collector's retained history actually begins — may be LATER
	// than RangeStartUTC (e.g. a 7-day range requested from a collector that's only been up 2
	// hours). The UI must use this, not silently assume the full requested range has data.
	HistoryStartUTC       string          `json:"history_start_utc,omitempty"`
	ScrapeIntervalSeconds int             `json:"scrape_interval_seconds"`
	BucketSizeSeconds     int             `json:"bucket_size_seconds"`
	Buckets               []historyBucket `json:"buckets"`
}

type historyBucket struct {
	StartUTC       string  `json:"start_utc"`
	EndUTC         string  `json:"end_utc"`
	CrossAZCostUSD float64 `json:"cross_az_cost_usd"`
	CrossAZGB      float64 `json:"cross_az_gb"`
	SameAZGB       float64 `json:"same_az_gb"`
	// Complete=false means this bucket's window is only partially covered by observed data
	// (the current in-progress bucket, or one that started before the collector's history
	// began) — the UI must visually distinguish this from a fully-observed bucket rather than
	// presenting a partial number as if it were the whole period's total.
	Complete bool `json:"complete"`
	// HasData=false means there is no observed data at all for this bucket (entirely before
	// the collector's history began) — distinct from Complete=false-with-partial-data.
	HasData bool `json:"has_data"`
}

// supportedRanges maps the API's ?range= query values to their duration and the fixed hourly
// bucket size used to build history.Buckets — day-level buckets are computed client-side over
// bucket boundaries where relevant, rather than the collector maintaining two bucket
// granularities, since the UI can trivially fold 24 hourly buckets into 1 daily one but not the
// reverse.
var supportedRanges = map[string]time.Duration{
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
}

// mapRanges additionally includes 15m — the shortest window offered by the Cross-AZ Service Map
// toolbar's range picker (internal/api's own /history chart never asked for anything finer than
// 1h, so this is a strict superset kept separate rather than changing supportedRanges' existing
// meaning for that endpoint).
var mapRanges = map[string]time.Duration{
	"15m": 15 * time.Minute,
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
}

// mapEntry is one route's real, observed cost/traffic delta for the requested time window —
// the wire format for GET /api/v1/map.
type mapEntry struct {
	SrcZone      string  `json:"src_zone"`
	DstZone      string  `json:"dst_zone"`
	SrcNamespace string  `json:"src_namespace"`
	SrcWorkload  string  `json:"src_workload"`
	DstNamespace string  `json:"dst_namespace"`
	DstWorkload  string  `json:"dst_workload"`
	GB           float64 `json:"gb"`
	CostUSD      float64 `json:"cost_usd"`
}

// mapResponse is the wire format for GET /api/v1/map — the Cross-AZ Service Map's primary data
// source. Unlike /api/v1/costs (which only ever reports cumulative-since-restart totals), this
// endpoint answers "what happened in THIS specific time window" — required for the map's
// 15m/1h/6h/24h/7d/custom range picker to actually rebuild routes/nodes/totals, not just relabel
// the same always-cumulative numbers.
type mapResponse struct {
	RangeRequested string `json:"range_requested"`
	RangeStartUTC  string `json:"range_start_utc"`
	RangeEndUTC    string `json:"range_end_utc"`
	ServerTimeUTC  string `json:"server_time_utc"`
	Cloud          string `json:"cloud"`
	Region         string `json:"region"`
	// HasData=false: no snapshot data exists yet to answer this query at all (collector just
	// started, or the entire window is before any data was ever recorded).
	HasData bool `json:"has_data"`
	// Complete=false: real data exists, but the window is only PARTIALLY observed — either it
	// starts before this collector's history began (fallback baseline used, undercounting the
	// unobservable portion) or it extends into the current, still-in-progress moment. The
	// entries/totals below are real, not fabricated, but do not represent the FULL requested
	// window — the UI must show this distinction, never silently present a partial number as
	// if it were the complete period's total.
	Complete               bool       `json:"complete"`
	PricePerGBUSD          float64    `json:"price_per_gb_usd"`
	PricePerGBDirectionUSD float64    `json:"price_per_gb_direction_usd"`
	Entries                []mapEntry `json:"entries"`
	TotalCrossAZGB         float64    `json:"total_cross_az_gb"`
	TotalCrossAZCostUSD    float64    `json:"total_cross_az_cost_usd"`
}

// parseRFC3339 parses a required RFC3339 query parameter, returning ok=false if missing/invalid.
func parseRFC3339(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	return t, err == nil
}

// Map handles GET /api/v1/map?range=15m|1h|6h|24h|7d|custom[&since=RFC3339&until=RFC3339],
// returning the real observed per-route cost/traffic delta for the requested window — the data
// source for the Cross-AZ Service Map's time-range picker. For "custom", since/until are
// required and must be a valid RFC3339 range with until > since; any other value for `range`
// (or a missing one) uses the corresponding fixed duration ending now.
func (h *Handler) Map(w http.ResponseWriter, r *http.Request) {
	rangeParam := r.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = "24h"
	}

	now := time.Now().UTC()
	var since time.Time
	if rangeParam == "custom" {
		s, sOK := parseRFC3339(r.URL.Query().Get("since"))
		u, uOK := parseRFC3339(r.URL.Query().Get("until"))
		if !sOK || !uOK || !u.After(s) {
			http.Error(w, `invalid custom range — "since" and "until" must be RFC3339 timestamps with until after since`, http.StatusBadRequest)
			return
		}
		since, now = s.UTC(), u.UTC()
	} else {
		dur, ok := mapRanges[rangeParam]
		if !ok {
			http.Error(w, `invalid "range" — supported values: 15m, 1h, 6h, 24h, 7d, custom`, http.StatusBadRequest)
			return
		}
		since = now.Add(-dur)
	}

	summary, _, _ := h.store.Latest()
	// A window ending at "now" (the exact request instant) will essentially never exactly match
	// a snapshot recorded on a periodic cadence — allow up to 2x the collector's real scrape
	// interval of staleness before calling the window "incomplete", so a continuously-running,
	// up-to-date collector correctly reports complete=true instead of permanently appearing
	// stale. Falls back to a conservative 60s floor if ScrapeInterval hasn't been set yet (e.g.
	// a fresh Store before Run() has started, or in unit tests using a zero-value Store).
	freshnessTolerance := 2 * h.store.ScrapeInterval()
	if freshnessTolerance < 60*time.Second {
		freshnessTolerance = 60 * time.Second
	}
	deltas, hasData, complete := h.store.History().EntriesRange(since, now, freshnessTolerance)

	resp := mapResponse{
		RangeRequested:         rangeParam,
		RangeStartUTC:          since.Format(time.RFC3339),
		RangeEndUTC:            now.Format(time.RFC3339),
		ServerTimeUTC:          time.Now().UTC().Format(time.RFC3339),
		Cloud:                  summary.Cloud,
		Region:                 summary.Region,
		HasData:                hasData,
		Complete:               complete,
		PricePerGBUSD:          summary.EffectivePricePerGB,
		PricePerGBDirectionUSD: summary.PricePerGBDirection,
		Entries:                make([]mapEntry, 0, len(deltas)),
	}
	for _, d := range deltas {
		resp.Entries = append(resp.Entries, mapEntry{
			SrcZone: d.SrcZone, DstZone: d.DstZone,
			SrcNamespace: d.SrcNamespace, SrcWorkload: d.SrcWorkload,
			DstNamespace: d.DstNamespace, DstWorkload: d.DstWorkload,
			GB: d.GB, CostUSD: d.CostUSD,
		})
		resp.TotalCrossAZGB += d.GB
		resp.TotalCrossAZCostUSD += d.CostUSD
	}
	sortMapEntriesByCostDesc(resp.Entries)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func sortMapEntriesByCostDesc(entries []mapEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].CostUSD > entries[j-1].CostUSD; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

// History handles GET /api/v1/history?range=1h|6h|24h|7d (default 24h), returning hourly
// cost/traffic delta buckets derived from collector.History's cumulative-total snapshots. This
// is in-memory history bounded to how long THIS collector process has been running (see
// collector.Store.StartedAt/History.EarliestSnapshot) — not a durable time-series database —
// and every bucket honestly reports whether it reflects a complete period or partial/no data,
// so the UI never presents an incomplete window as equivalent to a full one.
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	rangeParam := r.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = "24h"
	}
	dur, ok := supportedRanges[rangeParam]
	if !ok {
		http.Error(w, `invalid "range" — supported values: 1h, 6h, 24h, 7d`, http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	since := now.Add(-dur)
	buckets := h.store.History().Buckets(since, now, time.Hour)

	resp := historyResponse{
		RangeRequested:        rangeParam,
		RangeStartUTC:         since.Format(time.RFC3339),
		RangeEndUTC:           now.Format(time.RFC3339),
		ServerTimeUTC:         now.Format(time.RFC3339),
		ScrapeIntervalSeconds: int(h.store.ScrapeInterval().Seconds()),
		BucketSizeSeconds:     int(time.Hour.Seconds()),
		Buckets:               make([]historyBucket, 0, len(buckets)),
	}
	if earliest := h.store.History().EarliestSnapshot(); !earliest.IsZero() {
		resp.HistoryStartUTC = earliest.UTC().Format(time.RFC3339)
	}
	for _, b := range buckets {
		resp.Buckets = append(resp.Buckets, historyBucket{
			StartUTC: b.Start.UTC().Format(time.RFC3339), EndUTC: b.End.UTC().Format(time.RFC3339),
			CrossAZCostUSD: b.CrossAZCostUSD, CrossAZGB: b.CrossAZGB, SameAZGB: b.SameAZGB,
			Complete: b.Complete, HasData: b.HasData,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
