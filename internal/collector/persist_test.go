package collector

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gargkrishna730/zonetax/internal/costengine"
)

func agentSum(gb float64) costengine.Summary {
	return costengine.Summary{
		Entries:       []costengine.Entry{fakeEntry("a", "b", "web", "db", gb, gb*0.02)},
		TotalSameAZGB: gb,
	}
}

func totalGB(h *History, since, now time.Time) float64 {
	t := 0.0
	for _, b := range h.Buckets(since, now, time.Hour) {
		t += b.CrossAZGB
	}
	return t
}

// A failed scrape must not create a fake spike when the agent comes back.
func TestStore_FailedScrapeDoesNotSpike(t *testing.T) {
	s := &Store{}
	base := mustParse(t, "2026-10-01T10:00:00Z")
	s.observe(map[string]costengine.Summary{"a1": agentSum(1000), "a2": agentSum(500)}, base)
	s.observe(map[string]costengine.Summary{"a1": agentSum(1001), "a2": agentSum(501)}, base.Add(30*time.Second))
	s.observe(map[string]costengine.Summary{"a1": agentSum(1002)}, base.Add(60*time.Second)) // a2 failed
	s.observe(map[string]costengine.Summary{"a1": agentSum(1003), "a2": agentSum(503)}, base.Add(90*time.Second))
	// Real traffic: a1 +3, a2 +3 = 6 GB. Old code saw totals 1500,1502,1002,1506 -> reset -> ~1506.
	if got := totalGB(s.History(), base, base.Add(2*time.Minute)); got < 5.99 || got > 6.01 {
		t.Fatalf("observed %.2f GB, want 6", got)
	}
}

// Agent restart (counter reset within the same pod) and a brand-new agent pod are both handled.
func TestStore_AgentResetAndNewAgent(t *testing.T) {
	s := &Store{}
	base := mustParse(t, "2026-10-01T10:00:00Z")
	s.observe(map[string]costengine.Summary{"a1": agentSum(100)}, base)
	s.observe(map[string]costengine.Summary{"a1": agentSum(110)}, base.Add(time.Minute))                       // +10
	s.observe(map[string]costengine.Summary{"a1": agentSum(4)}, base.Add(2*time.Minute))                       // reset, +4
	s.observe(map[string]costengine.Summary{"a1": agentSum(6), "new": agentSum(900)}, base.Add(3*time.Minute)) // +2, new = baseline only
	s.observe(map[string]costengine.Summary{"a1": agentSum(6), "new": agentSum(905)}, base.Add(4*time.Minute)) // +5
	if got := totalGB(s.History(), base, base.Add(5*time.Minute)); got < 20.99 || got > 21.01 {
		t.Fatalf("observed %.2f GB, want 21", got)
	}
}

func TestHistory_SaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "history.json.gz")
	base := mustParse(t, "2026-10-01T10:00:00Z")
	h := NewHistory(24)
	for i := 0; i <= 120; i++ {
		gb := float64(i)
		h.Record(base.Add(time.Duration(i)*time.Minute), gb*0.02, gb, gb/2,
			[]costengine.Entry{fakeEntry("a", "b", "web", "db", gb, gb*0.02), fakeEntry("b", "c", "api", "cache", gb/4, gb/4*0.02)})
	}
	if err := h.Save(path); err != nil {
		t.Fatal(err)
	}
	now := base.Add(2 * time.Hour)
	h2 := NewHistory(24)
	if err := h2.Load(path, now); err != nil {
		t.Fatal(err)
	}
	if len(h2.snaps) != len(h.snaps) {
		t.Fatalf("loaded %d snapshots, want %d", len(h2.snaps), len(h.snaps))
	}
	b1, b2 := h.Buckets(base, now, time.Hour), h2.Buckets(base, now, time.Hour)
	for i := range b1 {
		if b1[i] != b2[i] {
			t.Errorf("bucket %d differs: %+v vs %+v", i, b1[i], b2[i])
		}
	}
	d1, _, _ := h.EntriesRange(base, now, time.Minute)
	d2, _, _ := h2.EntriesRange(base, now, time.Minute)
	if len(d1) != 2 || len(d2) != 2 {
		t.Fatalf("routes: %d vs %d", len(d1), len(d2))
	}
	if fi, _ := os.Stat(path); fi.Size() > 64*1024 {
		t.Errorf("file unexpectedly large: %d bytes", fi.Size())
	}
}

