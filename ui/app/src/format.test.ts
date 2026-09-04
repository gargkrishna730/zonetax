import { describe, expect, it } from 'vitest'
import { fmtAgo, fmtDuration, fmtExact, fmtGB, fmtUSD, localTzAbbrev } from './format'

describe('fmtAgo', () => {
  it('returns "never" for an undefined timestamp', () => {
    expect(fmtAgo(undefined)).toBe('never')
  })

  it('returns "just now" for sub-5-second freshness', () => {
    expect(fmtAgo(new Date().toISOString())).toBe('just now')
  })

  it('formats seconds for under a minute', () => {
    const iso = new Date(Date.now() - 30_000).toISOString()
    expect(fmtAgo(iso)).toMatch(/^\d+s ago$/)
  })

  it('formats minutes for under an hour', () => {
    const iso = new Date(Date.now() - 5 * 60_000).toISOString()
    expect(fmtAgo(iso)).toMatch(/^\d+m ago$/)
  })

  it('formats hours beyond 60 minutes instead of a huge minute count', () => {
    const iso = new Date(Date.now() - 3 * 3600_000).toISOString()
    expect(fmtAgo(iso)).toMatch(/^\d+h ago$/)
  })
})

describe('fmtExact', () => {
  it('returns null for a missing timestamp (explicit empty state, not a fabricated value)', () => {
    expect(fmtExact(undefined)).toBeNull()
  })

  it('returns null for an invalid timestamp string rather than "Invalid Date"', () => {
    expect(fmtExact('not-a-date')).toBeNull()
  })

  it('includes a timezone abbreviation so the exact time is unambiguous', () => {
    const out = fmtExact('2026-09-05T12:00:00Z')
    expect(out).not.toBeNull()
    expect(out).toContain(localTzAbbrev())
  })
})

describe('fmtDuration', () => {
  it('formats sub-minute durations as seconds', () => {
    expect(fmtDuration(45)).toBe('45s')
  })
  it('formats sub-hour durations as minutes', () => {
    expect(fmtDuration(125)).toBe('2m')
  })
  it('formats multi-hour durations as hours+minutes', () => {
    expect(fmtDuration(2 * 3600 + 15 * 60)).toBe('2h 15m')
  })
  it('formats exact-hour durations without a redundant 0m', () => {
    expect(fmtDuration(3 * 3600)).toBe('3h')
  })
  it('formats multi-day durations as days+hours', () => {
    expect(fmtDuration(26 * 3600)).toBe('1d 2h')
  })
})

describe('existing formatters still behave (regression guard)', () => {
  it('fmtUSD formats sub-cent values with extra precision', () => {
    expect(fmtUSD(0.0001234)).toBe('$0.000123')
  })
  it('fmtGB formats sub-MB values in MB', () => {
    expect(fmtGB(0.0001)).toContain('MB')
  })
})
