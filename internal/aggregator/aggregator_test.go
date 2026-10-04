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

	out := Aggregate(flows, topo.resolve, "")
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
		t.Errorf("source attribution = %s/%s, want prod/web", r.SrcNamespace, r.SrcWorkload)
	}
	if r.DstNamespace != "prod" || r.DstWorkload != "db" {
		t.Errorf("destination attribution = %s/%s, want prod/db", r.DstNamespace, r.DstWorkload)
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

	out := Aggregate(flows, topo.resolve, "")
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

	out := Aggregate(flows, topo.resolve, "")
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

	out := Aggregate(flows, topo.resolve, "")
	for _, r := range out.Results {
		if r.Bytes != 0 {
			t.Fatalf("flow without byte accounting contributed %d bytes", r.Bytes)
		}
	}
	if out.Unresolved != 0 {
		t.Fatalf("flow should resolve, unresolved=%d", out.Unresolved)
	}
}

// Two-node topology used by the attribution tests below.
func twoNode() fakeTopology {
	return fakeTopology{
		pods: map[string]podindex.PodInfo{
			"10.0.1.1": {Namespace: "app", Workload: "client", NodeName: "node-a"},
			"10.0.2.1": {Namespace: "app", Workload: "server", NodeName: "node-b"},
		},
		node: map[string]azmap.NodeInfo{
			"node-a": {Name: "node-a", Zone: "az-a"},
			"node-b": {Name: "node-b", Zone: "az-b"},
		},
	}
}

func bytesFor(out AggregateOutput, src, dst string) int64 {
	var n int64
	for _, r := range out.Results {
		if r.SrcWorkload == src && r.DstWorkload == dst {
			n += r.Bytes
		}
	}
	return n
}

// QA found 1 GiB uploaded cross-AZ reported as 2.16 GB: the connection is in both nodes'
// conntrack tables and both agents counted it. Only the source pod's node may count it.
func TestAggregate_EachConnectionCountedOnceAcrossNodes(t *testing.T) {
	topo := twoNode()
	flow := []conntrack.Flow{{OrigSrcIP: "10.0.1.1", OrigDstIP: "10.0.2.1", ReplySrcIP: "10.0.2.1", ReplyDstIP: "10.0.1.1", OrigBytes: 1000, ReplyBytes: 50}}
	a := Aggregate(flow, topo.resolve, "node-a")
	b := Aggregate(flow, topo.resolve, "node-b")
	if got := bytesFor(a, "client", "server") + bytesFor(b, "client", "server"); got != 1000 {
		t.Errorf("cluster-wide client->server = %d, want 1000 (counted once)", got)
	}
	if len(b.Results) != 0 {
		t.Errorf("destination node must not count the connection, got %+v", b.Results)
	}
}

// QA found a 1 GiB download (server streams to client) reported as 0.003 GB: reply bytes
// were ignored. They must be attributed server -> client.
func TestAggregate_ReplyBytesCountedInReverseDirection(t *testing.T) {
	topo := twoNode()
	flow := []conntrack.Flow{{OrigSrcIP: "10.0.1.1", OrigDstIP: "10.0.2.1", ReplySrcIP: "10.0.2.1", ReplyDstIP: "10.0.1.1", OrigBytes: 200, ReplyBytes: 5000}}
	out := Aggregate(flow, topo.resolve, "node-a")
	if got := bytesFor(out, "server", "client"); got != 5000 {
		t.Errorf("server->client = %d, want 5000", got)
	}
	for _, r := range out.Results {
		if r.SrcWorkload == "server" && (r.SrcZone != "az-b" || r.DstZone != "az-a") {
			t.Errorf("reply zones = %s->%s, want az-b->az-a", r.SrcZone, r.DstZone)
		}
	}
}

// Traffic to a ClusterIP Service: on the source node the original destination is the virtual
// Service IP, and only the reply tuple carries the backend pod IP.
func TestAggregate_ServiceTrafficResolvedViaReplyTuple(t *testing.T) {
	topo := twoNode()
	flow := []conntrack.Flow{{OrigSrcIP: "10.0.1.1", OrigDstIP: "172.20.0.10", ReplySrcIP: "10.0.2.1", ReplyDstIP: "10.0.1.1", OrigBytes: 700, ReplyBytes: 30}}
	out := Aggregate(flow, topo.resolve, "node-a")
	if out.Unresolved != 0 {
		t.Fatalf("service flow should resolve via reply tuple, unresolved=%d", out.Unresolved)
	}
	if got := bytesFor(out, "client", "server"); got != 700 {
		t.Errorf("client->server via service = %d, want 700", got)
	}
}
