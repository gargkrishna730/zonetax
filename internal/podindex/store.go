// Package podindex maintains an in-memory index of pod IP -> pod/workload/node, and node ->
// AZ/region, so the agent can attribute conntrack flows to Kubernetes workloads without a
// per-flow API call. Store holds the pure in-memory state (easily unit tested); informer.go
// wires it up to a live cluster via client-go.
package podindex

import (
	"sync"

	"github.com/gargkrishna730/zonetax/internal/azmap"
)

// PodInfo describes a pod's identity and placement, as needed to attribute network flows.
type PodInfo struct {
	Namespace string
	Name      string
	// Workload is the best-effort owning workload name (Deployment/StatefulSet/DaemonSet/etc),
	// falling back to the pod name itself for bare/unowned pods. See workloadName() in
	// informer.go for the resolution heuristic.
	Workload string
	NodeName string
}

// Store is a thread-safe, in-memory index of pod IP -> PodInfo and node name -> AZ/region.
type Store struct {
	mu       sync.RWMutex
	podsByIP map[string]PodInfo
	nodes    map[string]azmap.NodeInfo
}

// NewStore returns an empty Store.
func NewStore() *Store {
	return &Store{
		podsByIP: make(map[string]PodInfo),
		nodes:    make(map[string]azmap.NodeInfo),
	}
}

// UpsertPod records or updates a pod's IP -> info mapping. A no-op if ip is empty.
func (s *Store) UpsertPod(ip string, info PodInfo) {
	if ip == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.podsByIP[ip] = info
}

// DeletePodByIP removes a pod's IP mapping (e.g. on pod deletion/eviction).
func (s *Store) DeletePodByIP(ip string) {
	if ip == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.podsByIP, ip)
}

// UpsertNode records or updates a node's AZ/region info.
func (s *Store) UpsertNode(info azmap.NodeInfo) {
	if info.Name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[info.Name] = info
}

// DeleteNode removes a node's AZ/region info.
func (s *Store) DeleteNode(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nodes, name)
}

// Lookup resolves an IP to its PodInfo and the AZ/region info of the node it's running on. ok is
// false if the IP is unknown, or if the pod's node hasn't been indexed yet (in which case
// PodInfo is still returned so callers can distinguish "unknown IP" from "unknown node").
func (s *Store) Lookup(ip string) (PodInfo, azmap.NodeInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pod, ok := s.podsByIP[ip]
	if !ok {
		return PodInfo{}, azmap.NodeInfo{}, false
	}
	node, ok := s.nodes[pod.NodeName]
	if !ok {
		return pod, azmap.NodeInfo{}, false
	}
	return pod, node, true
}

// Len returns the current number of indexed pods and nodes, for logging/diagnostics.
func (s *Store) Len() (pods int, nodes int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.podsByIP), len(s.nodes)
}
