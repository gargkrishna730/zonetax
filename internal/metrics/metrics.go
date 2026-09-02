// Package metrics defines the Prometheus metrics the agent exposes on /metrics. Deliberately
// scoped to raw byte counts only — no cost math here (see internal/pricing / cmd/collector).
package metrics

import "github.com/prometheus/client_golang/prometheus"

// CrossAZBytesTotal counts bytes transferred, labeled by source/destination AZ and the source
// namespace/workload responsible. It's a Counter (monotonically increasing) because conntrack
// byte counters are cumulative per-connection; the collector computes rates via Prometheus
// `rate()`/`increase()` rather than the agent trying to compute deltas itself.
var CrossAZBytesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "zonetax",
		Subsystem: "agent",
		Name:      "cross_az_bytes_total",
		Help:      "Cumulative bytes transferred between availability zones, by source/destination zone and originating namespace/workload.",
	},
	[]string{"src_zone", "dst_zone", "src_namespace", "src_workload"},
)

// SameAZBytesTotal is the same breakdown for same-zone (non-billable) traffic, kept separate so
// dashboards can show cross-AZ traffic as a fraction of total traffic.
var SameAZBytesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "zonetax",
		Subsystem: "agent",
		Name:      "same_az_bytes_total",
		Help:      "Cumulative bytes transferred within the same availability zone, by zone and originating namespace/workload.",
	},
	[]string{"zone", "src_namespace", "src_workload"},
)

// SampleDurationSeconds observes how long each conntrack sample-and-aggregate cycle takes, to
// catch performance regressions (e.g. huge conntrack tables on busy nodes).
var SampleDurationSeconds = prometheus.NewHistogram(
	prometheus.HistogramOpts{
		Namespace: "zonetax",
		Subsystem: "agent",
		Name:      "sample_duration_seconds",
		Help:      "Time taken to read, parse, and aggregate one conntrack sample.",
		Buckets:   prometheus.DefBuckets,
	},
)

// UnresolvedFlowsTotal counts flows skipped because one or both endpoints couldn't be resolved
// to a known pod/AZ (e.g. traffic to/from outside the cluster). High values are expected on
// nodes handling lots of egress traffic; a sudden change signals a podindex sync problem.
var UnresolvedFlowsTotal = prometheus.NewCounter(
	prometheus.CounterOpts{
		Namespace: "zonetax",
		Subsystem: "agent",
		Name:      "unresolved_flows_total",
		Help:      "Count of conntrack flows skipped because source and/or destination IP could not be resolved to a known pod.",
	},
)

// MustRegister registers all agent metrics with the given registerer. Called once at startup;
// panics on duplicate registration (a programming error, not a runtime condition to recover from).
func MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(CrossAZBytesTotal, SameAZBytesTotal, SampleDurationSeconds, UnresolvedFlowsTotal)
}
