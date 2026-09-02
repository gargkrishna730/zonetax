package podindex

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/gargkrishna730/zonetax/internal/azmap"
)

func TestNewInformerStore_SyncsPodsAndNodes(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				azmap.ZoneLabel:   "us-east-1a",
				azmap.RegionLabel: "us-east-1",
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-abc123",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "web-7d9f8c6b5"},
			},
		},
		Spec:   corev1.PodSpec{NodeName: "node-1"},
		Status: corev1.PodStatus{PodIP: "10.0.1.5"},
	}
	clientset := fake.NewSimpleClientset(node, pod)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := NewInformerStore(ctx, clientset, 0)
	if err != nil {
		t.Fatalf("NewInformerStore() error = %v", err)
	}

	podInfo, nodeInfo, ok := store.Lookup("10.0.1.5")
	if !ok {
		t.Fatal("Lookup(10.0.1.5) ok = false, want true")
	}
	if podInfo.Namespace != "default" || podInfo.Workload != "web" {
		t.Errorf("PodInfo = %+v, want Namespace=default Workload=web", podInfo)
	}
	if nodeInfo.Zone != "us-east-1a" || nodeInfo.Region != "us-east-1" {
		t.Errorf("NodeInfo = %+v, want Zone=us-east-1a Region=us-east-1", nodeInfo)
	}

	pods, nodes := store.Len()
	if pods != 1 || nodes != 1 {
		t.Errorf("Len() = %d, %d, want 1, 1", pods, nodes)
	}
}
