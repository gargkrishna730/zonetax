package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeCollector(t *testing.T, m mapResponse, h *historyResponse) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/map", func(w http.ResponseWriter, r *http.Request) {
		m.RangeRequested = r.URL.Query().Get("range")
		json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("/api/v1/history", func(w http.ResponseWriter, r *http.Request) {
		if h == nil {
			http.Error(w, "nope", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(h)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

var sample = mapResponse{
	RangeStartUTC: "2026-10-01T00:00:00Z", RangeEndUTC: "2026-10-02T00:00:00Z",
	Cloud: "aws", Region: "us-east-1", HasData: true, Complete: true, PricePerGBUSD: 0.02,
	Entries: []mapEntry{
		{SrcZone: "a", DstZone: "b", SrcNamespace: "shop", SrcWorkload: "api", DstNamespace: "shop", DstWorkload: "db", GB: 10, CostUSD: 0.20},
		{SrcZone: "b", DstZone: "c", SrcNamespace: "obs", SrcWorkload: "otel", DstNamespace: "obs", DstWorkload: "tempo", GB: 2, CostUSD: 0.04},
		{SrcZone: "a", DstZone: "b", SrcNamespace: "shop", SrcWorkload: "web", DstNamespace: "shop", DstWorkload: "api", GB: 0.5, CostUSD: 0.01},
	},
	TotalCrossAZGB: 12.5, TotalCrossAZCostUSD: 0.25,
}

func runCLI(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestTopTableAndLimit(t *testing.T) {
	srv := fakeCollector(t, sample, nil)
	code, out, errs := runCLI("top", "--url", srv.URL, "-n", "2", "--range", "6h")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	for _, want := range []string{"last 6h", "shop/api", "shop/db", "a -> b", "$0.20", "10.00 GB", "Showing 2 of 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "shop/web") {
		t.Errorf("-n 2 should hide the third route:\n%s", out)
	}
}

func TestTopJSONPassthrough(t *testing.T) {
	srv := fakeCollector(t, sample, nil)
	code, out, _ := runCLI("top", "--json", "--url", srv.URL)
	if code != 0 {
		t.Fatal("non-zero exit")
	}
	var m mapResponse
	if err := json.Unmarshal([]byte(out), &m); err != nil || len(m.Entries) != 3 {
		t.Fatalf("expected raw JSON with 3 entries, got err=%v out=%s", err, out)
	}
}

func TestTopNoDataIsHonest(t *testing.T) {
	srv := fakeCollector(t, mapResponse{HasData: false}, nil)
	_, out, _ := runCLI("top", "--url", srv.URL)
	if !strings.Contains(out, "No cross-AZ traffic observed") {
		t.Fatalf("expected no-data message:\n%s", out)
	}
}

func TestReportPartialAndHourly(t *testing.T) {
	m := sample
	m.Complete = false
	h := &historyResponse{Buckets: []historyBucket{
		{StartUTC: "2026-10-01T00:00:00Z", HasData: false},
		{StartUTC: "2026-10-01T01:00:00Z", HasData: true, Complete: true, CrossAZGB: 1, CrossAZCostUSD: 0.02},
		{StartUTC: "2026-10-01T02:00:00Z", HasData: true, Complete: false, CrossAZGB: 0.1, CrossAZCostUSD: 0.002},
	}}
	srv := fakeCollector(t, m, h)
	code, out, errs := runCLI("report", "--url", srv.URL)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	for _, want := range []string{"partial window", "$0.25", "12.50 GB", "By zone pair", "a -> b", "$0.21", "no data", "partial", "$0.0020"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestReport15mSkipsHistory(t *testing.T) {
	srv := fakeCollector(t, sample, nil) // history would 400 if called
	code, out, errs := runCLI("report", "--range", "15m", "--url", srv.URL)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if !strings.Contains(out, "needs --range 1h") {
		t.Errorf("expected hint about hourly breakdown:\n%s", out)
	}
}

func TestErrors(t *testing.T) {
	if code, _, errs := runCLI("top", "--range", "3d"); code != 1 || !strings.Contains(errs, "invalid --range") {
		t.Errorf("bad range: code=%d err=%s", code, errs)
	}
	if code, _, errs := runCLI("top", "--url", "http://127.0.0.1:1"); code != 1 || !strings.Contains(errs, "port-forward") {
		t.Errorf("unreachable: code=%d err=%s", code, errs)
	}
	if code, _, _ := runCLI("bogus"); code != 2 {
		t.Errorf("unknown command should exit 2, got %d", code)
	}
	if code, _, _ := runCLI(); code != 2 {
		t.Errorf("no args should exit 2, got %d", code)
	}
	if code, out, _ := runCLI("version"); code != 0 || !strings.Contains(out, "zonetax dev") {
		t.Errorf("version: %d %s", code, out)
	}
}

func TestEnvURL(t *testing.T) {
	srv := fakeCollector(t, sample, nil)
	t.Setenv("ZONETAX_URL", srv.URL+"/")
	if code, _, errs := runCLI("top"); code != 0 {
		t.Fatalf("ZONETAX_URL not honored: %s", errs)
	}
}
