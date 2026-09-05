package collector

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}

func TestHistory_BucketsComputeDeltaFromCumulativeSnapshots(t *testing.T) {
	h := NewHistory(24)
	base := mustParse(t, "2026-09-05T10:00:00Z")

	// Cumulative totals grow monotonically within the hour, as a real Prometheus counter would.
	h.Record(base, 1.00, 50, 5)
	h.Record(base.Add(20*time.Minute), 1.50, 75, 7)
	h.Record(base.Add(40*time.Minute), 2.20, 110, 9)
	// Cross into the next hour bucket.
	h.Record(base.Add(65*time.Minute), 2.80, 140, 10)

	buckets := h.Buckets(base, base.Add(70*time.Minute), time.Hour)
	if len(buckets) != 2 {
		t.Fatalf("Buckets() returned %d buckets, want 2 (one per hour boundary crossed)", len(buckets))
	}

	// First bucket [10:00, 11:00): baseline is the 10:00 snapshot (1.00), the last snapshot
	// at-or-before 11:00 within range is the 10:40 one (2.20) — NOT the 11:05 one, since that's
	// past this bucket's end.
	b0 := buckets[0]
	if got, want := b0.CrossAZCostUSD, 1.20; !almostEqualHistory(got, want) {
		t.Errorf("bucket0 CrossAZCostUSD = %v, want %v (2.20 - 1.00)", got, want)
	}
	if got, want := b0.CrossAZGB, 60.0; !almostEqualHistory(got, want) {
		t.Errorf("bucket0 CrossAZGB = %v, want %v (110 - 50)", got, want)
	}
	if !b0.HasData {
		t.Error("bucket0 HasData = false, want true")
	}

	// Second bucket [11:00, 12:00) has only the 11:05 snapshot as both its "at or before end"
	// and its baseline is the last snapshot at-or-before 11:00, which is still the 10:40 one
	// (2.20) since no snapshot exists exactly at/after 11:00 until 11:05.
	b1 := buckets[1]
	if got, want := b1.CrossAZCostUSD, 0.60; !almostEqualHistory(got, want) {
		t.Errorf("bucket1 CrossAZCostUSD = %v, want %v (2.80 - 2.20)", got, want)
	}
}

func TestHistory_CounterResetTreatedAsFreshStartNotNegative(t *testing.T) {
	h := NewHistory(24)
	base := mustParse(t, "2026-09-05T10:00:00Z")

	h.Record(base, 5.00, 200, 20)
	// Simulates an agent/collector restart mid-bucket: the cumulative counter drops back near
	// zero because the underlying process (and its Prometheus counter) restarted.
	h.Record(base.Add(30*time.Minute), 0.40, 15, 2)

	buckets := h.Buckets(base, base.Add(45*time.Minute), time.Hour)
	if len(buckets) != 1 {
		t.Fatalf("Buckets() returned %d buckets, want 1", len(buckets))
	}
	b := buckets[0]
	// Must NOT be 0.40 - 5.00 = -4.60 (nonsensical). The post-restart value (0.40) IS the
	// correct "how much accumulated since the reset" on its own.
	if got, want := b.CrossAZCostUSD, 0.40; !almostEqualHistory(got, want) {
		t.Errorf("CrossAZCostUSD after counter reset = %v, want %v (treated as fresh start, not negative)", got, want)
	}
	if b.CrossAZCostUSD < 0 {
		t.Errorf("CrossAZCostUSD = %v, must never be negative", b.CrossAZCostUSD)
	}
}

func TestHistory_BucketBeforeAnySnapshotHasNoData(t *testing.T) {
	h := NewHistory(24)
	base := mustParse(t, "2026-09-05T10:00:00Z")
	h.Record(base, 1.00, 50, 5)

	// Ask for a range that starts well before the collector's first snapshot.
	buckets := h.Buckets(base.Add(-3*time.Hour), base, time.Hour)
	for i, b := range buckets[:len(buckets)-1] { // exclude the final bucket, which does have data
		if b.HasData {
			t.Errorf("bucket[%d] (%s) HasData = true, want false (entirely before first snapshot)", i, b.Start)
		}
		if b.Complete {
			t.Errorf("bucket[%d] Complete = true, want false (no data at all)", i)
		}
	}
}

