// Package collectormetrics exposes the collector's costed view of the cluster as Prometheus
// metrics on /metrics, so cross-AZ spend can be alerted on and graphed with the tooling a
// cluster already runs (Prometheus, Alertmanager, Grafana) instead of ZoneTax shipping its own
// notifier.
//
// The agent's own /metrics (see internal/metrics) deliberately exposes raw byte counters with no
// cost math. These metrics are the other half: dollars, already priced for the detected
// cloud/region, plus the measurement-coverage signals needed to avoid alerting on missing data.
package collectormetrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/gargkrishna730/zonetax/internal/costengine"
)

// Gauges rather than counters: these are the current value of a sampled window, not a
// monotonically increasing total. Prometheus counters must never decrease, and these can
// (traffic stops, agents restart, the window rolls).
var (
	// CrossAZCostUSD is the costed cross-AZ spend for the collector's current window.
	CrossAZCostUSD = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "zonetax",
		Name:      "cross_az_cost_usd",
		Help:      "Cross-AZ transfer cost in USD for the current observation window, priced for the detected cloud/region.",
	})

	// CrossAZGB is the observed cross-AZ volume behind CrossAZCostUSD.
	CrossAZGB = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "zonetax",
		Name:      "cross_az_gb",
		Help:      "Cross-AZ transfer volume in GB for the current observation window.",
	})

	// UnattributedGB is traffic seen but not attributable to two in-cluster pods. Exposed so
	// the gap against the cloud bill is visible rather than silently missing.
	UnattributedGB = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "zonetax",
		Name:      "unattributed_gb",
		Help:      "Traffic observed but not attributable to two in-cluster pods (internet, managed services, load balancers). Not costed.",
	})

	// EffectivePricePerGB is the rate actually applied, which already includes the
	// bill-both-directions multiplier. Exposed so a dashboard can explain a cost change that
	// came from pricing rather than from traffic.
	EffectivePricePerGB = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "zonetax",
		Name:      "effective_price_per_gb_usd",
		Help:      "Effective USD per observed GB of cross-AZ traffic, including the bill-both-directions multiplier.",
	}, []string{"cloud", "region"})

	// NodesTotal and NodesMeasured together say how much of the cluster was actually observed.
	// Alert on these: a drop in reported cost is meaningless if coverage dropped too.
	NodesTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "zonetax",
		Name:      "nodes_total",
		Help:      "Number of nodes in the cluster.",
	})

	NodesMeasured = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "zonetax",
		Name:      "nodes_measured",
		Help:      "Number of nodes whose agent reported metrics in the last cycle. Less than zonetax_nodes_total means the cost figures are incomplete.",
	})

	// CoverageComplete is 1 only when every node reported and no scrape failed, giving a single
	// boolean to gate alerts on so they don't fire on partial data.
	CoverageComplete = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "zonetax",
		Name:      "coverage_complete",
		Help:      "1 if every node was measured in the last cycle, 0 if measurement was partial.",
	})

	// PerRouteCostUSD breaks spend down by zone pair and workload pair, matching the dashboard's
	// top-offenders view. Cardinality is bounded by distinct workload pairs that actually talk
	// across zones, which is small in practice.
	PerRouteCostUSD = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "zonetax",
		Name:      "route_cross_az_cost_usd",
		Help:      "Cross-AZ cost in USD for the current window, broken down by source/destination zone and workload.",
	}, []string{"src_zone", "dst_zone", "src_namespace", "src_workload", "dst_namespace", "dst_workload"})
)

// Register adds the collector's metrics to a registry. Kept separate from the package-level vars
// so tests can use a fresh registry instead of the global one.
func Register(r prometheus.Registerer) {
	r.MustRegister(
		CrossAZCostUSD, CrossAZGB, UnattributedGB, EffectivePricePerGB,
		NodesTotal, NodesMeasured, CoverageComplete, PerRouteCostUSD,
	)
}

// Update publishes one costed Summary to the metrics above.
//
// Per-route gauges are reset first: a route that stopped carrying traffic must disappear rather
// than keep reporting its last value forever, which would make a fixed cost problem look
// permanently unfixed.
func Update(s costengine.Summary) {
	CrossAZCostUSD.Set(s.TotalCrossAZCost)
	CrossAZGB.Set(s.TotalCrossAZGB)
	UnattributedGB.Set(s.TotalUnattributedGB)

	EffectivePricePerGB.Reset()
	EffectivePricePerGB.WithLabelValues(s.Cloud, regionLabel(s.Region)).Set(s.EffectivePricePerGB)

	NodesTotal.Set(float64(s.Coverage.NodesTotal))
	NodesMeasured.Set(float64(s.Coverage.AgentsScraped))
	CoverageComplete.Set(boolToFloat(s.Coverage.Complete()))

	PerRouteCostUSD.Reset()
	for _, e := range s.Entries {
		PerRouteCostUSD.WithLabelValues(
			e.SrcZone, e.DstZone,
			e.SrcNamespace, e.SrcWorkload,
			e.DstNamespace, e.DstWorkload,
		).Set(e.CostUSD)
	}
}

// regionLabel keeps an undetermined region visible as "unknown" rather than an empty label,
// which is easy to miss in a dashboard or alert expression.
func regionLabel(region string) string {
	if region == "" {
		return "unknown"
	}
	return region
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
