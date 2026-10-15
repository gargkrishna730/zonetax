//go:build livecluster

package clouddetect

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Proves detection genuinely reads the cluster rather than falling back to a baked-in default:
// the detected value must match what the nodes actually say, whatever that is. Run against a
// cluster whose region is NOT the old hard-coded "us-east-1" to fully disambiguate.
func TestLiveDetectMatchesNodeLabels(t *testing.T) {
	kubeContext := os.Args[len(os.Args)-1]
	rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: filepath.Join(os.Getenv("HOME"), ".kube", "config")}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules, &clientcmd.ConfigOverrides{CurrentContext: kubeContext}).ClientConfig()
	if err != nil {
		t.Fatalf("client config: %v", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}

	res, err := Detect(context.Background(), cs)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	// Independently read the labels and compare, so a hard-coded return value would fail here.
	nodes, err := cs.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	want := nodes.Items[0].Labels["topology.kubernetes.io/region"]
	if res.Region != want {
		t.Errorf("Detect returned %q but node labels say %q", res.Region, want)
	}
	t.Logf("detected %q == node label %q (%d nodes)", res.Region, want, res.NodesSeen)
}