func TestHistory_PartialBucketStraddlingFirstSnapshotUsesEarliestAsFallbackBaseline(t *testing.T) {
	// A bucket that STARTS before the first snapshot but ENDS after it: we don't know the true
	// cumulative value at the bucket's real start, so we fall back to the earliest available
	// snapshot as an approximate baseline — this under-counts the unobservable portion before
	// history began, but correctly surfaces the real, observed delta instead of permanently
	// showing "no data" for a period that plainly has data (the bug this regresses against: the
	// very first bucket after every collector restart stayed "no data" forever, even minutes
	// later once real numbers existed within it — caught via a user screen recording).
	h := NewHistory(24)
	firstSnap := mustParse(t, "2026-09-05T10:30:00Z")
	h.Record(firstSnap, 1.00, 50, 5)
	h.Record(mustParse(t, "2026-09-05T10:45:00Z"), 1.60, 80, 8)

	buckets := h.Buckets(mustParse(t, "2026-09-05T10:00:00Z"), mustParse(t, "2026-09-05T11:00:00Z"), time.Hour)
	if len(buckets) != 1 {
		t.Fatalf("Buckets() returned %d, want 1", len(buckets))
	}
	b := buckets[0]
	if !b.HasData {
		t.Error("bucket straddling the first-ever snapshot should have HasData = true (real data exists from 10:30 onward)")
	}
	if got, want := b.CrossAZCostUSD, 0.60; !almostEqualHistory(got, want) {
		t.Errorf("CrossAZCostUSD = %v, want %v (1.60 - 1.00, using the earliest snapshot as baseline)", got, want)
	}
	// This bucket is missing the 10:00-10:30 portion (before history began) — it must never be
	// reported as Complete even though a later snapshot exists past its end.
	if b.Complete {
		t.Error("bucket using a fallback (approximate) baseline must not be marked Complete")
	}
}

// TestHistory_BucketEntirelyAfterFirstSnapshotIsUnaffectedByFallback is a regression guard: the
// fallback-baseline logic must only kick in for the specific bucket that straddles history's
// start, not for later buckets that already have a proper at-or-before-start snapshot.
func TestHistory_BucketEntirelyAfterFirstSnapshotIsUnaffectedByFallback(t *testing.T) {
	h := NewHistory(24)
	base := mustParse(t, "2026-09-05T10:00:00Z")
	h.Record(base, 1.00, 50, 5)
	h.Record(base.Add(65*time.Minute), 2.00, 100, 10)
	h.Record(base.Add(125*time.Minute), 3.50, 175, 17)

	buckets := h.Buckets(base, base.Add(130*time.Minute), time.Hour)
	if len(buckets) != 3 {
		t.Fatalf("Buckets() returned %d, want 3", len(buckets))
	}
	// The second bucket [11:00,12:00) has a proper baseline (the 11:05 snapshot's predecessor,
	// the 10:00 one is before 11:00 — actually the last snapshot at-or-before 11:00 is still the
	// 10:00 one) and a proper latest (11:05) — it's fully observed and should be Complete.
	if !buckets[1].Complete {
		t.Error("a normally-baselined, fully-observed bucket should still be marked Complete")
	}
}

func TestHistory_MostRecentBucketIncomplete(t *testing.T) {
	h := NewHistory(24)
	base := mustParse(t, "2026-09-05T10:00:00Z")
	h.Record(base, 1.00, 50, 5)
	h.Record(base.Add(20*time.Minute), 1.50, 75, 7)

	// "now" is mid-bucket — the most recent snapshot (10:20) is well before the bucket's end
	// (11:00), so this bucket only reflects a partial hour and must be marked incomplete.
	buckets := h.Buckets(base, base.Add(20*time.Minute), time.Hour)
	if len(buckets) != 1 {
		t.Fatalf("Buckets() returned %d, want 1", len(buckets))
	}
	if buckets[0].Complete {
		t.Error("current in-progress bucket Complete = true, want false")
	}
	if !buckets[0].HasData {
		t.Error("current in-progress bucket HasData = false, want true (it does have partial data)")
	}
}

func TestHistory_CompletedPastBucketMarkedComplete(t *testing.T) {
	h := NewHistory(24)
	base := mustParse(t, "2026-09-05T10:00:00Z")
	h.Record(base, 1.00, 50, 5)
	// A snapshot well past the bucket's end confirms the bucket is fully observed.
	h.Record(base.Add(90*time.Minute), 3.00, 150, 15)

	buckets := h.Buckets(base, base.Add(90*time.Minute), time.Hour)
	if len(buckets) < 1 {
		t.Fatal("expected at least 1 bucket")
	}
	if !buckets[0].Complete {
		t.Error("fully-observed past bucket Complete = false, want true")
	}
}

func TestHistory_RecordEvictsBeyondMaxHours(t *testing.T) {
	h := NewHistory(2) // retain only 2 hours
	base := mustParse(t, "2026-09-05T10:00:00Z")

	h.Record(base, 1.00, 50, 5)
	h.Record(base.Add(1*time.Hour), 2.00, 100, 10)
	h.Record(base.Add(2*time.Hour), 3.00, 150, 15)
	// This should push the very first (10:00) snapshot out of the 2-hour retention window.
	h.Record(base.Add(4*time.Hour), 5.00, 250, 25)

	earliest := h.EarliestSnapshot()
	if earliest.Before(base.Add(1 * time.Hour)) {
		t.Errorf("EarliestSnapshot() = %s, want >= %s (oldest snapshot should have been evicted)", earliest, base.Add(time.Hour))
	}
}

func TestHistory_EmptyHistoryReturnsNoBuckets(t *testing.T) {
	h := NewHistory(24)
	buckets := h.Buckets(mustParse(t, "2026-09-05T10:00:00Z"), mustParse(t, "2026-09-05T11:00:00Z"), time.Hour)
	if buckets != nil {
		t.Errorf("Buckets() on empty History = %v, want nil", buckets)
	}
}

func almostEqualHistory(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}
