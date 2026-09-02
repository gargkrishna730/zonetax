// Package azmap resolves node -> availability zone using standard Kubernetes topology labels,
// so the agent and collector don't need cloud-specific IMDS calls or credentials.
package azmap

const (
	// ZoneLabel is the standard Kubernetes node label populated by cloud-controller-manager.
	ZoneLabel = "topology.kubernetes.io/zone"
	// RegionLabel is the standard Kubernetes node label for the node's region.
	RegionLabel = "topology.kubernetes.io/region"
)

// NodeInfo captures the zone/region placement of a single node, keyed by node name.
type NodeInfo struct {
	Name   string
	Zone   string
	Region string
}

// Resolver maps node names to their AZ/region, backed by a k8s informer cache in production
// (see cmd/agent) but kept as a plain interface here so it's trivially testable.
type Resolver interface {
	NodeInfo(nodeName string) (NodeInfo, bool)
}

// StaticResolver is a simple in-memory Resolver, useful for tests and for the MVP before the
// full client-go informer wiring lands in M1.
type StaticResolver struct {
	nodes map[string]NodeInfo
}

// NewStaticResolver builds a StaticResolver from a slice of NodeInfo.
func NewStaticResolver(nodes []NodeInfo) *StaticResolver {
	m := make(map[string]NodeInfo, len(nodes))
	for _, n := range nodes {
		m[n.Name] = n
	}
	return &StaticResolver{nodes: m}
}

// NodeInfo returns the zone/region info for a node, or false if unknown.
func (s *StaticResolver) NodeInfo(nodeName string) (NodeInfo, bool) {
	n, ok := s.nodes[nodeName]
	return n, ok
}

// IsCrossAZ reports whether two nodes are in different availability zones. Nodes with unknown
// or empty zones are treated as same-zone (i.e. not flagged) to avoid false positives from
// missing label data.
func IsCrossAZ(a, b NodeInfo) bool {
	if a.Zone == "" || b.Zone == "" {
		return false
	}
	return a.Zone != b.Zone
}
