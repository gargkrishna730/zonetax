import { describe, it, expect } from 'vitest'
import {
  emptyFilters,
  isFiltersEmpty,
  countActiveFilters,
  matchesFilters,
  applyFilters,
  buildFilterOptions,
  extractZones,
  extractNamespaces,
  extractSrcWorkloads,
  searchOptions,
  srcWorkloadKey,
  dstWorkloadKey,
  routeKey,
} from './mapFilters'
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

describe('key helpers', () => {
  it('builds stable workload/route keys', () => {
    const e = entry()
    expect(srcWorkloadKey(e)).toBe('prod|web')
    expect(dstWorkloadKey(e)).toBe('prod|db')
    expect(routeKey(e)).toBe('us-east-1a>us-east-1b')
  })
})

describe('emptyFilters/isFiltersEmpty/countActiveFilters', () => {
  it('starts empty with zero active filters', () => {
    const f = emptyFilters()
    expect(isFiltersEmpty(f)).toBe(true)
    expect(countActiveFilters(f)).toBe(0)
  })

  it('counts each selected value and each set range bound', () => {
    const f = emptyFilters()
    f.zones.add('us-east-1a')
    f.zones.add('us-east-1b')
    f.costMin = 0.1
    expect(isFiltersEmpty(f)).toBe(false)
    expect(countActiveFilters(f)).toBe(3)
  })
})

describe('matchesFilters', () => {
  it('matches an entry when either side is in the selected zone set', () => {
    const f = emptyFilters()
    f.zones.add('us-east-1b')
    expect(matchesFilters(entry(), f)).toBe(true) // dst_zone matches
  })

  it('rejects an entry when neither side matches the selected zone set', () => {
    const f = emptyFilters()
    f.zones.add('us-east-1c')
    expect(matchesFilters(entry(), f)).toBe(false)
  })

  it('applies inclusive cost range bounds', () => {
    const e = entry({ cost_usd: 0.5 })
    const f1 = emptyFilters()
    f1.costMin = 0.5
    expect(matchesFilters(e, f1)).toBe(true)
    const f2 = emptyFilters()
    f2.costMax = 0.49
    expect(matchesFilters(e, f2)).toBe(false)
  })

  it('applies inclusive traffic range bounds', () => {
    const e = entry({ gb: 5 })
    const f = emptyFilters()
    f.trafficMinGB = 5
    f.trafficMaxGB = 5
    expect(matchesFilters(e, f)).toBe(true)
    f.trafficMaxGB = 4.99
    expect(matchesFilters(e, f)).toBe(false)
  })

  it('applies srcWorkloads/dstWorkloads as direction-specific, unlike the general workloads group', () => {
    const e = entry()
    const f = emptyFilters()
    f.srcWorkloads.add(dstWorkloadKey(e)) // deliberately the WRONG side
    expect(matchesFilters(e, f)).toBe(false)
    f.srcWorkloads.clear()
    f.srcWorkloads.add(srcWorkloadKey(e))
    expect(matchesFilters(e, f)).toBe(true)
  })

  it('combines multiple active groups with AND semantics', () => {
    const e = entry()
    const f = emptyFilters()
    f.zones.add('us-east-1a')
    f.routes.add('us-east-1a>us-east-1x') // doesn't match this entry's real route
    expect(matchesFilters(e, f)).toBe(false)
  })
})

describe('applyFilters', () => {
  it('returns the original array unchanged when no filters are active (no silent hiding)', () => {
    const entries = [entry(), entry({ src_zone: 'us-east-1c' })]
    expect(applyFilters(entries, emptyFilters())).toEqual(entries)
  })

  it('narrows to only matching entries', () => {
    const entries = [entry(), entry({ dst_zone: 'us-east-1c' })]
    const f = emptyFilters()
    f.zones.add('us-east-1c')
    expect(applyFilters(entries, f)).toHaveLength(1)
  })
})

describe('buildFilterOptions', () => {
  it('produces one option per distinct value with a real count, sorted by count desc', () => {
    const entries = [
      entry({ src_zone: 'us-east-1a', dst_zone: 'us-east-1b' }),
      entry({ src_zone: 'us-east-1a', dst_zone: 'us-east-1c' }),
      entry({ src_zone: 'us-east-1b', dst_zone: 'us-east-1c' }),
    ]
    const options = buildFilterOptions(entries, extractZones, (v) => v)
    const byValue = new Map(options.map((o) => [o.value, o.count]))
    // us-east-1a appears in 2 entries, us-east-1b in 2 (1 as dst row1 + 1 as src row3), us-east-1c in 2
    expect(byValue.get('us-east-1a')).toBe(2)
    expect(byValue.get('us-east-1c')).toBe(2)
  })

  it('does not double-count a value appearing on both sides of the same entry', () => {
    const entries = [entry({ src_namespace: 'prod', dst_namespace: 'prod' })]
    const options = buildFilterOptions(entries, extractNamespaces, (v) => v)
    expect(options.find((o) => o.value === 'prod')?.count).toBe(1)
  })

  it('reflects sibling-filter narrowing when called against an already-filtered entry set', () => {
    const entries = [entry({ src_workload: 'web' }), entry({ src_workload: 'worker', dst_zone: 'us-east-1c' })]
    const narrowed = applyFilters(entries, (() => {
      const f = emptyFilters()
      f.zones.add('us-east-1c')
      return f
    })())
    const options = buildFilterOptions(narrowed, extractSrcWorkloads, (v) => v)
    expect(options).toHaveLength(1)
    expect(options[0].value).toBe('prod|worker')
  })
})

describe('searchOptions', () => {
  it('is case-insensitive and matches label or raw value', () => {
    const options = [
      { value: 'prod|web', label: 'web', count: 3 },
      { value: 'prod|db', label: 'db', count: 1 },
    ]
    expect(searchOptions(options, 'WEB')).toHaveLength(1)
    expect(searchOptions(options, 'prod|db')).toHaveLength(1)
    expect(searchOptions(options, '')).toHaveLength(2)
  })
})
