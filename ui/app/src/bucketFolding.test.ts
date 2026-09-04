import { describe, expect, it } from 'vitest'
import { foldHourlyToDaily } from './bucketFolding'
import type { HistoryBucket } from './types'

function hb(startISO: string, cost: number, complete = true, hasData = true): HistoryBucket {
  const start = new Date(startISO)
  const end = new Date(start.getTime() + 3600_000)
  return {
    start_utc: start.toISOString(),
    end_utc: end.toISOString(),
    cross_az_cost_usd: cost,
    cross_az_gb: cost * 10,
    same_az_gb: cost,
    complete,
    has_data: hasData,
  }
}

describe('foldHourlyToDaily', () => {
  it('sums hourly costs into a single day bucket', () => {
    const buckets = [hb('2026-09-05T00:00:00Z', 1), hb('2026-09-05T01:00:00Z', 2), hb('2026-09-05T02:00:00Z', 3)]
    const days = foldHourlyToDaily(buckets)
    expect(days).toHaveLength(1)
    expect(days[0].cross_az_cost_usd).toBeCloseTo(6)
  })

  it('splits into separate day buckets across a day boundary', () => {
    // These are >24h apart in real wall-clock hours, guaranteeing different local calendar
    // dates regardless of the test runner's timezone.
    const buckets = [hb('2026-09-05T00:00:00Z', 1), hb('2026-09-07T00:00:00Z', 2)]
    const days = foldHourlyToDaily(buckets)
    expect(days).toHaveLength(2)
  })

  it('marks a day incomplete if any contributing hour was incomplete', () => {
    const buckets = [hb('2026-09-05T00:00:00Z', 1, true), hb('2026-09-05T01:00:00Z', 1, false)]
    const days = foldHourlyToDaily(buckets)
    expect(days[0].complete).toBe(false)
  })

  it('marks a day complete only if every contributing hour was complete', () => {
    const buckets = [hb('2026-09-05T00:00:00Z', 1, true), hb('2026-09-05T01:00:00Z', 1, true)]
    const days = foldHourlyToDaily(buckets)
    expect(days[0].complete).toBe(true)
  })

  it('marks has_data true if at least one contributing hour had data', () => {
    const buckets = [hb('2026-09-05T00:00:00Z', 0, false, false), hb('2026-09-05T01:00:00Z', 1, true, true)]
    const days = foldHourlyToDaily(buckets)
    expect(days[0].has_data).toBe(true)
  })

  it('returns an empty array for empty input, not a fabricated day', () => {
    expect(foldHourlyToDaily([])).toEqual([])
  })

  it('preserves chronological order of days', () => {
    const buckets = [hb('2026-09-07T00:00:00Z', 1), hb('2026-09-05T00:00:00Z', 1)]
    const days = foldHourlyToDaily(buckets)
    expect(new Date(days[0].start_utc).getTime()).toBeLessThan(new Date(days[1].start_utc).getTime())
  })
})
