package aggregator

import (
	"testing"

	"github.com/gargkrishna730/zonetax/internal/azmap"
	"github.com/gargkrishna730/zonetax/internal/conntrack"
	"github.com/gargkrishna730/zonetax/internal/podindex"
)

// fakeTopology backs a ResolveFunc for tests, mapping IP -> (pod, node, ok) directly.
type fakeTopology struct {
	pods map[string]podindex.PodInfo
	node map[string]azmap.NodeInfo // keyed by NodeName
}

func (f fakeTopology) resolve(ip string) (podindex.PodInfo, azmap.NodeInfo, bool) {
	pod, ok := f.pods[ip]
	if !ok {
		return podindex.PodInfo{}, azmap.NodeInfo{}, false
	}
	node, ok := f.node[pod.NodeName]
	if !ok {
		return pod, azmap.NodeInfo{}, false
	}
	return pod, node, true
}

func TestAggregate_CrossAZTrafficSummed(t *testing.T) {
	topo := fakeTopology{
		pods: map[string]podindex.PodInfo{
			"10.0.1.1": {Namespace: "prod", Workload: "web", NodeName: "node-a"},
			"10.0.2.1": {Namespace: "prod", Workload: "db", NodeName: "node-b"},
		},
		node: map[string]azmap.NodeInfo{
			"node-a": {Name: "node-a", Zone: "us-east-1a"},
			"node-b": {Name: "node-b", Zone: "us-east-1b"},
		},
	}
	flows := []conntrack.Flow{
		{OrigSrcIP: "10.0.1.1", OrigDstIP: "10.0.2.1", OrigBytes: 1000},
		{OrigSrcIP: "10.0.1.1", OrigDstIP: "10.0.2.1", OrigBytes: 500},
	}

	out := Aggregate(flows, topo.resolve)
	if len(out.Results) != 1 {
		t.Fatalf("Aggregate() returned %d results, want 1; got %+v", len(out.Results), out.Results)
	}
	r := out.Results[0]
	if r.Bytes != 1500 {
		t.Errorf("Bytes = %d, want 1500", r.Bytes)
	}
	if !r.CrossAZ() {
		t.Error("CrossAZ() = false, want true")
	}
	if r.SrcZone != "us-east-1a" || r.DstZone != "us-east-1b" {
		t.Errorf("zones = %s -> %s, want us-east-1a -> us-east-1b", r.SrcZone, r.DstZone)
	}
	if r.SrcNamespace != "prod" || r.SrcWorkload != "web" {
		t.Errorf("attribution = %s/%s, want prod/web", r.SrcNamespace, r.SrcWorkload)
	}
}

func TestAggregate_SameAZNotFlaggedCrossAZ(t *testing.T) {
	topo := fakeTopology{
		pods: map[string]podindex.PodInfo{
			"10.0.1.1": {NodeName: "node-a"},
			"10.0.1.2": {NodeName: "node-a2"},
		},
		node: map[string]azmap.NodeInfo{
			"node-a":  {Name: "node-a", Zone: "us-east-1a"},
			"node-a2": {Name: "node-a2", Zone: "us-east-1a"},
		},
	}
	flows := []conntrack.Flow{{OrigSrcIP: "10.0.1.1", OrigDstIP: "10.0.1.2", OrigBytes: 100}}

	out := Aggregate(flows, topo.resolve)
	if len(out.Results) != 1 {
		t.Fatalf("Aggregate() returned %d results, want 1", len(out.Results))
	}
	if out.Results[0].CrossAZ() {
		t.Error("CrossAZ() = true for same-zone flow, want false")
	}
}

func TestAggregate_UnresolvableIPsSkipped(t *testing.T) {
	topo := fakeTopology{
		pods: map[string]podindex.PodInfo{
			"10.0.1.1": {NodeName: "node-a"},
		},
		node: map[string]azmap.NodeInfo{
			"node-a": {Name: "node-a", Zone: "us-east-1a"},
		},
	}
	// Destination IP is outside the cluster / not indexed.
	flows := []conntrack.Flow{{OrigSrcIP: "10.0.1.1", OrigDstIP: "8.8.8.8", OrigBytes: 100}}

	out := Aggregate(flows, topo.resolve)
	if len(out.Results) != 0 {
		t.Errorf("Aggregate() returned %d results, want 0 (unresolvable dst should be skipped)", len(out.Results))
	}
	if out.Unresolved != 1 {
		t.Errorf("Unresolved = %d, want 1", out.Unresolved)
	}
}

func TestAggregate_FlowWithoutByteAccountingCountsZero(t *testing.T) {
	topo := fakeTopology{
		pods: map[string]podindex.PodInfo{
			"10.0.1.1": {NodeName: "node-a"},
			"10.0.2.1": {NodeName: "node-b"},
		},
		node: map[string]azmap.NodeInfo{
			"node-a": {Name: "node-a", Zone: "us-east-1a"},
			"node-b": {Name: "node-b", Zone: "us-east-1b"},
		},
	}
	// OrigBytes left at zero value (0), simulating no accounting data.
	flows := []conntrack.Flow{{OrigSrcIP: "10.0.1.1", OrigDstIP: "10.0.2.1", OrigBytes: 0}}

	out := Aggregate(flows, topo.resolve)
	if len(out.Results) != 1 || out.Results[0].Bytes != 0 {
		t.Fatalf("Aggregate() = %+v, want 1 result with Bytes=0", out.Results)
	}
}
