// Package aggregator turns raw conntrack Flows into cross-AZ byte totals, attributing each flow
// to the pods/AZs at both ends via a podindex.Store lookup.
package aggregator

import (
	"github.com/gargkrishna730/zonetax/internal/azmap"
	"github.com/gargkrishna730/zonetax/internal/conntrack"
	"github.com/gargkrishna730/zonetax/internal/podindex"
)

// ResolveFunc looks up an IP's pod and node/AZ info, matching podindex.Store.Lookup's signature.
// Kept as a function type (rather than requiring a full Resolver interface) so callers can pass
// *podindex.Store.Lookup directly or a lightweight test fake with no adapter boilerplate.
type ResolveFunc func(ip string) (podindex.PodInfo, azmap.NodeInfo, bool)

// Key identifies one aggregation bucket: traffic from one AZ to another, broken down by the
// source namespace/workload responsible for it. Same-AZ traffic (SrcZone == DstZone) is also
// tracked so totals/ratios can be reported, but only cross-AZ buckets are billed.
type Key struct {
	SrcZone      string
	DstZone      string
	SrcNamespace string
	SrcWorkload  string
}

// CrossAZ reports whether this bucket represents billable cross-AZ traffic.
func (k Key) CrossAZ() bool {
	return k.SrcZone != "" && k.DstZone != "" && k.SrcZone != k.DstZone
}

// Result is the aggregated byte total for one Key over a sampling window.
type Result struct {
	Key
	Bytes int64
}

// AggregateOutput bundles the aggregated per-AZ-pair results with a count of flows that had to
// be skipped because one or both endpoints couldn't be resolved to a known pod.
type AggregateOutput struct {
	Results    []Result
	Unresolved int
}

// Aggregate resolves each flow's source/destination IP to pod+AZ info via resolve, and sums
// bytes transferred per Key. Flows whose source or destination IP can't be resolved to a known
// pod (e.g. traffic to/from outside the cluster, or a pod not yet indexed) are skipped — this
// tool intentionally only attributes intra-cluster, pod-to-pod traffic.
//
// Byte accounting uses OrigBytes when present (kernel conntrack accounting enabled), and treats
// a flow with no accounting data (HasByteAccounting() == false) as contributing 0 bytes rather
// than being dropped, so it still counts toward connection visibility in future milestones.
func Aggregate(flows []conntrack.Flow, resolve ResolveFunc) AggregateOutput {
	totals := make(map[Key]int64)
	unresolved := 0

	for _, f := range flows {
		srcPod, srcNode, srcOK := resolve(f.OrigSrcIP)
		_, dstNode, dstOK := resolve(f.OrigDstIP)
		if !srcOK || !dstOK {
			unresolved++
			continue
		}

		var bytes int64
		if f.OrigBytes > 0 {
			bytes = f.OrigBytes
		}

		key := Key{
			SrcZone:      srcNode.Zone,
			DstZone:      dstNode.Zone,
			SrcNamespace: srcPod.Namespace,
			SrcWorkload:  srcPod.Workload,
		}
		totals[key] += bytes
	}

	results := make([]Result, 0, len(totals))
	for k, b := range totals {
		results = append(results, Result{Key: k, Bytes: b})
	}
	return AggregateOutput{Results: results, Unresolved: unresolved}
}
