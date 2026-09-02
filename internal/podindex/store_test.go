package podindex

import (
	"testing"

	"github.com/gargkrishna730/zonetax/internal/azmap"
)

func TestStore_UpsertAndLookup(t *testing.T) {
	s := NewStore()
	s.UpsertNode(azmap.NodeInfo{Name: "node-1", Zone: "us-east-1a", Region: "us-east-1"})
	s.UpsertPod("10.0.1.5", PodInfo{Namespace: "default", Name: "web-1", Workload: "web", NodeName: "node-1"})

	pod, node, ok := s.Lookup("10.0.1.5")
	if !ok {
		t.Fatal("Lookup() ok = false, want true")
	}
	if pod.Workload != "web" {
		t.Errorf("Workload = %q, want web", pod.Workload)
	}
	if node.Zone != "us-east-1a" {
		t.Errorf("Zone = %q, want us-east-1a", node.Zone)
	}

	pods, nodes := s.Len()
	if pods != 1 || nodes != 1 {
		t.Errorf("Len() = %d, %d, want 1, 1", pods, nodes)
	}
}

func TestStore_LookupUnknownIP(t *testing.T) {
	s := NewStore()
	if _, _, ok := s.Lookup("10.0.0.1"); ok {
		t.Error("Lookup() ok = true for unknown IP, want false")
	}
}

func TestStore_LookupPodWithoutIndexedNode(t *testing.T) {
	s := NewStore()
	s.UpsertPod("10.0.1.5", PodInfo{Namespace: "default", Name: "web-1", NodeName: "node-missing"})
	pod, _, ok := s.Lookup("10.0.1.5")
	if ok {
		t.Error("Lookup() ok = true when node is unindexed, want false")
	}
	if pod.Name != "web-1" {
		t.Errorf("PodInfo still returned as %+v, want Name=web-1", pod)
	}
}

func TestStore_DeletePodAndNode(t *testing.T) {
	s := NewStore()
	s.UpsertNode(azmap.NodeInfo{Name: "node-1", Zone: "us-east-1a"})
	s.UpsertPod("10.0.1.5", PodInfo{NodeName: "node-1"})

	s.DeletePodByIP("10.0.1.5")
	if _, _, ok := s.Lookup("10.0.1.5"); ok {
		t.Error("Lookup() ok = true after DeletePodByIP, want false")
	}

	s.UpsertPod("10.0.1.6", PodInfo{NodeName: "node-1"})
	s.DeleteNode("node-1")
	if _, _, ok := s.Lookup("10.0.1.6"); ok {
		t.Error("Lookup() ok = true after DeleteNode, want false")
	}
}
