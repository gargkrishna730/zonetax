import { describe, it, expect } from 'vitest'
import { routeColor, sameAZColor, ROUTE_COLOR_LEGEND } from './routeColors'

describe('routeColor', () => {
  it('state overrides always win regardless of relativeCost', () => {
    const selected = routeColor('selected', 0.9)
    const stale = routeColor('stale', 0.9)
    const unavailable = routeColor('unavailable', 0.9)
    expect(selected).toBe(routeColor('selected', 0))
    expect(stale).toBe(routeColor('stale', 1))
    expect(unavailable).toBe(routeColor('unavailable', 1))
  })

  it('produces a distinct pale color at relativeCost=0 and a distinct hot color at relativeCost=1', () => {
    const cheap = routeColor('normal', 0)
    const expensive = routeColor('normal', 1)
    expect(cheap).not.toBe(expensive)
  })

  it('clamps out-of-range relativeCost instead of producing invalid output', () => {
    expect(() => routeColor('normal', -5)).not.toThrow()
    expect(() => routeColor('normal', 5)).not.toThrow()
    expect(routeColor('normal', -5)).toBe(routeColor('normal', 0))
    expect(routeColor('normal', 5)).toBe(routeColor('normal', 1))
  })

  it('returns a valid rgb() string for every state', () => {
    for (const state of ['normal', 'selected', 'stale', 'unavailable'] as const) {
      expect(routeColor(state, 0.5)).toMatch(/^rgb\(\d+, \d+, \d+\)$/)
    }
  })
})

describe('sameAZColor', () => {
  it('is a distinct, valid color from every normal-state route color', () => {
    expect(sameAZColor()).toMatch(/^rgb\(\d+, \d+, \d+\)$/)
    expect(sameAZColor()).not.toBe(routeColor('normal', 0))
    expect(sameAZColor()).not.toBe(routeColor('normal', 1))
  })
})

describe('ROUTE_COLOR_LEGEND', () => {
  it('documents every color used by routeColor/sameAZColor with a human label', () => {
    expect(ROUTE_COLOR_LEGEND.length).toBeGreaterThanOrEqual(6)
    for (const entry of ROUTE_COLOR_LEGEND) {
      expect(entry.color).toMatch(/^rgb\(/)
      expect(entry.label.length).toBeGreaterThan(0)
    }
  })
})
