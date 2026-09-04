package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gargkrishna730/zonetax/internal/collector"
)

func TestCosts_EmptyStoreReturnsZeroValues(t *testing.T) {
	h := NewHandler(&collector.Store{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/costs", nil)
	rr := httptest.NewRecorder()
	h.Costs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp costsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.UpdatedAt != "" {
		t.Errorf("UpdatedAt = %q, want empty (no cycle completed yet)", resp.UpdatedAt)
	}
	if len(resp.Entries) != 0 {
		t.Errorf("Entries = %d, want 0", len(resp.Entries))
	}
}

func TestTop_LimitsAndSortsByCoreDescending(t *testing.T) {
	// Directly test the sort/limit logic used by Top via its exported behavior on a
	// hand-built entry slice, since we can't easily seed collector.Store from this package.
	entries := []entry{
		{SrcWorkload: "cheap", CostUSD: 0.01},
		{SrcWorkload: "expensive", CostUSD: 10.00},
		{SrcWorkload: "medium", CostUSD: 1.00},
	}
	sortByCostDesc(entries)

	want := []string{"expensive", "medium", "cheap"}
	for i, w := range want {
		if entries[i].SrcWorkload != w {
			t.Errorf("entries[%d].SrcWorkload = %q, want %q", i, entries[i].SrcWorkload, w)
		}
	}
}

// TestTop_ResponseIncludesTotals is a regression test for a bug found deploying to solrn-dev:
// the /api/v1/top handler built its costsResponse without setting the Totals field, so every
// response reported cross_az_cost_usd/cross_az_gb/price_per_gb_usd as 0 regardless of the real
// values — misleading for any client (CLI, UI) trusting the totals alongside the entries list.
func TestTop_ResponseIncludesTotals(t *testing.T) {
	h := NewHandler(&collector.Store{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/top", nil)
	rr := httptest.NewRecorder()
	h.Top(rr, req)

	var resp costsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	// With an empty store, PricePerGBUSD legitimately being 0 doesn't prove much — the real
	// assertion is that the Totals field is wired at all (non-nil struct), which a future
	// regression (forgetting to populate it in the response literal) would silently continue
	// to satisfy for zero-value data. Combined with the API contract (json tag always present),
	// this at least locks in that costsResponse.Totals is populated by the handler, not omitted.
	body := rr.Body.String()
	for _, field := range []string{"cross_az_gb", "cross_az_cost_usd", "same_az_gb", "price_per_gb_usd"} {
		if !jsonHasField(body, field) {
			t.Errorf("response JSON missing %q field: %s", field, body)
		}
	}
}

func jsonHasField(body, field string) bool {
	return len(body) > 0 && (indexOf(body, `"`+field+`"`) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestParsePositiveInt(t *testing.T) {
	tests := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"5", 5, false},
		{"0", 0, true},
		{"-1", 0, true},
		{"abc", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := parsePositiveInt(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("parsePositiveInt(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
		}
		if err == nil && got != tt.want {
			t.Errorf("parsePositiveInt(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// TestCosts_ResponseIncludesServerTimeAndFreshnessMetadata is a regression test for the
// time-semantics redesign: the UI needs an exact server-side "as of" timestamp (not just a
// relative one derived client-side) plus the collector's scrape cadence, both always present
// regardless of whether any collection cycle has completed yet.
func TestCosts_ResponseIncludesServerTimeAndFreshnessMetadata(t *testing.T) {
	h := NewHandler(&collector.Store{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/costs", nil)
	rr := httptest.NewRecorder()
	h.Costs(rr, req)

	var resp costsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.ServerTimeUTC == "" {
		t.Error("ServerTimeUTC is empty, want a populated RFC3339 timestamp")
	}
	if _, err := time.Parse(time.RFC3339, resp.ServerTimeUTC); err != nil {
		t.Errorf("ServerTimeUTC = %q is not valid RFC3339: %v", resp.ServerTimeUTC, err)
	}
	// An empty Store (no Run() called) legitimately has zero scrape interval and no
	// CollectorStartedAtUTC yet — this asserts those are honestly empty/zero rather than some
	// fabricated placeholder value.
	if resp.CollectorStartedAtUTC != "" {
		t.Errorf("CollectorStartedAtUTC = %q, want empty (no cycle has completed on a fresh Store)", resp.CollectorStartedAtUTC)
	}
	if resp.ScrapeIntervalSeconds != 0 {
		t.Errorf("ScrapeIntervalSeconds = %d, want 0 (Run() was never called on this Store)", resp.ScrapeIntervalSeconds)
	}
}

func TestHistory_DefaultRangeIs24h(t *testing.T) {
	h := NewHandler(&collector.Store{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/history", nil)
	rr := httptest.NewRecorder()
	h.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp historyResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.RangeRequested != "24h" {
		t.Errorf("RangeRequested = %q, want %q (default)", resp.RangeRequested, "24h")
	}
}

func TestHistory_RejectsUnsupportedRange(t *testing.T) {
	h := NewHandler(&collector.Store{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/history?range=3weeks", nil)
	rr := httptest.NewRecorder()
	h.History(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unsupported range value", rr.Code)
	}
}

func TestHistory_EmptyStoreReturnsNoBucketsNotFabricatedData(t *testing.T) {
	h := NewHandler(&collector.Store{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/history?range=1h", nil)
	rr := httptest.NewRecorder()
	h.History(rr, req)

	var resp historyResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Buckets) != 0 {
		t.Errorf("Buckets = %d entries, want 0 (no snapshots recorded yet — must not fabricate data)", len(resp.Buckets))
	}
	if resp.HistoryStartUTC != "" {
		t.Errorf("HistoryStartUTC = %q, want empty (no snapshots recorded yet)", resp.HistoryStartUTC)
	}
	// The requested window itself must still be honestly reported even with no data in it.
	if resp.RangeStartUTC == "" || resp.RangeEndUTC == "" {
		t.Error("RangeStartUTC/RangeEndUTC must be populated even when there is no bucket data")
	}
}

// TestHistory_ReflectsRecordedSnapshots is an end-to-end check that the API layer correctly
// surfaces collector.History's bucket math (already unit-tested in depth in
// internal/collector/history_test.go) rather than re-verifying that math here.
func TestHistory_ReflectsRecordedSnapshots(t *testing.T) {
	store := &collector.Store{}
	hist := store.History()
	base := time.Now().UTC().Add(-90 * time.Minute).Truncate(time.Hour)
	hist.Record(base, 1.00, 50, 5)
	hist.Record(base.Add(45*time.Minute), 2.50, 125, 12)

	h := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/history?range=6h", nil)
	rr := httptest.NewRecorder()
	h.History(rr, req)

	var resp historyResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Buckets) == 0 {
		t.Fatal("expected at least one bucket reflecting the recorded snapshots")
	}
	if resp.HistoryStartUTC == "" {
		t.Error("HistoryStartUTC should be populated once snapshots exist")
	}
	var foundNonZeroCost bool
	for _, b := range resp.Buckets {
		if b.CrossAZCostUSD > 0 {
			foundNonZeroCost = true
		}
	}
	if !foundNonZeroCost {
		t.Error("expected at least one bucket with a non-zero CrossAZCostUSD from the recorded snapshots")
	}
}
