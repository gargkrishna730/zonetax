package scrape

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDiscoverAgents_SkipsNonRunningAndPortlessPods(t *testing.T) {
	running := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-1", Namespace: "zonetax", Labels: map[string]string{"app.kubernetes.io/component": "agent"}},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{Name: "agent", Ports: []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9090}}},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.1.5"},
	}
	pending := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-2", Namespace: "zonetax", Labels: map[string]string{"app.kubernetes.io/component": "agent"}},
		Spec: corev1.PodSpec{
			NodeName: "node-2",
			Containers: []corev1.Container{
				{Name: "agent", Ports: []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9090}}},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	noPort := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-3", Namespace: "zonetax", Labels: map[string]string{"app.kubernetes.io/component": "agent"}},
		Spec: corev1.PodSpec{
			NodeName:   "node-3",
			Containers: []corev1.Container{{Name: "agent"}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.1.7"},
	}

	clientset := fake.NewSimpleClientset(running, pending, noPort)
	targets, err := DiscoverAgents(context.Background(), clientset, "zonetax", "app.kubernetes.io/component=agent")
	if err != nil {
		t.Fatalf("DiscoverAgents() error = %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1 (only the running pod with a metrics port)", len(targets))
	}
	if targets[0].PodName != "agent-1" || targets[0].Port != 9090 {
		t.Errorf("target = %+v, want agent-1 on port 9090", targets[0])
	}
}

func TestTarget_URL(t *testing.T) {
	tgt := Target{IP: "10.0.1.5", Port: 9090}
	want := "http://10.0.1.5:9090/metrics"
	if got := tgt.URL(); got != want {
		t.Errorf("URL() = %q, want %q", got, want)
	}
}
