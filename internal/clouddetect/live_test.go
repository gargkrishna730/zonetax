//go:build livecluster

// Live-cluster verification of cloud/region detection. Not part of the normal test run:
// requires a real kubeconfig. Run with:
//
//	go test -tags livecluster ./internal/clouddetect/ -run TestLive -v -args <kube-context>
package clouddetect

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func TestLiveClusterDetect(t *testing.T) {
	kubeContext := ""
	if args := os.Args; len(args) > 0 {
		kubeContext = args[len(args)-1]
	}
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules, &clientcmd.ConfigOverrides{CurrentContext: kubeContext}).ClientConfig()
	if err != nil {
		t.Fatalf("build client config for context %q: %v", kubeContext, err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("new clientset: %v", err)
	}

	res, err := Detect(context.Background(), cs)
	if err != nil {
		t.Fatalf("Detect against live cluster: %v", err)
	}
	t.Logf("context=%s cloud=%q region=%q nodes=%d zones=%v",
		kubeContext, res.Cloud, res.Region, res.NodesSeen, res.Zones)

	if res.Region == "" {
		t.Error("detected an empty region against a live cloud cluster")
	}
	if res.NodesSeen == 0 {
		t.Error("saw no nodes")
	}
}
