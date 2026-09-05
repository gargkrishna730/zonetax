// Package collector — history.go implements an in-memory, hourly-bucketed view of cost/traffic
// over time, built by snapshotting the cumulative totals from each successful collection cycle.
//
// Why snapshots+diffing rather than storing per-cycle values directly: costengine.Summary's
// totals (TotalCrossAZCost, TotalCrossAZGB, TotalSameAZGB) are CUMULATIVE since the underlying
// Prometheus counter started — i.e. since the agent process last restarted (see
// internal/metrics: CrossAZBytesTotal is a prometheus.CounterVec, monotonically increasing by
// design so the collector can use rate()/increase() semantics, matching how the agent's own
// delta-tracking bug fix works at the byte-counting layer). A single snapshot's value is
// therefore not "how much was spent in this hour" — it's "how much has been spent in total so
// far." Hourly cost/traffic requires diffing consecutive snapshots within each hour bucket.
//
// This is intentionally NOT a real time-series database: it's bounded in-memory history that
// only covers time since this collector process started (see Store.StartedAt), evicts data
// older than maxHistoryHours, and is lost on restart. The API and UI must both state this
// honestly rather than imply a longer or more durable window than actually exists.
package collector

import (
	"time"
)

// snapshot is one recorded reading of the cumulative totals at a point in time.
type snapshot struct {
	at          time.Time
	crossAZCost float64
	crossAZGB   float64
	sameAZGB    float64
}

// History accumulates cumulative-total snapshots and derives hourly cost/traffic deltas from
// them. Safe for concurrent use.
type History struct {
	maxHours int
	snaps    []snapshot
}

// NewHistory returns an empty History retaining at most maxHours hours of snapshots.
func NewHistory(maxHours int) *History {
	if maxHours <= 0 {
		maxHours = 1
	}
	return &History{maxHours: maxHours}
}

// Record appends a new cumulative-total snapshot and evicts anything older than maxHours. Not
// safe to call concurrently with itself (Store serializes access via its own mutex — see
// Store.set/Store.History, which is the only intended caller).
func (h *History) Record(at time.Time, crossAZCost, crossAZGB, sameAZGB float64) {
	h.snaps = append(h.snaps, snapshot{at: at, crossAZCost: crossAZCost, crossAZGB: crossAZGB, sameAZGB: sameAZGB})
	cutoff := at.Add(-time.Duration(h.maxHours) * time.Hour)
	i := 0
	for i < len(h.snaps) && h.snaps[i].at.Before(cutoff) {
		i++
	}
	// Keep one snapshot before the cutoff (if any) so the very first retained bucket after
	// eviction still has a prior baseline to diff against, rather than silently starting from
	// zero and understating that bucket's real delta.
	if i > 1 {
		h.snaps = h.snaps[i-1:]
	}
}

// Bucket is one time-bucketed cost/traffic delta, plus whether the bucket is fully observed.
type Bucket struct {
	Start          time.Time
	End            time.Time
	CrossAZCostUSD float64
	CrossAZGB      float64
	SameAZGB       float64
	// Complete is false when this bucket's time range extends past the most recent snapshot
	// (i.e. it's the current, still-in-progress hour/day) or starts before this History's
	// earliest snapshot (i.e. the collector wasn't running/successfully collecting for that
	// whole bucket) — either way, the bucket's totals reflect only the portion actually
	// observed, not the full period, and callers (API/UI) must say so rather than presenting a
	// partial bucket as equivalent to a complete one.
	Complete bool
	// HasData is false when there is no overlapping snapshot data for this bucket at all (e.g.
	// requesting a bucket entirely before the collector's earliest snapshot) — distinct from
	// Complete=false-but-some-data, since "no data" and "partial data" need different UI
	// treatment (a gap/missing warning vs. an "in progress" label).
	HasData bool
}

