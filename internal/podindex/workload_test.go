package podindex

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkloadName(t *testing.T) {
	podWithOwner := func(kind, name string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "irrelevant-pod-name",
				OwnerReferences: []metav1.OwnerReference{{Kind: kind, Name: name}},
			},
		}
	}

	tests := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{
			name: "unowned pod falls back to pod name",
			pod:  &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "standalone-pod"}},
			want: "standalone-pod",
		},
		{
			name: "replicaset with hash suffix resolves to deployment name",
			pod:  podWithOwner("ReplicaSet", "web-7d9f8c6b5"),
			want: "web",
		},
		{
			name: "replicaset without a matching hash suffix falls back to raw name",
			pod:  podWithOwner("ReplicaSet", "web"),
			want: "web",
		},
		{
			name: "statefulset owner name used directly",
			pod:  podWithOwner("StatefulSet", "cache"),
			want: "cache",
		},
		{
			name: "daemonset owner name used directly",
			pod:  podWithOwner("DaemonSet", "zonetax-agent"),
			want: "zonetax-agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workloadName(tt.pod); got != tt.want {
				t.Errorf("workloadName() = %q, want %q", got, tt.want)
			}
		})
	}
}
