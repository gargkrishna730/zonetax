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
