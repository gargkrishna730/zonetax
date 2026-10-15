package main

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func awsNode(name, zone, region string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"topology.kubernetes.io/zone":   zone,
				"topology.kubernetes.io/region": region,
			},
		},
		Spec: corev1.NodeSpec{ProviderID: "aws:///" + zone + "/i-" + name},
	}
}

// The reported bug: installing into us-west-2 must not report us-east-1.
func TestResolveCloudRegionAutoDetects(t *testing.T) {
	t.Setenv("CLOUD_PROVIDER", "")
	t.Setenv("CLOUD_REGION", "")

	cs := fake.NewSimpleClientset(
		awsNode("a", "us-west-2a", "us-west-2"),
		awsNode("b", "us-west-2b", "us-west-2"),
	)
	cloud, region := resolveCloudRegion(context.Background(), cs)
	if cloud != "aws" || region != "us-west-2" {
		t.Errorf("got cloud=%q region=%q, want aws/us-west-2", cloud, region)
	}
}

// An explicit env var must still win, for clusters detection can't handle.
func TestResolveCloudRegionEnvOverrides(t *testing.T) {
	t.Setenv("CLOUD_REGION", "eu-central-1")

	cs := fake.NewSimpleClientset(awsNode("a", "us-west-2a", "us-west-2"))
	_, region := resolveCloudRegion(context.Background(), cs)
	if region != "eu-central-1" {
		t.Errorf("region = %q, want eu-central-1 (env override)", region)
	}
}

// With no labels and no env, we must NOT invent a region. Empty region means the API/UI
// reports it as unknown and the cloud-level default rate applies.
func TestResolveCloudRegionUnknownStaysEmpty(t *testing.T) {
	t.Setenv("CLOUD_PROVIDER", "")
	t.Setenv("CLOUD_REGION", "")

	cs := fake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "bare"},
	})
	cloud, region := resolveCloudRegion(context.Background(), cs)
	if region != "" {
		t.Errorf("region = %q, want empty rather than a fabricated region", region)
	}
	if cloud != "aws" {
		t.Errorf("cloud = %q, want the aws fallback so pricing still resolves", cloud)
	}
}
