package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// Wire types: the subset of internal/api's JSON the CLI needs. Kept local (not imported) so the
// CLI only depends on the public HTTP contract, like any external client would.

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

type mapResponse struct {
	RangeRequested      string     `json:"range_requested"`
	RangeStartUTC       string     `json:"range_start_utc"`
	RangeEndUTC         string     `json:"range_end_utc"`
	Cloud               string     `json:"cloud"`
	Region              string     `json:"region"`
	HasData             bool       `json:"has_data"`
	Complete            bool       `json:"complete"`
	PricePerGBUSD       float64    `json:"price_per_gb_usd"`
	Entries             []mapEntry `json:"entries"`
	TotalCrossAZGB      float64    `json:"total_cross_az_gb"`
	TotalCrossAZCostUSD float64    `json:"total_cross_az_cost_usd"`
}

type historyBucket struct {
	StartUTC       string  `json:"start_utc"`
	CrossAZCostUSD float64 `json:"cross_az_cost_usd"`
	CrossAZGB      float64 `json:"cross_az_gb"`
	Complete       bool    `json:"complete"`
	HasData        bool    `json:"has_data"`
}

type historyResponse struct {
	HistoryStartUTC string          `json:"history_start_utc"`
	Buckets         []historyBucket `json:"buckets"`
}

var mapRanges = map[string]bool{"15m": true, "1h": true, "6h": true, "24h": true, "7d": true}
var historyRanges = map[string]bool{"1h": true, "6h": true, "24h": true, "7d": true}

type commonFlags struct {
	url     string
	rng     string
	jsonOut bool
}