func TestHistory_LoadDropsExpiredAndHandlesBadFiles(t *testing.T) {
	dir := t.TempDir()
	base := mustParse(t, "2026-10-01T00:00:00Z")
	h := NewHistory(48)
	h.Record(base, 1, 1, 0, nil)
	h.Record(base.Add(30*time.Hour), 2, 2, 0, nil)
	path := filepath.Join(dir, "h.gz")
	if err := h.Save(path); err != nil {
		t.Fatal(err)
	}
	h2 := NewHistory(24)
	if err := h2.Load(path, base.Add(31*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(h2.snaps) != 1 {
		t.Errorf("expected expired snapshot dropped, got %d", len(h2.snaps))
	}

	if err := NewHistory(24).Load(filepath.Join(dir, "missing"), base); err != ErrNoHistoryFile {
		t.Errorf("missing file: got %v", err)
	}
	bad := filepath.Join(dir, "bad")
	os.WriteFile(bad, []byte("not gzip"), 0o644)
	h3 := NewHistory(24)
	h3.Record(base, 1, 1, 0, nil)
	if err := h3.Load(bad, base); err == nil {
		t.Error("corrupt file should error")
	}
	if len(h3.snaps) != 1 {
		t.Error("failed load must leave history unchanged")
	}
}

// Full restart flow: history before downtime is kept, downtime hours are "no data", traffic
// that happened while the collector was down is not dumped into the first hour after restart.
func TestStore_RestartKeepsHistoryAndMarksDowntime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json.gz")
	base := mustParse(t, "2026-10-01T10:00:00Z")

	s1 := &Store{}
	for i := 0; i <= 120; i++ { // 10:00-12:00, 1 GB/min
		s1.observe(map[string]costengine.Summary{"a1": agentSum(float64(i))}, base.Add(time.Duration(i)*time.Minute))
	}
	if err := s1.History().Save(path); err != nil {
		t.Fatal(err)
	}

	// Collector down 12:00-15:00. Agents keep counting (a1 reaches 300 by 15:00).
	s2 := &Store{}
	if err := s2.loadHistory(path); err != nil {
		t.Fatal(err)
	}
	restart := base.Add(5 * time.Hour)
	for i := 0; i <= 60; i++ { // 15:00-16:00, 1 GB/min
		s2.observe(map[string]costengine.Summary{"a1": agentSum(300 + float64(i))}, restart.Add(time.Duration(i)*time.Minute))
	}
	h := s2.History()
	buckets := h.Buckets(base, restart.Add(time.Hour), time.Hour)
	want := []struct {
		gb       float64
		has, cmp bool
	}{
		{60, true, true},  // 10:00
		{60, true, true},  // 11:00
		{0, false, false}, // 12:00: last snapshot was exactly 12:00, nothing observed after -> no data
		{0, false, false}, // 13:00 down
		{0, false, false}, // 14:00 down
		{60, true, true},  // 15:00 (first post-restart scrape is a baseline only)
	}
	if len(buckets) != len(want) {
		t.Fatalf("got %d buckets", len(buckets))
	}
	for i, w := range want {
		b := buckets[i]
		if b.HasData != w.has || b.Complete != w.cmp || b.CrossAZGB < w.gb-0.01 || b.CrossAZGB > w.gb+0.01 {
			t.Errorf("bucket %d (%s): got gb=%.2f has=%v complete=%v, want %+v", i, b.Start.Format("15:04"), b.CrossAZGB, b.HasData, b.Complete, w)
		}
	}
	// A 24h window across the restart is real but partial, never claimed complete.
	_, has, complete := h.EntriesRange(restart.Add(time.Hour).Add(-24*time.Hour), restart.Add(time.Hour), time.Minute)
	if !has || complete {
		t.Errorf("window across downtime: has=%v complete=%v, want true,false", has, complete)
	}
	// 24h total = 120 (before) + 60 (after); the 180 GB of downtime traffic is not attributed.
	if got := totalGB(h, base, restart.Add(time.Hour)); got < 179.9 || got > 180.1 {
		t.Errorf("total %.2f GB, want 180", got)
	}
}

func TestHistory_ConcurrentAccess(t *testing.T) {
	h := NewHistory(24)
	base := mustParse(t, "2026-10-01T10:00:00Z")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			h.Record(base.Add(time.Duration(i)*time.Second), float64(i), float64(i), 0, []costengine.Entry{fakeEntry("a", "b", "x", "y", float64(i), 0)})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			h.Buckets(base, base.Add(time.Hour), time.Hour)
			h.EntriesRange(base, base.Add(time.Hour), time.Minute)
		}
	}()
	wg.Wait()
}

// Downtime that starts mid-hour: that hour keeps its observed data but is partial.
func TestStore_DowntimeMidHourIsPartial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.gz")
	base := mustParse(t, "2026-10-01T10:00:00Z")
	s1 := &Store{}
	for i := 0; i <= 30; i++ { // 10:00-10:30
		s1.observe(map[string]costengine.Summary{"a1": agentSum(float64(i))}, base.Add(time.Duration(i)*time.Minute))
	}
	s1.History().Save(path)
	s2 := &Store{}
	if err := s2.loadHistory(path); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 10; i++ { // back at 12:00
		s2.observe(map[string]costengine.Summary{"a1": agentSum(500 + float64(i))}, base.Add(2*time.Hour+time.Duration(i)*time.Minute))
	}
	b := s2.History().Buckets(base, base.Add(2*time.Hour+10*time.Minute), time.Hour)
	if !b[0].HasData || b[0].Complete || b[0].CrossAZGB < 29.99 || b[0].CrossAZGB > 30.01 {
		t.Errorf("10:00 bucket: %+v, want 30 GB partial", b[0])
	}
	if b[1].HasData {
		t.Errorf("11:00 bucket should be no data: %+v", b[1])
	}
}
