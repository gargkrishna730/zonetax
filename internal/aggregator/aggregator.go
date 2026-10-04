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

// Key identifies one aggregation bucket: bytes sent from a pod in SrcZone to a pod in DstZone,
// broken down by sending and receiving workload. "Src" is always the SENDER of the bytes, which
// for the reply half of a connection is the connection's destination (e.g. a server streaming
// a download back to a client). Same-AZ traffic (SrcZone == DstZone) is tracked for ratios but
// not billed.
type Key struct {
	SrcZone      string
	DstZone      string
	SrcNamespace string
	SrcWorkload  string
	// DstNamespace/DstWorkload identify the receiving pod's owning workload. Pod names are not
	// tracked: pods are ephemeral and would fragment attribution across pod generations.
	DstNamespace string
	DstWorkload  string
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

// Aggregate attributes the bytes in each conntrack flow to the workloads and AZs at both ends.
//
// Both directions of a connection are counted: OrigBytes as sent by the connection's source,
// ReplyBytes as sent by its destination. A download (client connects, server streams data back)
// is real cross-AZ traffic and was previously invisible because only OrigBytes was read.
//
// localNode, when non-empty, is the node this agent runs on. A cross-node connection appears in
// the conntrack tables of BOTH nodes, so if every agent counted every flow the collector (which
// sums all agents) would report each connection twice. Each agent therefore only counts a flow
// when the connection's source pod (the side that opened it) is on its own node, so every
// connection is counted exactly once cluster-wide. Pass "" to count everything (tests, or a
// single-node view).
//
// Flows whose endpoints can't be resolved to a known pod (traffic to/from outside the cluster,
// or a pod not yet indexed) are skipped: ZoneTax only attributes intra-cluster pod traffic.
func Aggregate(flows []conntrack.Flow, resolve ResolveFunc, localNode string) AggregateOutput {
	totals := make(map[Key]int64)
	unresolved := 0

	for _, f := range flows {
		srcPod, srcNode, srcOK := resolve(f.OrigSrcIP)
		// The real responder is the reply tuple's source. For a connection to a ClusterIP
		// Service, OrigDstIP is the virtual Service IP (DNAT happens on the source node) and only
		// ReplySrcIP holds the backend pod's IP. For direct pod-to-pod traffic they are equal.
		dstIP := f.ReplySrcIP
		if dstIP == "" {
			dstIP = f.OrigDstIP
		}
		dstPod, dstNode, dstOK := resolve(dstIP)
		if !dstOK && dstIP != f.OrigDstIP {
			dstPod, dstNode, dstOK = resolve(f.OrigDstIP)
		}
		if !srcOK || !dstOK {
			unresolved++
			continue
		}
		if localNode != "" && srcPod.NodeName != localNode {
			continue // the agent on the source pod's node owns this connection
		}

		if f.OrigBytes > 0 {
			totals[Key{
				SrcZone: srcNode.Zone, DstZone: dstNode.Zone,
				SrcNamespace: srcPod.Namespace, SrcWorkload: srcPod.Workload,
				DstNamespace: dstPod.Namespace, DstWorkload: dstPod.Workload,
			}] += f.OrigBytes
		}
		if f.ReplyBytes > 0 {
			totals[Key{
				SrcZone: dstNode.Zone, DstZone: srcNode.Zone,
				SrcNamespace: dstPod.Namespace, SrcWorkload: dstPod.Workload,
				DstNamespace: srcPod.Namespace, DstWorkload: srcPod.Workload,
			}] += f.ReplyBytes
		}
	}

	results := make([]Result, 0, len(totals))
	for k, b := range totals {
		results = append(results, Result{Key: k, Bytes: b})
	}
	return AggregateOutput{Results: results, Unresolved: unresolved}
}
