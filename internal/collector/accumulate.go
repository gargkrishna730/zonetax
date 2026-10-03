package collector

import (
	"time"

	"github.com/gargkrishna730/zonetax/internal/costengine"
)

// accumulator turns raw per-agent cumulative counters into collector-owned counters that only
// ever grow by traffic the collector actually observed. History records these, not the raw
// merged agent totals, because raw totals move for reasons that are not traffic:
//
//   - A failed scrape drops that agent's whole contribution for a cycle, and the next good
//     scrape "adds" it all back. Diffing raw totals turned that into a fake cost spike.
//   - A node scale-down removes an agent permanently, so totals drop and were read as a
//     counter reset.
//   - After a collector restart, agents have kept counting. Diffing the last saved snapshot
//     against the first new one would silently attribute all downtime traffic to one hour.
//
// Rules: an agent seen for the first time (new pod, or first cycle after a collector restart)
// only establishes a baseline and contributes 0. For a known agent, each route contributes
// v-prev, or v if the counter went down (container restart inside the same pod). A failed
// scrape leaves the agent's state untouched, so the next good scrape counts exactly the
// traffic that happened in between. Agents unseen for agentForgetAfter are dropped.
type accumulator struct {
	agents map[string]*agentState
	obs    map[entryKey]entryVal // monotonic observed totals per route
	sameGB float64               // monotonic observed same-AZ GB
}

type agentState struct {
	lastSeen time.Time
	routes   map[entryKey]entryVal
	sameGB   float64
}

const agentForgetAfter = time.Hour

func newAccumulator() *accumulator {
	return &accumulator{agents: map[string]*agentState{}, obs: map[entryKey]entryVal{}}
}

// seed continues the monotonic totals from a loaded snapshot so history stays continuous
// across a collector restart.
func (a *accumulator) seed(s snapshot) {
	for k, v := range s.entries {
		a.obs[k] = v
	}
	a.sameGB = s.sameAZGB
}

// observe folds one successfully scraped agent's cumulative summary into the observed totals.
func (a *accumulator) observe(agent string, s costengine.Summary, now time.Time) {
	st, known := a.agents[agent]
	if !known {
		st = &agentState{routes: map[entryKey]entryVal{}}
		a.agents[agent] = st
	}
	for _, e := range s.Entries {
		k := keyOf(e)
		v := entryVal{gb: e.GB, costUSD: e.CostUSD}
		if known {
			prev, had := st.routes[k]
			d := v
			if had && v.gb >= prev.gb {
				d = entryVal{gb: v.gb - prev.gb, costUSD: v.costUSD - prev.costUSD}
			}
			if d.gb > 0 || d.costUSD > 0 {
				o := a.obs[k]
				a.obs[k] = entryVal{gb: o.gb + d.gb, costUSD: o.costUSD + d.costUSD}
			}
		}
		st.routes[k] = v
	}
	if known {
		if s.TotalSameAZGB >= st.sameGB {
			a.sameGB += s.TotalSameAZGB - st.sameGB
		} else {
			a.sameGB += s.TotalSameAZGB
		}
	}
	st.sameGB = s.TotalSameAZGB
	st.lastSeen = now
}

func (a *accumulator) prune(now time.Time) {
	for name, st := range a.agents {
		if now.Sub(st.lastSeen) > agentForgetAfter {
			delete(a.agents, name)
		}
	}
}

// totals returns the current observed totals in the shape History.Record expects.
func (a *accumulator) totals() (cost, gb, same float64, entries []costengine.Entry) {
	entries = make([]costengine.Entry, 0, len(a.obs))
	for k, v := range a.obs {
		cost += v.costUSD
		gb += v.gb
		entries = append(entries, costengine.Entry{
			SrcZone: k.srcZone, DstZone: k.dstZone,
			SrcNamespace: k.srcNamespace, SrcWorkload: k.srcWorkload,
			DstNamespace: k.dstNamespace, DstWorkload: k.dstWorkload,
			GB: v.gb, CostUSD: v.costUSD,
		})
	}
	return cost, gb, a.sameGB, entries
}

func keyOf(e costengine.Entry) entryKey {
	return entryKey{
		srcZone: e.SrcZone, dstZone: e.DstZone,
		srcNamespace: e.SrcNamespace, srcWorkload: e.SrcWorkload,
		dstNamespace: e.DstNamespace, dstWorkload: e.DstWorkload,
	}
}
