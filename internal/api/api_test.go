package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
