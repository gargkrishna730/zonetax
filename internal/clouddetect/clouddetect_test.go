package clouddetect

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/gargkrishna730/zonetax/internal/azmap"
)

func TestCloudFromProviderID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"aws:///us-east-1a/i-0e552859f3ddacbaa", "aws"},
		{"gce://my-project/us-central1-a/gke-node-1", "gcp"},
		{"azure:///subscriptions/x/resourceGroups/y/providers/Microsoft.Compute/virtualMachines/z", "azure"},
		{"", ""},
		{"no-scheme", ""},
	}
	for _, c := range cases {
		if got := CloudFromProviderID(c.in); got != c.want {
			t.Errorf("CloudFromProviderID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRegionFromZone(t *testing.T) {
	cases := []struct{ in, want string }{
		// AWS style: trailing letter directly after a digit.
		{"us-west-2a", "us-west-2"},
		{"us-east-1c", "us-east-1"},
		{"ap-south-1b", "ap-south-1"},
		// GCP style: zone suffix separated by a hyphen.
		{"us-central1-a", "us-central1"},
		{"europe-west4-b", "europe-west4"},
		// Not zone-like: must not fabricate a region.
		{"", ""},
		{"onprem", ""},
		{"us-west-2", ""},
	}
	for _, c := range cases {
		if got := RegionFromZone(c.in); got != c.want {
			t.Errorf("RegionFromZone(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func node(name, zone, region, providerID string) *corev1.Node {
	labels := map[string]string{}
	if zone != "" {
		labels[azmap.ZoneLabel] = zone
	}
	if region != "" {
		labels[azmap.RegionLabel] = region
	}
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec:       corev1.NodeSpec{ProviderID: providerID},
	}
}

// The regression this package exists for: a cluster in us-west-2 must report us-west-2,
// never a hard-coded default.
func TestDetectUsWest2(t *testing.T) {
	cs := fake.NewSimpleClientset(
		node("n1", "us-west-2a", "us-west-2", "aws:///us-west-2a/i-1"),
		node("n2", "us-west-2b", "us-west-2", "aws:///us-west-2b/i-2"),
		node("n3", "us-west-2c", "us-west-2", "aws:///us-west-2c/i-3"),
	)
	got, err := Detect(context.Background(), cs)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got.Region != "us-west-2" {
		t.Errorf("Region = %q, want us-west-2", got.Region)
	}
	if got.Cloud != "aws" {
		t.Errorf("Cloud = %q, want aws", got.Cloud)
	}
	if len(got.Zones) != 3 {
		t.Errorf("Zones = %v, want 3 zones", got.Zones)
	}
	if got.NodesSeen != 3 {
		t.Errorf("NodesSeen = %d, want 3", got.NodesSeen)
	}
}

// Region label missing (some self-managed clusters): fall back to deriving it from the zone.
func TestDetectRegionFromZoneFallback(t *testing.T) {
	cs := fake.NewSimpleClientset(
		node("n1", "eu-west-1a", "", "aws:///eu-west-1a/i-1"),
		node("n2", "eu-west-1b", "", "aws:///eu-west-1b/i-2"),
	)
	got, err := Detect(context.Background(), cs)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got.Region != "eu-west-1" {
		t.Errorf("Region = %q, want eu-west-1", got.Region)
	}
}

func TestDetectGKE(t *testing.T) {
	cs := fake.NewSimpleClientset(
		node("n1", "us-central1-a", "us-central1", "gce://proj/us-central1-a/gke-1"),
		node("n2", "us-central1-b", "us-central1", "gce://proj/us-central1-b/gke-2"),
	)
	got, err := Detect(context.Background(), cs)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got.Cloud != "gcp" || got.Region != "us-central1" {
		t.Errorf("got cloud=%q region=%q, want gcp/us-central1", got.Cloud, got.Region)
	}
}

// A majority of nodes decides, so one odd node can't move the whole cluster's pricing.
func TestDetectMajorityWins(t *testing.T) {
	cs := fake.NewSimpleClientset(
		node("n1", "us-west-2a", "us-west-2", "aws:///us-west-2a/i-1"),
		node("n2", "us-west-2b", "us-west-2", "aws:///us-west-2b/i-2"),
		node("odd", "us-east-1a", "us-east-1", "aws:///us-east-1a/i-3"),
	)
	got, err := Detect(context.Background(), cs)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got.Region != "us-west-2" {
		t.Errorf("Region = %q, want us-west-2 (majority)", got.Region)
	}
}

// Bare-metal / kind: no labels at all. Must error rather than guess a region.
func TestDetectNoCloudInfo(t *testing.T) {
	cs := fake.NewSimpleClientset(node("n1", "", "", ""))
	if _, err := Detect(context.Background(), cs); err == nil {
		t.Error("expected an error when no cloud/region could be inferred")
	}
}

func TestDetectNoNodes(t *testing.T) {
	if _, err := Detect(context.Background(), fake.NewSimpleClientset()); err == nil {
		t.Error("expected an error when the cluster has no nodes")
	}
}

// Single-zone clusters cannot incur cross-AZ cost; Detect should still report the zone set
// so callers can say so explicitly.
func TestDetectSingleZone(t *testing.T) {
	cs := fake.NewSimpleClientset(
		node("n1", "us-west-2a", "us-west-2", "aws:///us-west-2a/i-1"),
		node("n2", "us-west-2a", "us-west-2", "aws:///us-west-2a/i-2"),
	)
	got, err := Detect(context.Background(), cs)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got.Zones) != 1 || got.Zones[0] != "us-west-2a" {
		t.Errorf("Zones = %v, want exactly [us-west-2a]", got.Zones)
	}
}
