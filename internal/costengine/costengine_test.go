package costengine

import (
	"math"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/gargkrishna730/zonetax/internal/pricing"
)

func strp(s string) *string   { return &s }
func f64p(f float64) *float64 { return &f }
func mtype() dto.MetricType   { return dto.MetricType_COUNTER }
func mtypep() *dto.MetricType { t := mtype(); return &t }

func counterMetric(labels map[string]string, value float64) *dto.Metric {
	var lp []*dto.LabelPair
	for k, v := range labels {
		lp = append(lp, &dto.LabelPair{Name: strp(k), Value: strp(v)})
	}
	return &dto.Metric{Label: lp, Counter: &dto.Counter{Value: f64p(value)}}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestCompute_PricesCrossAZTraffic(t *testing.T) {
	table, err := pricing.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error = %v", err)
	}

	families := map[string]*dto.MetricFamily{
		crossAZMetricName: {
			Name: strp(crossAZMetricName),
			Type: mtypep(),
			Metric: []*dto.Metric{
				counterMetric(map[string]string{
					"src_zone": "us-east-1a", "dst_zone": "us-east-1b",
					"src_namespace": "prod", "src_workload": "web",
					"dst_namespace": "prod", "dst_workload": "db",
				}, 1e9), // exactly 1 GB
			},
		},
	}

	summary, err := Compute(families, table, "aws", "us-east-1")
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if len(summary.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(summary.Entries))
	}
	e := summary.Entries[0]
	if !almostEqual(e.GB, 1.0) {
		t.Errorf("GB = %v, want 1.0", e.GB)
	}
	if !almostEqual(e.CostUSD, 0.02) { // 1GB observed * $0.01/GB direction * 2x billing = $0.02
		t.Errorf("CostUSD = %v, want 0.02", e.CostUSD)
	}
	if !almostEqual(summary.TotalCrossAZCost, 0.02) {
		t.Errorf("TotalCrossAZCost = %v, want 0.02", summary.TotalCrossAZCost)
	}
	if e.SrcWorkload != "web" || e.SrcNamespace != "prod" {
		t.Errorf("source attribution = %s/%s, want prod/web", e.SrcNamespace, e.SrcWorkload)
	}
	if e.DstWorkload != "db" || e.DstNamespace != "prod" {
		t.Errorf("destination attribution = %s/%s, want prod/db", e.DstNamespace, e.DstWorkload)
	}
}

func TestCompute_UnknownRegionFallsBackToDefault(t *testing.T) {
	table, _ := pricing.LoadDefault()
	summary, err := Compute(map[string]*dto.MetricFamily{}, table, "aws", "af-south-1")
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if !almostEqual(summary.EffectivePricePerGB, 0.02) {
		t.Errorf("EffectivePricePerGB = %v, want 0.02 (default)", summary.EffectivePricePerGB)
	}
}

func TestCompute_UnknownCloudErrors(t *testing.T) {
	table, _ := pricing.LoadDefault()
	if _, err := Compute(map[string]*dto.MetricFamily{}, table, "gcp", "us-east-1"); err == nil {
		t.Error("Compute() error = nil, want error for unknown cloud")
	}
}

func TestCompute_SameAZTotalledSeparately(t *testing.T) {
	table, _ := pricing.LoadDefault()
	families := map[string]*dto.MetricFamily{
		sameAZMetricName: {
			Name: strp(sameAZMetricName),
			Type: mtypep(),
			Metric: []*dto.Metric{
				counterMetric(map[string]string{"zone": "us-east-1a"}, 2e9),
			},
		},
	}
	summary, err := Compute(families, table, "aws", "us-east-1")
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if !almostEqual(summary.TotalSameAZGB, 2.0) {
		t.Errorf("TotalSameAZGB = %v, want 2.0", summary.TotalSameAZGB)
	}
	if len(summary.Entries) != 0 {
		t.Errorf("Entries = %d, want 0 (same-AZ shouldn't produce cost entries)", len(summary.Entries))
	}
}

func TestMergeFamilies_SumsAcrossAgents(t *testing.T) {
	labels := map[string]string{
		"src_zone": "us-east-1a", "dst_zone": "us-east-1b",
		"src_namespace": "prod", "src_workload": "web",
	}
	agent1 := map[string]*dto.MetricFamily{
		crossAZMetricName: {
			Name: strp(crossAZMetricName), Type: mtypep(),
			Metric: []*dto.Metric{counterMetric(labels, 100)},
		},
	}
	agent2 := map[string]*dto.MetricFamily{
		crossAZMetricName: {
			Name: strp(crossAZMetricName), Type: mtypep(),
			Metric: []*dto.Metric{counterMetric(labels, 250)},
		},
	}

	merged := MergeFamilies([]map[string]*dto.MetricFamily{agent1, agent2})
	mf, ok := merged[crossAZMetricName]
	if !ok {
		t.Fatal("merged families missing cross-AZ metric")
	}
	if len(mf.Metric) != 1 {
		t.Fatalf("merged metric count = %d, want 1 (same label set should merge)", len(mf.Metric))
	}
	if got := mf.Metric[0].GetCounter().GetValue(); !almostEqual(got, 350) {
		t.Errorf("merged value = %v, want 350", got)
	}
}

func TestMergeFamilies_DistinctLabelsKeptSeparate(t *testing.T) {
	agent1 := map[string]*dto.MetricFamily{
		crossAZMetricName: {
			Name: strp(crossAZMetricName), Type: mtypep(),
			Metric: []*dto.Metric{counterMetric(map[string]string{"src_zone": "us-east-1a", "dst_zone": "us-east-1b", "src_namespace": "prod", "src_workload": "web"}, 100)},
		},
	}
	agent2 := map[string]*dto.MetricFamily{
		crossAZMetricName: {
			Name: strp(crossAZMetricName), Type: mtypep(),
			Metric: []*dto.Metric{counterMetric(map[string]string{"src_zone": "us-east-1b", "dst_zone": "us-east-1c", "src_namespace": "prod", "src_workload": "db"}, 200)},
		},
	}

	merged := MergeFamilies([]map[string]*dto.MetricFamily{agent1, agent2})
	if len(merged[crossAZMetricName].Metric) != 2 {
		t.Errorf("merged metric count = %d, want 2 distinct entries", len(merged[crossAZMetricName].Metric))
	}
}
