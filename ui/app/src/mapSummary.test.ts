import { describe, it, expect } from 'vitest'
import { computeMapSummary } from './mapSummary'
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

describe('computeMapSummary', () => {
  it('returns honest zero/null values for an empty entry set', () => {
    const s = computeMapSummary([])
    expect(s.totalCostUSD).toBe(0)
    expect(s.totalGB).toBe(0)
    expect(s.routeCount).toBe(0)
    expect(s.affectedWorkloadCount).toBe(0)
    expect(s.highestCostRoute).toBeNull()
    expect(s.avgCostPerGB).toBeNull() // must not divide by zero / fabricate a rate
  })

  it('sums cost and traffic across all entries', () => {
    const s = computeMapSummary([entry({ cost_usd: 1, gb: 5 }), entry({ cost_usd: 2, gb: 5, dst_zone: 'us-east-1c' })])
    expect(s.totalCostUSD).toBeCloseTo(3)
    expect(s.totalGB).toBeCloseTo(10)
  })

  it('counts distinct routes, not raw entries', () => {
    const s = computeMapSummary([entry(), entry()]) // same src/dst zone twice
    expect(s.routeCount).toBe(1)
  })

  it('counts distinct affected workloads across both src and dst', () => {
    const s = computeMapSummary([
      entry({ src_workload: 'a', dst_workload: 'b' }),
      entry({ src_workload: 'b', dst_workload: 'c', dst_zone: 'us-east-1c' }),
    ])
    // Distinct workload keys: prod|a, prod|b, prod|c = 3
    expect(s.affectedWorkloadCount).toBe(3)
  })

  it('finds the real highest-cost route entry', () => {
    const expensive = entry({ cost_usd: 9, dst_zone: 'us-east-1c' })
    const s = computeMapSummary([entry({ cost_usd: 1 }), expensive])
    expect(s.highestCostRoute).toBe(expensive)
  })

  it('computes average cost per GB from real totals, not a hardcoded rate', () => {
    const s = computeMapSummary([entry({ cost_usd: 2, gb: 10 }), entry({ cost_usd: 2, gb: 10, dst_zone: 'us-east-1c' })])
    expect(s.avgCostPerGB).toBeCloseTo(0.2) // 4 total cost / 20 total gb
  })
})
