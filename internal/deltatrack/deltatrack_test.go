package deltatrack

import (
	"testing"

	"github.com/gargkrishna730/zonetax/internal/conntrack"
)

func flow(srcIP string, srcPort int, dstIP string, dstPort int, bytes int64) conntrack.Flow {
	return conntrack.Flow{
		Protocol: "tcp", OrigSrcIP: srcIP, OrigSrcPort: srcPort, OrigDstIP: dstIP, OrigDstPort: dstPort,
		OrigBytes: bytes,
	}
}

// TestDelta_FirstSightingReturnsFullValue is a regression test for the actual bug found
// comparing ZoneTax against real AWS billing (Cost Explorer showed ~$2/day, ZoneTax reported an
// extrapolated ~$173/day — an ~80x overcount): a connection's FIRST sample is legitimately its
// full cumulative byte count so far (there's no prior sample to diff against), but every sample
// AFTER that must be a delta, not the same cumulative total re-added.
func TestDelta_FirstSightingReturnsFullValue(t *testing.T) {
	tr := New()
	k := Key{Protocol: "tcp", SrcIP: "10.0.0.1", SrcPort: 1234, DstIP: "10.0.0.2", DstPort: 80}
	if got := tr.Delta(k, 1000); got != 1000 {
		t.Errorf("Delta() on first sighting = %d, want 1000 (full cumulative value)", got)
	}
}

func TestDelta_SubsequentSamplesReturnIncrementOnly(t *testing.T) {
	tr := New()
	k := Key{Protocol: "tcp", SrcIP: "10.0.0.1", SrcPort: 1234, DstIP: "10.0.0.2", DstPort: 80}

	if got := tr.Delta(k, 1000); got != 1000 {
		t.Fatalf("first Delta() = %d, want 1000", got)
	}
	// This is the exact bug: without delta tracking, a long-lived connection sampled 3 times
	// would contribute 1000+1500+2200=4700 bytes total instead of the real 2200 bytes
	// transferred — a 2.1x overcount for just 3 samples, compounding further the longer the
	// connection stays open (which is why a persistent OTel gRPC stream, sampled every 15s over
	// hours, produced an ~80x real-world overcount).
	if got := tr.Delta(k, 1500); got != 500 {
		t.Errorf("second Delta() = %d, want 500 (1500-1000)", got)
	}
	if got := tr.Delta(k, 2200); got != 700 {
		t.Errorf("third Delta() = %d, want 700 (2200-1500)", got)
	}
}

func TestDelta_CounterWentBackwardsTreatedAsNewConnection(t *testing.T) {
	tr := New()
	k := Key{Protocol: "tcp", SrcIP: "10.0.0.1", SrcPort: 1234, DstIP: "10.0.0.2", DstPort: 80}
	tr.Delta(k, 5000)
	// A lower byte count on the same 5-tuple can only mean the original connection closed and a
	// new, unrelated connection reused the identical tuple+protocol between samples — treat the
	// new value as a fresh baseline rather than computing a nonsensical negative delta.
	if got := tr.Delta(k, 100); got != 100 {
		t.Errorf("Delta() after counter decrease = %d, want 100 (treated as new connection)", got)
	}
}

func TestPrune_RemovesClosedConnections(t *testing.T) {
	tr := New()
	k1 := Key{Protocol: "tcp", SrcIP: "10.0.0.1", SrcPort: 1, DstIP: "10.0.0.2", DstPort: 80}
	k2 := Key{Protocol: "tcp", SrcIP: "10.0.0.3", SrcPort: 2, DstIP: "10.0.0.4", DstPort: 80}
	tr.Delta(k1, 1000)
	tr.Delta(k2, 2000)

	tr.Prune(map[Key]bool{k1: true}) // k2's connection closed

	// k1 should still be tracked (delta, not full value)
	if got := tr.Delta(k1, 1100); got != 100 {
		t.Errorf("Delta() for pruned-survivor k1 = %d, want 100 (still tracked)", got)
	}
	// k2 was pruned, so its next sighting (even with the same tuple, e.g. a reused ephemeral
	// port) is treated as a brand new connection — full value, not a delta against stale state.
	if got := tr.Delta(k2, 2000); got != 2000 {
		t.Errorf("Delta() for pruned k2 = %d, want 2000 (treated as new after prune)", got)
	}
}

func TestApplyFlowDeltas_RewritesOrigBytesToPerSampleDelta(t *testing.T) {
	tr := New()

	// Sample 1: two flows, both first-seen — full cumulative value is the correct delta.
	sample1 := []conntrack.Flow{
		flow("10.0.1.1", 100, "10.0.2.1", 80, 1000),
		flow("10.0.1.2", 200, "10.0.2.2", 443, 500),
	}
	out1 := tr.ApplyFlowDeltas(sample1)
	if out1[0].OrigBytes != 1000 || out1[1].OrigBytes != 500 {
		t.Fatalf("sample1 deltas = %d, %d; want 1000, 500", out1[0].OrigBytes, out1[1].OrigBytes)
	}

	// Sample 2: first flow persists and grew (the long-lived-connection case that caused the
	// real overcount bug), second flow closed (absent) and a brand new third flow appears.
	sample2 := []conntrack.Flow{
		flow("10.0.1.1", 100, "10.0.2.1", 80, 1600), // same connection, +600 bytes
		flow("10.0.1.3", 300, "10.0.2.3", 22, 750),  // new connection
	}
	out2 := tr.ApplyFlowDeltas(sample2)
	if out2[0].OrigBytes != 600 {
		t.Errorf("persistent connection delta = %d, want 600 (1600-1000, NOT the full 1600)", out2[0].OrigBytes)
	}
	if out2[1].OrigBytes != 750 {
		t.Errorf("new connection delta = %d, want 750 (full value, first sighting)", out2[1].OrigBytes)
	}

	// Sample 3: the first connection reappears — its closed sibling from sample1 (10.0.1.2)
	// should have been pruned after sample2 didn't include it, so a hypothetical reappearance
	// with the same tuple would be treated as new, not diffed against stale sample1 state. We
	// verify pruning happened by checking the tracker's internal map size stayed bounded rather
	// than growing to include the long-closed flow.
	if _, stillTracked := tr.last[keyOf(sample1[1])]; stillTracked {
		t.Error("closed connection from sample1 should have been pruned after sample2, but is still tracked")
	}
}

func TestApplyFlowDeltas_FlowsWithoutByteAccountingPassThroughUnchanged(t *testing.T) {
	tr := New()
	f := flow("10.0.1.1", 100, "10.0.2.1", 80, 0)
	f.OrigBytes = -1 // simulates conntrack accounting disabled (see conntrack.Flow doc)

	out := tr.ApplyFlowDeltas([]conntrack.Flow{f})
	if out[0].OrigBytes != -1 {
		t.Errorf("OrigBytes = %d, want -1 (unchanged passthrough, no accounting data to diff)", out[0].OrigBytes)
	}
}
