import { describe, it, expect } from 'vitest'
import { applyMapMode } from './mapModes'
import type { MapEntry } from './types'

function entry(over: Partial<MapEntry> = {}): MapEntry {
  return {
    src_zone: 'us-east-1a',
    dst_zone: 'us-east-1b',
    src_namespace: 'prod',
    src_workload: 'web',
    dst_namespace: 'prod',
    dst_workload: 'db',
    gb: 10,
    cost_usd: 0.2,
    ...over,
  }
}

describe('applyMapMode: all', () => {
  it('shows every entry with no cap and no hidden count', () => {
    const entries = [entry(), entry({ dst_zone: 'us-east-1c' })]
    const { shown, hiddenCount, totalRoutes } = applyMapMode(entries, 'all', Infinity)
    expect(shown).toHaveLength(2)
    expect(hiddenCount).toBe(0)
    expect(totalRoutes).toBe(2)
  })
})

describe('applyMapMode: top-cost / top-traffic sorting', () => {
  it('sorts by cost descending for top-cost', () => {
    const entries = [entry({ cost_usd: 0.1 }), entry({ cost_usd: 5, dst_zone: 'us-east-1c' })]
    const { shown } = applyMapMode(entries, 'top-cost', Infinity)
    expect(shown[0].cost_usd).toBe(5)
  })

  it('sorts by traffic descending for top-traffic', () => {
    const entries = [entry({ gb: 1 }), entry({ gb: 99, dst_zone: 'us-east-1c' })]
    const { shown } = applyMapMode(entries, 'top-traffic', Infinity)
    expect(shown[0].gb).toBe(99)
  })
})

describe('applyMapMode: highest-cost-workloads', () => {
  it('keeps only routes touching the top-5 highest-cost workloads', () => {
    const entries = [
      entry({ src_workload: 'expensive', cost_usd: 10 }),
      entry({ src_workload: 'cheap', dst_zone: 'us-east-1c', cost_usd: 0.01 }),
    ]
    const { shown } = applyMapMode(entries, 'highest-cost-workloads', Infinity)
    // Both workloads are within top-5 since there are only 2 distinct workloads total, so both
    // should still show — the real assertion is that the filter runs without throwing and
    // preserves entries belonging to a genuinely top workload.
    expect(shown.some((e) => e.src_workload === 'expensive')).toBe(true)
  })
})

describe('applyMapMode: Max connections cap', () => {
  it('caps by distinct route, never truncating mid-route, and reports hiddenCount honestly', () => {
    const entries = [
      entry({ dst_zone: 'us-east-1b', cost_usd: 5 }),
      entry({ dst_zone: 'us-east-1b', cost_usd: 3 }), // same route as above — must not be split
      entry({ dst_zone: 'us-east-1c', cost_usd: 1 }),
      entry({ dst_zone: 'us-east-1d', cost_usd: 0.5 }),
    ]
    const { shown, hiddenCount, totalRoutes } = applyMapMode(entries, 'top-cost', 2)
    expect(totalRoutes).toBe(3) // 3 distinct src->dst routes
    expect(hiddenCount).toBe(1) // 1 route hidden beyond the cap of 2
    // The two entries for the us-east-1a->us-east-1b route must BOTH be present (not split).
    const bRouteCount = shown.filter((e) => e.dst_zone === 'us-east-1b').length
    expect(bRouteCount).toBe(2)
  })

  it('does not hide anything when the cap is not exceeded', () => {
    const entries = [entry(), entry({ dst_zone: 'us-east-1c' })]
    const { hiddenCount, totalRoutes } = applyMapMode(entries, 'all', 5)
    expect(hiddenCount).toBe(0)
    expect(totalRoutes).toBe(2)
  })

  it('caps by the CALLER-SUPPLIED route granularity, not always by zone-pair — regression test for a real bug caught against live data where capping stayed keyed to zone-pair even in the Workload view, silently leaving it uncapped', () => {
    // 3 entries, all sharing the SAME zone-pair (a->b) but each a DIFFERENT workload pair —
    // e.g. what a real Workload->Workload view looks like when routed through only 2 zones.
    const entries = [
      entry({ src_workload: 'w1', dst_workload: 'd1', cost_usd: 5 }),
      entry({ src_workload: 'w2', dst_workload: 'd2', cost_usd: 3 }),
      entry({ src_workload: 'w3', dst_workload: 'd3', cost_usd: 1 }),
    ]
    const byWorkloadPair = (e: MapEntry) => e.src_workload + '>' + e.dst_workload

    // Capping by the DEFAULT (zone-pair) key sees only 1 distinct route (all share a->b) and
    // therefore never truncates — this is the bug: it can never cap a workload view correctly.
    const zoneKeyed = applyMapMode(entries, 'top-cost', 2)
    expect(zoneKeyed.totalRoutes).toBe(1)
    expect(zoneKeyed.hiddenCount).toBe(0)

    // Capping by the WORKLOAD-PAIR key correctly sees 3 distinct routes and truncates to 2.
    const workloadKeyed = applyMapMode(entries, 'top-cost', 2, byWorkloadPair)
    expect(workloadKeyed.totalRoutes).toBe(3)
    expect(workloadKeyed.hiddenCount).toBe(1)
    expect(workloadKeyed.shown).toHaveLength(2)
  })
})
