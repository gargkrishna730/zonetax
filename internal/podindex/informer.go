package podindex

import (
	"context"
	"fmt"
	"regexp"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/gargkrishna730/zonetax/internal/azmap"
)

// replicaSetHashSuffix matches the pod-template-hash suffix Kubernetes appends to ReplicaSet
// names (e.g. "web-7d9f8c6b5" -> "web"), so we can approximate the owning Deployment's name
// without an extra API call. This is a heuristic, not a guarantee — unusual naming can defeat
// it, in which case the raw ReplicaSet name is used instead.
var replicaSetHashSuffix = regexp.MustCompile(`^(.+)-[a-z0-9]{8,10}$`)

// workloadName returns the best-effort owning workload name for a pod: the Deployment name (via
// the ReplicaSet-name heuristic above) for Deployment-managed pods, the direct owner name for
// other controllers (StatefulSet, DaemonSet, Job, ...), or the pod's own name if unowned.
func workloadName(pod *corev1.Pod) string {
	if len(pod.OwnerReferences) == 0 {
		return pod.Name
	}
	owner := pod.OwnerReferences[0]
	if owner.Kind == "ReplicaSet" {
		if m := replicaSetHashSuffix.FindStringSubmatch(owner.Name); len(m) == 2 {
			return m[1]
		}
	}
	return owner.Name
}

// NewInformerStore builds a Store kept in sync with the cluster via client-go informers for
// Pods and Nodes (cluster-scoped — RBAC must grant get/list/watch on both, see the Helm chart).
// It blocks until the initial cache sync completes or ctx is cancelled.
func NewInformerStore(ctx context.Context, clientset kubernetes.Interface, resync time.Duration) (*Store, error) {
	store := NewStore()
	factory := informers.NewSharedInformerFactory(clientset, resync)

	nodeInformer := factory.Core().V1().Nodes().Informer()
	_, err := nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { upsertNode(store, obj) },
		UpdateFunc: func(_, obj interface{}) { upsertNode(store, obj) },
		DeleteFunc: func(obj interface{}) {
			if node := asNode(obj); node != nil {
				store.DeleteNode(node.Name)
			}
		},
	})
	if err != nil {
		return nil, fmt.Errorf("podindex: register node handler: %w", err)
	}

	podInformer := factory.Core().V1().Pods().Informer()
	_, err = podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { upsertPod(store, obj) },
		UpdateFunc: func(_, obj interface{}) { upsertPod(store, obj) },
		DeleteFunc: func(obj interface{}) {
			if pod := asPod(obj); pod != nil && pod.Status.PodIP != "" {
				store.DeletePodByIP(pod.Status.PodIP)
			}
		},
	})
	if err != nil {
		return nil, fmt.Errorf("podindex: register pod handler: %w", err)
	}

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), nodeInformer.HasSynced, podInformer.HasSynced) {
		return nil, fmt.Errorf("podindex: informer caches did not sync before context was done")
	}
	return store, nil
}

func upsertNode(store *Store, obj interface{}) {
	node := asNode(obj)
	if node == nil {
		return
	}
	store.UpsertNode(azmap.NodeInfo{
		Name:   node.Name,
		Zone:   node.Labels[azmap.ZoneLabel],
		Region: node.Labels[azmap.RegionLabel],
	})
}

func upsertPod(store *Store, obj interface{}) {
	pod := asPod(obj)
	if pod == nil || pod.Status.PodIP == "" || pod.Spec.NodeName == "" {
		return
	}
	store.UpsertPod(pod.Status.PodIP, PodInfo{
		Namespace: pod.Namespace,
		Name:      pod.Name,
		Workload:  workloadName(pod),
		NodeName:  pod.Spec.NodeName,
	})
}

// asNode/asPod unwrap informer event objects, including the DeletedFinalStateUnknown wrapper
// client-go uses when it misses a delete event and has to reconstruct it from the last known
// cached state.
func asNode(obj interface{}) *corev1.Node {
	if n, ok := obj.(*corev1.Node); ok {
		return n
	}
	if tomb, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		if n, ok := tomb.Obj.(*corev1.Node); ok {
			return n
		}
	}
	return nil
}

func asPod(obj interface{}) *corev1.Pod {
	if p, ok := obj.(*corev1.Pod); ok {
		return p
	}
	if tomb, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		if p, ok := tomb.Obj.(*corev1.Pod); ok {
			return p
		}
	}
	return nil
}
