// Package clouddetect infers the cloud provider and region the cluster runs in from the
// standard Kubernetes node labels and the node's spec.providerID, so operators don't have to
// hand-configure CLOUD_REGION (and silently get priced against the wrong region if they forget).
//
// Why this exists: the collector previously defaulted to a hard-coded region. An install in any
// other region kept reporting that default, which meant the pricing table was consulted for the
// wrong region and the dashboard displayed a region the cluster wasn't in.
package clouddetect

import (
	"context"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/gargkrishna730/zonetax/internal/azmap"
)

// Result is what we could infer about where this cluster runs.
type Result struct {
	// Cloud is the provider key used to look up the pricing table ("aws", "gcp", ...).
	Cloud string
	// Region is the cloud region, e.g. "us-west-2".
	Region string
	// Zones are the distinct zones observed across nodes, sorted. Informational: a single-zone
	// cluster can't incur cross-AZ cost at all, which is worth surfacing to the operator.
	Zones []string
	// NodesSeen is how many nodes contributed to this result.
	NodesSeen int
}

// CloudFromProviderID maps a node's spec.providerID prefix to a pricing-table cloud key.
// Format is "<provider>://<provider-specific-id>", e.g. "aws:///us-east-1a/i-0abc".
func CloudFromProviderID(providerID string) string {
	scheme, _, found := strings.Cut(providerID, "://")
	if !found {
		return ""
	}
	switch strings.ToLower(scheme) {
	case "aws":
		return "aws"
	case "gce":
		return "gcp"
	case "azure":
		return "azure"
	default:
		// Unknown/other providers (openstack, vsphere, kind, ...) have no cross-AZ pricing
		// model here; returning "" lets the caller fall back rather than invent a price.
		return strings.ToLower(scheme)
	}
}

// RegionFromZone derives a region from a zone label when the region label is absent, by
// stripping the trailing zone suffix: "us-west-2a" -> "us-west-2", "us-central1-a" -> "us-central1".
// Returns "" when the zone doesn't look like a cloud zone, so callers don't fabricate a region.
func RegionFromZone(zone string) string {
	if zone == "" {
		return ""
	}
	// GCP style: us-central1-a — a single trailing letter after a final hyphen, where the
	// preceding segment ends in a digit ("us-central1"). The digit check matters: without it
	// an already-region value like "us-west-2" would be truncated to "us-west".
	if i := strings.LastIndex(zone, "-"); i > 0 && len(zone)-i == 2 {
		suffix := zone[i+1]
		head := zone[:i]
		if suffix >= 'a' && suffix <= 'z' && head[len(head)-1] >= '0' && head[len(head)-1] <= '9' {
			return head
		}
	}
	// AWS style: us-west-2a (single trailing letter, no separator).
	last := zone[len(zone)-1]
	if last >= 'a' && last <= 'z' {
		trimmed := zone[:len(zone)-1]
		// Only treat it as a zone suffix if what remains ends in a digit (us-west-2).
		if n := len(trimmed); n > 0 && trimmed[n-1] >= '0' && trimmed[n-1] <= '9' {
			return trimmed
		}
	}
	return ""
}

// Detect lists cluster nodes and infers cloud/region/zones from their labels and providerIDs.
// It returns an error only when nothing could be determined, so the caller can decide whether
// to fall back to explicit configuration.
func Detect(ctx context.Context, clientset kubernetes.Interface) (Result, error) {
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Result{}, fmt.Errorf("list nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return Result{}, fmt.Errorf("no nodes found")
	}

	// Count observations so a single mislabelled or edge node can't override the cluster.
	regionVotes := map[string]int{}
	cloudVotes := map[string]int{}
	zoneSet := map[string]struct{}{}

	for i := range nodes.Items {
		n := &nodes.Items[i]

		zone := n.Labels[azmap.ZoneLabel]
		if zone != "" {
			zoneSet[zone] = struct{}{}
		}

		region := n.Labels[azmap.RegionLabel]
		if region == "" {
			region = RegionFromZone(zone)
		}
		if region != "" {
			regionVotes[region]++
		}

		if c := CloudFromProviderID(n.Spec.ProviderID); c != "" {
			cloudVotes[c]++
		}
	}

	res := Result{NodesSeen: len(nodes.Items)}
	res.Region = topVote(regionVotes)
	res.Cloud = topVote(cloudVotes)
	for z := range zoneSet {
		res.Zones = append(res.Zones, z)
	}
	sort.Strings(res.Zones)

	if res.Region == "" && res.Cloud == "" {
		return res, fmt.Errorf("could not infer cloud or region from %d node(s)", len(nodes.Items))
	}
	return res, nil
}

// topVote returns the key with the most votes, breaking ties deterministically by name so
// repeated runs on the same cluster don't flap between equally-voted values.
func topVote(votes map[string]int) string {
	best, bestN := "", 0
	keys := make([]string, 0, len(votes))
	for k := range votes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if votes[k] > bestN {
			best, bestN = k, votes[k]
		}
	}
	return best
}
