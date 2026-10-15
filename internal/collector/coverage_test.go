package collector

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/gargkrishna730/zonetax/internal/scrape"
)

func plainNode(name string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func TestComputeCoverageAllNodesMeasured(t *testing.T) {
	cs := fake.NewSimpleClientset(plainNode("n1"), plainNode("n2"))
	targets := []scrape.Target{{NodeName: "n1"}, {NodeName: "n2"}}
	results := []scrape.Result{
		{Target: scrape.Target{NodeName: "n1"}},
		{Target: scrape.Target{NodeName: "n2"}},
	}

	cov := computeCoverage(context.Background(), cs, targets, results)
	if !cov.Complete() {
		t.Errorf("expected complete coverage, got %+v", cov)
	}
	if cov.NodesTotal != 2 || cov.AgentsScraped != 2 {
		t.Errorf("got %+v, want 2 nodes / 2 scraped", cov)
	}
	if len(cov.UnmeasuredNodes) != 0 {
		t.Errorf("unmeasured = %v, want none", cov.UnmeasuredNodes)
	}
}

// The real-cluster case: an agent stuck Pending on a memory-full node means that node is never
// measured. That must be reported, not silently treated as zero traffic.
func TestComputeCoveragePendingAgentIsReported(t *testing.T) {
	cs := fake.NewSimpleClientset(plainNode("n1"), plainNode("n2"), plainNode("full-node"))
	// Only two agents are Running, so DiscoverAgents never returned a target for full-node.
	targets := []scrape.Target{{NodeName: "n1"}, {NodeName: "n2"}}
	results := []scrape.Result{
		{Target: scrape.Target{NodeName: "n1"}},
		{Target: scrape.Target{NodeName: "n2"}},
	}

	cov := computeCoverage(context.Background(), cs, targets, results)
	if cov.Complete() {
		t.Error("coverage must not be reported complete when a node has no agent")
	}
	if cov.NodesTotal != 3 {
		t.Errorf("NodesTotal = %d, want 3", cov.NodesTotal)
	}
	if len(cov.UnmeasuredNodes) != 1 || cov.UnmeasuredNodes[0] != "full-node" {
		t.Errorf("UnmeasuredNodes = %v, want [full-node]", cov.UnmeasuredNodes)
	}
}

// A scrape that errors is a measurement gap too, even though the agent pod exists.
func TestComputeCoverageFailedScrapeCounted(t *testing.T) {
	cs := fake.NewSimpleClientset(plainNode("n1"), plainNode("n2"))
	targets := []scrape.Target{{NodeName: "n1"}, {NodeName: "n2"}}
	results := []scrape.Result{
		{Target: scrape.Target{NodeName: "n1"}},
		{Target: scrape.Target{NodeName: "n2"}, Err: context.DeadlineExceeded},
	}

	cov := computeCoverage(context.Background(), cs, targets, results)
	if cov.AgentsFailed != 1 {
		t.Errorf("AgentsFailed = %d, want 1", cov.AgentsFailed)
	}
	if cov.Complete() {
		t.Error("coverage must not be complete when a scrape failed")
	}
	if len(cov.UnmeasuredNodes) != 1 || cov.UnmeasuredNodes[0] != "n2" {
		t.Errorf("UnmeasuredNodes = %v, want [n2]", cov.UnmeasuredNodes)
	}
}

// Empty cluster/no data must not claim completeness.
func TestComputeCoverageNoAgents(t *testing.T) {
	cs := fake.NewSimpleClientset(plainNode("n1"))
	cov := computeCoverage(context.Background(), cs, nil, nil)
	if cov.Complete() {
		t.Error("coverage with zero agents must not be complete")
	}
	if len(cov.UnmeasuredNodes) != 1 {
		t.Errorf("UnmeasuredNodes = %v, want [n1]", cov.UnmeasuredNodes)
	}
}
