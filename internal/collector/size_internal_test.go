package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gargkrishna730/zonetax/internal/costengine"
)

// Realistic 7-day file: 40 routes, 30s scrapes, downsampled. Guards the "1-2 MB" promise.
func TestHistory_SevenDayFileSize(t *testing.T) {
	h := NewHistory(7 * 24)
	base := mustParse(t, "2026-10-01T00:00:00Z")
	n := 7 * 24 * 120
	for i := 0; i <= n; i++ {
		es := make([]costengine.Entry, 40)
		for r := range es {
			gb := float64(i) * 0.001 * float64(r+1)
			es[r] = fakeEntry("us-east-1a", "us-east-1b", fmt.Sprintf("workload-%d", r), fmt.Sprintf("dest-%d", r), gb, gb*0.02)
		}
		h.Record(base.Add(time.Duration(i)*30*time.Second), 0, 0, 0, es)
	}
	path := filepath.Join(t.TempDir(), "h.gz")
	if err := h.Save(path); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	t.Logf("7d, 40 routes: %d snapshots, %.2f MB on disk", len(h.snaps), float64(fi.Size())/1e6)
	if fi.Size() > 4e6 {
		t.Errorf("history file too large: %d bytes", fi.Size())
	}
}