// Buckets returns hourly deltas covering [since, now], one Bucket per hour boundary, using the
// snapshot immediately at-or-before each boundary as that boundary's cumulative baseline
// (standard "last observation carried forward" for sparse periodic sampling). Handles agent/
// collector restarts (a cumulative value that decreased since the previous snapshot, or a gap
// with no snapshots at all) by treating that portion of the range as having no attributable
// delta rather than computing a nonsensical negative number — see resolveDelta.
func (h *History) Buckets(since, now time.Time, bucketSize time.Duration) []Bucket {
	if bucketSize <= 0 {
		bucketSize = time.Hour
	}
	if len(h.snaps) == 0 || !now.After(since) {
		return nil
	}
	snaps := h.snaps // already time-ordered by append order (Record is called sequentially)

	var buckets []Bucket
	for start := truncateTo(since, bucketSize); start.Before(now); start = start.Add(bucketSize) {
		end := start.Add(bucketSize)
		baseline, baselineOK := snapshotAtOrBefore(snaps, start)
		latest, latestOK := snapshotAtOrBefore(snaps, end)
		if !latestOK {
			// No snapshot exists at or before this bucket's end at all — either the bucket is
			// entirely before the collector's first successful cycle, or (for the trailing/
			// current bucket) no cycle has completed within it yet.
			buckets = append(buckets, Bucket{Start: start, End: end, HasData: false, Complete: false})
			continue
		}
		var costDelta, gbDelta, sameDelta float64
		hasData := true
		// usedFallbackBaseline marks that we approximated using the earliest available
		// snapshot instead of a true at-or-before-start snapshot — this bucket is missing the
		// portion of its window before history began, so it must never be reported as
		// Complete even once a snapshot exists at/after its end.
		usedFallbackBaseline := false
		if !baselineOK {
			// The bucket started before any snapshot existed (guaranteed here, since baselineOK
			// is false — i.e. no snapshot has at<=start, so snaps[0].at, the earliest ever
			// snapshot, is > start). If that earliest snapshot still falls before this bucket's
			// END, we have real, usable data from that point onward — using it as the baseline
			// correctly computes "how much accumulated since we started observing," at the cost
			// of under-counting the (unobservable) portion before history began. This was a
			// real bug: without this fallback, the very first bucket after every collector
			// restart stayed permanently "no data" even minutes later once plenty of real delta
			// data existed within it — caught by a user screen recording showing the chart
			// rendering completely empty shortly after a redeploy. If the earliest snapshot is
			// at/after this bucket's end, there's truly no overlapping data and hasData stays
			// false.
			if snaps[0].at.Before(end) {
				baseline = snaps[0]
				baselineOK = true
				usedFallbackBaseline = true
			} else {
				hasData = false
			}
		}
		if baselineOK {
			costDelta = resolveDelta(baseline.crossAZCost, latest.crossAZCost)
			gbDelta = resolveDelta(baseline.crossAZGB, latest.crossAZGB)
			sameDelta = resolveDelta(baseline.sameAZGB, latest.sameAZGB)
		}
		complete := hasData && !usedFallbackBaseline && !end.After(mostRecentSnapshotTime(snaps))
		buckets = append(buckets, Bucket{
			Start: start, End: end,
			CrossAZCostUSD: costDelta, CrossAZGB: gbDelta, SameAZGB: sameDelta,
			Complete: complete, HasData: hasData,
		})
	}
	return buckets
}

// resolveDelta returns latest-baseline, treating a decrease (latest < baseline) as a counter
// reset (the underlying agent/collector process restarted, so its cumulative counter restarted
// at zero) rather than a negative delta. In that case latest's absolute value is itself the
// correct "how much accumulated since the reset" — mirroring internal/deltatrack's identical
// reasoning for the same class of problem at the byte-counting layer.
func resolveDelta(baseline, latest float64) float64 {
	if latest < baseline {
		return latest
	}
	return latest - baseline
}

func truncateTo(t time.Time, d time.Duration) time.Time {
	return t.Truncate(d)
}

// snapshotAtOrBefore returns the last snapshot with at <= t, using a linear scan from the end
// (snapshot counts are small — bounded by maxHistoryHours * one point per scrape interval,
// typically a few thousand at most) rather than a binary search, favoring simplicity.
func snapshotAtOrBefore(snaps []snapshot, t time.Time) (snapshot, bool) {
	for i := len(snaps) - 1; i >= 0; i-- {
		if !snaps[i].at.After(t) {
			return snaps[i], true
		}
	}
	return snapshot{}, false
}

func mostRecentSnapshotTime(snaps []snapshot) time.Time {
	if len(snaps) == 0 {
		return time.Time{}
	}
	return snaps[len(snaps)-1].at
}

// EarliestSnapshot returns the time of the oldest snapshot still retained, or the zero Time if
// History has no data yet. Used by the API to report the actual start of available history,
// distinct from Store.StartedAt (which never gets evicted) so a caller asking for a 7-day range
// on a collector that's only been up for 2 hours gets an honest "history starts at X" answer.
func (h *History) EarliestSnapshot() time.Time {
	if len(h.snaps) == 0 {
		return time.Time{}
	}
	return h.snaps[0].at
}
