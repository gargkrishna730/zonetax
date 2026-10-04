import { describe, expect, it } from 'vitest'
import { THEME_TOKENS, contrastRatio, costLegendStops, costScale, hexToRgb, relativeLuminance } from './theme'

const L = (hex: string) => relativeLuminance(hexToRgb(hex))
const Lrgb = (rgb: string) => {
  const m = rgb.match(/(\d+),\s*(\d+),\s*(\d+)/)!
  return relativeLuminance([Number(m[1]), Number(m[2]), Number(m[3])])
}

describe('theme contrast (WCAG 2.1)', () => {
  for (const [name, t] of Object.entries(THEME_TOKENS)) {
    it(`${name}: body and muted text meet AA (4.5:1) on every surface`, () => {
      for (const surface of [t.bg, t.surface, t.surface2]) {
        expect(contrastRatio(L(t.text), L(surface))).toBeGreaterThanOrEqual(7)
        expect(contrastRatio(L(t.textMuted), L(surface))).toBeGreaterThanOrEqual(4.5)
      }
    })
    it(`${name}: accent, danger and warning text meet AA on surfaces`, () => {
      for (const c of [t.accent, t.danger, t.warning]) {
        for (const surface of [t.bg, t.surface]) expect(contrastRatio(L(c), L(surface))).toBeGreaterThanOrEqual(4.5)
      }
    })
    it(`${name}: borders are visible (3:1 non-text not required for decorative borders, but >= 1.4)`, () => {
      expect(contrastRatio(L(t.border), L(t.surface))).toBeGreaterThanOrEqual(1.4)
    })
  }
})

describe('cost scale', () => {
  it('is monotonic in lightness (cheap = light, expensive = dark)', () => {
    let prev = Infinity
    for (let i = 0; i <= 20; i++) {
      const l = Lrgb(costScale(i / 20))
      expect(l).toBeLessThan(prev + 1e-9)
      prev = l
    }
  })
  it('clamps out-of-range and NaN input', () => {
    expect(costScale(-1)).toBe(costScale(0))
    expect(costScale(5)).toBe(costScale(1))
    expect(costScale(NaN)).toBe(costScale(0))
  })
  it('expensive end is distinguishable from the dark surface (>= 1.5:1) so edges stay visible', () => {
    expect(contrastRatio(Lrgb(costScale(1)), L(THEME_TOKENS.dark.surface))).toBeGreaterThanOrEqual(1.5)
    expect(contrastRatio(Lrgb(costScale(0)), L(THEME_TOKENS.light.surface))).toBeGreaterThanOrEqual(1.1)
  })
  it('legend uses the same scale endpoints as the map', () => {
    const stops = costLegendStops()
    expect(stops[0]).toBe(costScale(0))
    expect(stops[stops.length - 1]).toBe(costScale(1))
  })
})
