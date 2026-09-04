// Package deltatrack converts conntrack's cumulative-since-connection-start byte counters into
// per-sample deltas suitable for feeding a monotonic Prometheus counter's Add().
//
// This exists to fix a real accuracy bug found comparing ZoneTax's tracked cost against the
// real AWS bill (Cost Explorer's DataTransfer-Regional-Bytes line item) on solrn-dev: ZoneTax
// reported roughly 80x the real cross-AZ spend. Root cause: /proc/net/nf_conntrack's `bytes=`
// field is the TOTAL bytes transferred over that connection's entire lifetime so far, not bytes
// since the last sample. The agent samples every 15s and was calling
// metrics.CrossAZBytesTotal.Add(flow.OrigBytes) directly — for any connection that stays open
// across multiple sample ticks (exactly the case for long-lived gRPC/HTTP2 streams, e.g. an
// OTel collector's export connection, which is why that workload dominated the top-offenders
// list), its full lifetime byte count got re-added every single tick instead of just the
// increment, compounding into a massive overcount the longer a connection stayed open.
package deltatrack

import "github.com/gargkrishna730/zonetax/internal/conntrack"

// Key identifies one connection by its original-direction 5-tuple (protocol + both endpoints'
// IP:port), the standard notion of a flow's identity in conntrack. This must be computed from
// the ORIGINAL direction consistently (not the reply direction) since that's the side the
// aggregator attributes cost to (see internal/aggregator).
type Key struct {
	Protocol string
	SrcIP    string
	SrcPort  int
	DstIP    string
	DstPort  int
}

func keyOf(f conntrack.Flow) Key {
	return Key{
		Protocol: f.Protocol,
		SrcIP:    f.OrigSrcIP,
		SrcPort:  f.OrigSrcPort,
		DstIP:    f.OrigDstIP,
		DstPort:  f.OrigDstPort,
	}
}

// Tracker holds the last-seen cumulative byte count per flow, across sampling cycles. Not safe
// for concurrent use — callers should serialize access (the agent only ever samples from one
// goroutine at a time via runSampleLoop's ticker, so this is fine as a plain map).
type Tracker struct {
	last map[Key]int64
}

// New returns an empty Tracker.
func New() *Tracker {
	return &Tracker{last: make(map[Key]int64)}
}

// Delta returns the bytes transferred since the last sample for this flow, and records current
// as the new baseline for the next call. Three cases:
//   - Never seen before: this is either a genuinely new connection, or one that already existed
//     before the agent started tracking it — either way, `current` bytes haven't been counted
//     yet, so the full value is the correct delta.
//   - current >= previous (the normal case): delta is the increment, current-previous.
//   - current < previous: the byte counter went backwards, which cannot happen within one
//     connection's lifetime (nf_conntrack counters are monotonic per-connection) — the only way
//     this happens is the connection closed and a new, unrelated connection reused the exact
//     same 4-tuple+protocol before this Tracker's Prune() ran. Treated as a fresh connection:
//     the previously-tracked amount is presumed already fully counted (we can't recover exactly
//     what happened to the old connection's tail), and `current` becomes the delta going forward.
func (t *Tracker) Delta(k Key, current int64) int64 {
	prev, ok := t.last[k]
	t.last[k] = current
	if !ok || current < prev {
		return current
	}
	return current - prev
}

// Prune drops tracked flows whose key is not present in `seen` — i.e. connections that closed
// since the last sample — so the tracker's memory doesn't grow unboundedly as connections churn
// over the agent's lifetime. Call once per sample cycle after processing every flow with Delta.
func (t *Tracker) Prune(seen map[Key]bool) {
	for k := range t.last {
		if !seen[k] {
			delete(t.last, k)
		}
	}
}

// ApplyFlowDeltas is the integration point between raw conntrack samples (whose OrigBytes is
// cumulative since the connection opened) and the aggregator (which expects "bytes transferred
// during this sample interval"). Returns a new slice with each flow's OrigBytes rewritten to
// the delta since the tracker last saw that flow's 5-tuple; flows with no byte-accounting data
// (OrigBytes < 0, meaning conntrack accounting is disabled) pass through unchanged, since a
// delta is meaningless without a baseline. Must be called with the FULL set of currently-open
// flows once per sample cycle — it also prunes closed connections from the tracker's memory, so
// a partial flow list would incorrectly evict still-open connections' tracked baselines.
func (t *Tracker) ApplyFlowDeltas(flows []conntrack.Flow) []conntrack.Flow {
	out := make([]conntrack.Flow, len(flows))
	seen := make(map[Key]bool, len(flows))
	for i, f := range flows {
		out[i] = f
		if f.OrigBytes < 0 {
			continue
		}
		k := keyOf(f)
		seen[k] = true
		out[i].OrigBytes = t.Delta(k, f.OrigBytes)
	}
	t.Prune(seen)
	return out
}
