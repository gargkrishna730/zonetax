package collectormetrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/gargkrishna730/zonetax/internal/costengine"
)

func TestUpdatePublishesCostAndCoverage(t *testing.T) {
	Update(costengine.Summary{
		Cloud:               "aws",
		Region:              "us-west-2",
		TotalCrossAZCost:    0.42,
		TotalCrossAZGB:      21,
		TotalUnattributedGB: 7,
		EffectivePricePerGB: 0.02,
		Coverage: costengine.Coverage{
			NodesTotal:    5,
			AgentsScraped: 5,
		},
	})

	if got := testutil.ToFloat64(CrossAZCostUSD); got != 0.42 {
		t.Errorf("cost = %v, want 0.42", got)
	}
	if got := testutil.ToFloat64(CrossAZGB); got != 21 {
		t.Errorf("gb = %v, want 21", got)
	}
	if got := testutil.ToFloat64(UnattributedGB); got != 7 {
		t.Errorf("unattributed = %v, want 7", got)
	}
	if got := testutil.ToFloat64(CoverageComplete); got != 1 {
		t.Errorf("coverage_complete = %v, want 1", got)
	}
	if got := testutil.ToFloat64(EffectivePricePerGB.WithLabelValues("aws", "us-west-2")); got != 0.02 {
		t.Errorf("price = %v, want 0.02", got)
	}
}

// Partial coverage must be machine-detectable, so an alert can avoid firing on incomplete data.
func TestUpdateFlagsPartialCoverage(t *testing.T) {
	Update(costengine.Summary{
		Coverage: costengine.Coverage{
			NodesTotal:      5,
			AgentsScraped:   4,
			UnmeasuredNodes: []string{"n5"},
		},
	})

	if got := testutil.ToFloat64(CoverageComplete); got != 0 {
		t.Errorf("coverage_complete = %v, want 0 when a node is unmeasured", got)
	}
	if got := testutil.ToFloat64(NodesTotal); got != 5 {
		t.Errorf("nodes_total = %v, want 5", got)
	}
	if got := testutil.ToFloat64(NodesMeasured); got != 4 {
		t.Errorf("nodes_measured = %v, want 4", got)
	}
}

// A route that stops carrying traffic must disappear from the metrics. Otherwise a cost problem
// that was actually fixed would keep reporting its last value forever and alerts would never
// clear.
func TestUpdateClearsStaleRoutes(t *testing.T) {
	entry := func(src, dst string, cost float64) costengine.Entry {
		return costengine.Entry{
			SrcZone: "us-east-1a", DstZone: "us-east-1c",
			SrcNamespace: "ns", SrcWorkload: src,
			DstNamespace: "ns", DstWorkload: dst,
			CostUSD: cost,
		}
	}

	Update(costengine.Summary{Entries: []costengine.Entry{
		entry("noisy", "db", 1.5),
		entry("quiet", "db", 0.1),
	}})
	if n := testutil.CollectAndCount(PerRouteCostUSD); n != 2 {
		t.Fatalf("route series = %d, want 2", n)
	}

	// "noisy" was fixed and no longer appears in the summary.
	Update(costengine.Summary{Entries: []costengine.Entry{
		entry("quiet", "db", 0.1),
	}})

	if n := testutil.CollectAndCount(PerRouteCostUSD); n != 1 {
		t.Errorf("route series = %d, want 1 after the route went away", n)
	}
	out := dump(t, PerRouteCostUSD)
	if strings.Contains(out, `src_workload="noisy"`) {
		t.Error("a route with no current traffic is still being reported")
	}
}

// An undetermined region must be visible rather than an empty label that's easy to overlook.
func TestUpdateLabelsUnknownRegion(t *testing.T) {
	Update(costengine.Summary{Cloud: "aws", Region: "", EffectivePricePerGB: 0.02})
	if got := testutil.ToFloat64(EffectivePricePerGB.WithLabelValues("aws", "unknown")); got != 0.02 {
		t.Errorf("expected region label %q, got series missing (%v)", "unknown", got)
	}
}

func TestRegisterDoesNotPanic(t *testing.T) {
	// A fresh registry, so this does not collide with the package-level default.
	Register(prometheus.NewRegistry())
}

func dump(t *testing.T, c prometheus.Collector) string {
	t.Helper()
	var sb strings.Builder
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		// Already registered elsewhere; gather from the collector directly instead.
		mfs, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			t.Fatal(err)
		}
		for _, mf := range mfs {
			sb.WriteString(mf.String())
		}
		return sb.String()
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		sb.WriteString(mf.String())
	}
	return sb.String()
}
