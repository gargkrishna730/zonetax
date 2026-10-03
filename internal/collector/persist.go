package collector

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// On-disk format for History: gzip-compressed JSON, versioned. Route keys are stored once in a
// table and referenced by index so a week of 5-minute snapshots stays around a few hundred KB.

const historyFileVersion = 1

type historyFile struct {
	Version  int         `json:"version"`
	SavedAt  time.Time   `json:"saved_at"`
	MaxHours int         `json:"max_hours"`
	Keys     [][6]string `json:"keys"`
	Snaps    []fileSnap  `json:"snaps"`
}

type fileSnap struct {
	At       int64        `json:"t"` // unix seconds
	Cost     float64      `json:"c"`
	GB       float64      `json:"g"`
	Same     float64      `json:"s"`
	AfterGap bool         `json:"gap,omitempty"`
	Entries  [][3]float64 `json:"e,omitempty"` // [keyIndex, gb, costUSD]
}

// Save writes the history atomically: a temp file in the same directory is written, synced,
// then renamed over path, so a crash mid-write never leaves a truncated file behind.
func (h *History) Save(path string) error {
	h.mu.RLock()
	f := historyFile{Version: historyFileVersion, SavedAt: time.Now().UTC(), MaxHours: h.maxHours}
	idx := map[entryKey]int{}
	for _, s := range h.snaps {
		fs := fileSnap{At: s.at.Unix(), Cost: s.crossAZCost, GB: s.crossAZGB, Same: s.sameAZGB, AfterGap: s.afterGap}
		for k, v := range s.entries {
			i, ok := idx[k]
			if !ok {
				i = len(f.Keys)
				idx[k] = i
				f.Keys = append(f.Keys, [6]string{k.srcZone, k.dstZone, k.srcNamespace, k.srcWorkload, k.dstNamespace, k.dstWorkload})
			}
			fs.Entries = append(fs.Entries, [3]float64{float64(i), v.gb, v.costUSD})
		}
		f.Snaps = append(f.Snaps, fs)
	}
	h.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".history-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	zw := gzip.NewWriter(tmp)
	if err := json.NewEncoder(zw).Encode(&f); err != nil {
		tmp.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ErrNoHistoryFile is returned by Load when path does not exist (first start).
var ErrNoHistoryFile = errors.New("no history file")

// Load replaces this History's snapshots with those in path, dropping anything older than
// maxHours before now. On any error the History is left unchanged.
func (h *History) Load(path string, now time.Time) error {
	fh, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNoHistoryFile
	}
	if err != nil {
		return err
	}
	defer fh.Close()
	zr, err := gzip.NewReader(fh)
	if err != nil {
		return fmt.Errorf("history file %s: %w", path, err)
	}
	var f historyFile
	if err := json.NewDecoder(zr).Decode(&f); err != nil {
		return fmt.Errorf("history file %s: %w", path, err)
	}
	if f.Version != historyFileVersion {
		return fmt.Errorf("history file %s: unsupported version %d", path, f.Version)
	}

	keys := make([]entryKey, len(f.Keys))
	for i, k := range f.Keys {
		keys[i] = entryKey{srcZone: k[0], dstZone: k[1], srcNamespace: k[2], srcWorkload: k[3], dstNamespace: k[4], dstWorkload: k[5]}
	}
	cutoff := now.Add(-time.Duration(h.maxHours) * time.Hour)
	snaps := make([]snapshot, 0, len(f.Snaps))
	for _, fs := range f.Snaps {
		at := time.Unix(fs.At, 0).UTC()
		if at.Before(cutoff) || at.After(now) {
			continue
		}
		s := snapshot{at: at, crossAZCost: fs.Cost, crossAZGB: fs.GB, sameAZGB: fs.Same, afterGap: fs.AfterGap,
			entries: make(map[entryKey]entryVal, len(fs.Entries))}
		for _, e := range fs.Entries {
			i := int(e[0])
			if i < 0 || i >= len(keys) {
				return fmt.Errorf("history file %s: bad key index %d", path, i)
			}
			s.entries[keys[i]] = entryVal{gb: e[1], costUSD: e[2]}
		}
		snaps = append(snaps, s)
	}
	sort.SliceStable(snaps, func(i, j int) bool { return snaps[i].at.Before(snaps[j].at) })
	if len(snaps) > 0 {
		snaps[0].afterGap = false // nothing before it to have a gap from
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.snaps = snaps
	return nil
}

// last returns a copy of the most recent snapshot, if any.
func (h *History) last() (snapshot, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.snaps) == 0 {
		return snapshot{}, false
	}
	return h.snaps[len(h.snaps)-1], true
}
