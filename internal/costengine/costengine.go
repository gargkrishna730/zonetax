// Package costengine turns scraped zonetax_agent_cross_az_bytes_total metric families into
// dollar costs, using the pricing table in internal/pricing. It has no knowledge of HTTP,
// Kubernetes, or Prometheus wire formats — those live in internal/scrape and internal/api.
package costengine

import (
	dto "github.com/prometheus/client_model/go"
	"sort"

	"github.com/gargkrishna730/zonetax/internal/pricing"
)

const (
	crossAZMetricName = "zonetax_agent_cross_az_bytes_total"
	sameAZMetricName  = "zonetax_agent_same_az_bytes_total"
	bytesPerGB        = 1e9
)

// Entry is one costed cross-AZ traffic bucket: bytes observed between two zones, attributed to
// the source namespace/workload, with the resulting dollar cost applied.
type Entry struct {
	SrcZone      string  `json:"src_zone"`
	DstZone      string  `json:"dst_zone"`
	SrcNamespace string  `json:"src_namespace"`
	SrcWorkload  string  `json:"src_workload"`
	Bytes        float64 `json:"bytes"`
	GB           float64 `json:"gb"`
	CostUSD      float64 `json:"cost_usd"`
}

// Summary is the full costed picture for one collection cycle: cross-AZ entries plus aggregate
// totals, including how much same-AZ (non-billable) traffic exists for context.
type Summary struct {
	Cloud            string  `json:"cloud"`
	Region           string  `json:"region"`
	PricePerGB       float64 `json:"price_per_gb_usd"`
	Entries          []Entry `json:"entries"`
	TotalCrossAZGB   float64 `json:"total_cross_az_gb"`
	TotalCrossAZCost float64 `json:"total_cross_az_cost_usd"`
	TotalSameAZGB    float64 `json:"total_same_az_gb"`
}

// Compute sums the given cross-AZ and same-AZ metric families (typically merged from multiple
// scraped agents — see MergeFamilies) and prices the cross-AZ totals using the pricing table for
// the given cloud/region. Note: these are cumulative counter values since each agent process
// started (Prometheus counter semantics), not a rate — see docs/architecture.md for why the
// agent intentionally exposes raw cumulative counters rather than computing its own deltas.
func Compute(families map[string]*dto.MetricFamily, table *pricing.Table, cloud, region string) (Summary, error) {
	pricePerGB, err := table.CrossAZPerGB(cloud, region)
	if err != nil {
		return Summary{}, err
	}

	summary := Summary{Cloud: cloud, Region: region, PricePerGB: pricePerGB}

	if mf, ok := families[crossAZMetricName]; ok {
		for _, m := range mf.GetMetric() {
			labels := labelMap(m.GetLabel())
			bytes := m.GetCounter().GetValue()
			gb := bytes / bytesPerGB
			cost := gb * pricePerGB

			summary.Entries = append(summary.Entries, Entry{
				SrcZone:      labels["src_zone"],
				DstZone:      labels["dst_zone"],
				SrcNamespace: labels["src_namespace"],
				SrcWorkload:  labels["src_workload"],
				Bytes:        bytes,
				GB:           gb,
				CostUSD:      cost,
			})
			summary.TotalCrossAZGB += gb
			summary.TotalCrossAZCost += cost
		}
	}

	if mf, ok := families[sameAZMetricName]; ok {
		for _, m := range mf.GetMetric() {
			summary.TotalSameAZGB += m.GetCounter().GetValue() / bytesPerGB
		}
	}

	return summary, nil
}

func labelMap(pairs []*dto.LabelPair) map[string]string {
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		m[p.GetName()] = p.GetValue()
	}
	return m
}

// MergeFamilies sums metric values across multiple agents' scraped families, keyed by the full
// label set. Each agent only reports flows where it observed the *source* side of the
// connection (see internal/aggregator), so summing across agents does not double-count a given
// flow — but multiple agents can each contribute distinct label combinations, or (across
// process restarts within one scrape cycle) the same combination, which does need summing.
func MergeFamilies(perAgent []map[string]*dto.MetricFamily) map[string]*dto.MetricFamily {
	type key struct {
		metric string
		labels string
	}
	totals := make(map[key]float64)
	labelSets := make(map[key][]*dto.LabelPair)
	metricTypes := make(map[string]dto.MetricType)

	for _, families := range perAgent {
		for name, mf := range families {
			if name != crossAZMetricName && name != sameAZMetricName {
				continue
			}
			metricTypes[name] = mf.GetType()
			for _, m := range mf.GetMetric() {
				k := key{metric: name, labels: labelKey(m.GetLabel())}
				totals[k] += m.GetCounter().GetValue()
				labelSets[k] = m.GetLabel()
			}
		}
	}

	merged := make(map[string]*dto.MetricFamily)
	for k, total := range totals {
		mf, ok := merged[k.metric]
		if !ok {
			name := k.metric
			mtype := metricTypes[k.metric]
			mf = &dto.MetricFamily{Name: &name, Type: &mtype}
			merged[k.metric] = mf
		}
		value := total
		mf.Metric = append(mf.Metric, &dto.Metric{
			Label:   labelSets[k],
			Counter: &dto.Counter{Value: &value},
		})
	}
	return merged
}

func labelKey(pairs []*dto.LabelPair) string {
	// Sort by name so the key is independent of the caller's label order — expfmt parsing
	// happens to produce alphabetical order already, but relying on that implicitly would make
	// this fragile against any future non-expfmt caller (e.g. hand-built metrics in tests).
	sorted := make([]*dto.LabelPair, len(pairs))
	copy(sorted, pairs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].GetName() < sorted[j].GetName() })

	s := ""
	for _, p := range sorted {
		s += p.GetName() + "=" + p.GetValue() + ";"
	}
	return s
}