func newFlagSet(name string, c *commonFlags) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	def := os.Getenv("ZONETAX_URL")
	if def == "" {
		def = "http://localhost:8080"
	}
	fs.StringVar(&c.url, "url", def, "collector base URL")
	fs.StringVar(&c.rng, "range", "24h", "time window: 15m, 1h, 6h, 24h, 7d")
	fs.BoolVar(&c.jsonOut, "json", false, "print raw JSON")
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string, c *commonFlags) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if !mapRanges[c.rng] {
		return fmt.Errorf("invalid --range %q (use 15m, 1h, 6h, 24h, 7d)", c.rng)
	}
	c.url = strings.TrimRight(c.url, "/")
	return nil
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// getJSON fetches base+path and decodes into out, also returning the raw body for --json.
func getJSON(base, path string, q url.Values, out any) ([]byte, error) {
	u := base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	resp, err := httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("cannot reach collector at %s (%v)\nhint: kubectl -n zonetax port-forward svc/zonetax-collector 8080:8080", base, unwrapURLErr(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("collector returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return nil, fmt.Errorf("unexpected response from %s (is this a ZoneTax collector?): %v", u, err)
	}
	return body, nil
}

func unwrapURLErr(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

func cmdTop(args []string, w io.Writer) error {
	var c commonFlags
	fs := newFlagSet("top", &c)
	n := fs.Int("n", 10, "number of routes")
	if err := parseFlags(fs, args, &c); err != nil {
		return err
	}
	if *n <= 0 {
		return fmt.Errorf("-n must be positive")
	}
	var m mapResponse
	raw, err := getJSON(c.url, "/api/v1/map", url.Values{"range": {c.rng}}, &m)
	if err != nil {
		return err
	}
	if c.jsonOut {
		_, err = w.Write(raw)
		return err
	}
	printHeader(w, "Top cross-AZ routes", m)
	if !m.HasData || len(m.Entries) == 0 {
		fmt.Fprintln(w, "No cross-AZ traffic observed in this window.")
		return nil
	}
	entries := m.Entries
	if len(entries) > *n {
		entries = entries[:*n]
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tSOURCE\tDESTINATION\tZONES\tGB\tCOST")
	for i, e := range entries {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s -> %s\t%s\t%s\n", i+1,
			workload(e.SrcNamespace, e.SrcWorkload), workload(e.DstNamespace, e.DstWorkload),
			e.SrcZone, e.DstZone, fmtGB(e.GB), fmtUSD(e.CostUSD))
	}
	tw.Flush()
	if len(m.Entries) > len(entries) {
		fmt.Fprintf(w, "\nShowing %d of %d routes (use -n to see more).\n", len(entries), len(m.Entries))
	}
	return nil
}

func cmdReport(args []string, w io.Writer) error {
	var c commonFlags
	fs := newFlagSet("report", &c)
	if err := parseFlags(fs, args, &c); err != nil {
		return err
	}
	var m mapResponse
	rawMap, err := getJSON(c.url, "/api/v1/map", url.Values{"range": {c.rng}}, &m)
	if err != nil {
		return err
	}
	var h historyResponse
	var rawHist []byte
	if historyRanges[c.rng] {
		if rawHist, err = getJSON(c.url, "/api/v1/history", url.Values{"range": {c.rng}}, &h); err != nil {
			return err
		}
	}
	if c.jsonOut {
		out := map[string]json.RawMessage{"map": rawMap}
		if rawHist != nil {
			out["history"] = rawHist
		}
		return json.NewEncoder(w).Encode(out)
	}

	printHeader(w, "Cross-AZ cost report", m)
	if !m.HasData {
		fmt.Fprintln(w, "No data yet for this window (collector may have just started).")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Cross-AZ traffic:\t%s\n", fmtGB(m.TotalCrossAZGB))
	fmt.Fprintf(tw, "Cross-AZ cost:\t%s\n", fmtUSD(m.TotalCrossAZCostUSD))
	if m.PricePerGBUSD > 0 {
		fmt.Fprintf(tw, "Effective price:\t$%.4f/GB\n", m.PricePerGBUSD)
	}
	fmt.Fprintf(tw, "Routes:\t%d\n", len(m.Entries))
	tw.Flush()

	if zones := byZonePair(m.Entries); len(zones) > 0 {
		fmt.Fprintln(w, "\nBy zone pair:")
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  ZONES\tGB\tCOST")
		for _, z := range zones {
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", z.key, fmtGB(z.gb), fmtUSD(z.cost))
		}
		tw.Flush()
	}

	if len(h.Buckets) > 0 {
		fmt.Fprintln(w, "\nHourly (local time):")
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  HOUR\tGB\tCOST\t")
		for _, b := range h.Buckets {
			label := b.StartUTC
			if t, err := time.Parse(time.RFC3339, b.StartUTC); err == nil {
				label = t.Local().Format("Jan 02 15:04")
			}
			switch {
			case !b.HasData:
				fmt.Fprintf(tw, "  %s\t-\t-\tno data\n", label)
			case !b.Complete:
				fmt.Fprintf(tw, "  %s\t%s\t%s\tpartial\n", label, fmtGB(b.CrossAZGB), fmtUSD(b.CrossAZCostUSD))
			default:
				fmt.Fprintf(tw, "  %s\t%s\t%s\t\n", label, fmtGB(b.CrossAZGB), fmtUSD(b.CrossAZCostUSD))
			}
		}
		tw.Flush()
	} else if !historyRanges[c.rng] {
		fmt.Fprintln(w, "\n(Hourly breakdown needs --range 1h or longer.)")
	}
	return nil
}

func printHeader(w io.Writer, title string, m mapResponse) {
	fmt.Fprintf(w, "%s, last %s", title, m.RangeRequested)
	if s, err := time.Parse(time.RFC3339, m.RangeStartUTC); err == nil {
		if e, err2 := time.Parse(time.RFC3339, m.RangeEndUTC); err2 == nil {
			fmt.Fprintf(w, " (%s to %s)", s.Local().Format("Jan 02 15:04"), e.Local().Format("Jan 02 15:04 MST"))
		}
	}
	if m.Cloud != "" {
		fmt.Fprintf(w, " [%s %s]", m.Cloud, m.Region)
	}
	fmt.Fprintln(w)
	if m.HasData && !m.Complete {
		fmt.Fprintln(w, "Note: partial window, the collector has not observed the full range yet. Numbers are real but cover less than the requested period.")
	}
	fmt.Fprintln(w)
}

type zoneAgg struct {
	key      string
	gb, cost float64
}

func byZonePair(entries []mapEntry) []zoneAgg {
	idx := map[string]int{}
	var out []zoneAgg
	for _, e := range entries {
		k := e.SrcZone + " -> " + e.DstZone
		i, ok := idx[k]
		if !ok {
			i = len(out)
			idx[k] = i
			out = append(out, zoneAgg{key: k})
		}
		out[i].gb += e.GB
		out[i].cost += e.CostUSD
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].cost > out[j-1].cost; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func workload(ns, name string) string {
	if name == "" {
		name = "unknown"
	}
	if ns == "" {
		return name
	}
	return ns + "/" + name
}

func fmtGB(gb float64) string {
	switch {
	case gb >= 100:
		return fmt.Sprintf("%.0f GB", gb)
	case gb >= 1:
		return fmt.Sprintf("%.2f GB", gb)
	default:
		return fmt.Sprintf("%.1f MB", gb*1024)
	}
}

func fmtUSD(v float64) string {
	if v != 0 && v < 0.01 {
		return fmt.Sprintf("$%.4f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}
