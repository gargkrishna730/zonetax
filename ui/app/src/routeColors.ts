// Route/edge color semantics for the Cross-AZ Service Map, per the brief's explicit palette:
// pale blue-gray for low-cost/low-volume, strong blue for selected, green for same-AZ/zero-cost
// comparison, amber for warning/stale/incomplete/unusual, orange/red for expensive cross-AZ
// hotspots, gray for unavailable data. Kept as a small pure module (not reusing the existing
// green->red costColor ramp in format.ts, which encodes a different continuous relative-cost
// scale) so route STATE (selected/stale/unavailable) and route COST (cheap->expensive) are two
// independent, composable concerns rather than one ramp trying to represent both.
export type RouteColorState = 'normal' | 'selected' | 'stale' | 'unavailable'

const PALE_BLUE_GRAY = 'rgb(148, 163, 184)' // low-cost/low-volume baseline
const STRONG_BLUE = 'rgb(59, 130, 246)' // selected route
const GREEN = 'rgb(74, 222, 128)' // same-AZ / zero-cost comparison
const AMBER = 'rgb(245, 158, 11)' // warning / stale / incomplete / unusual
const ORANGE = 'rgb(249, 115, 22)' // expensive (mid-high band)
const RED = 'rgb(239, 68, 68)' // expensive (hotspot band)
const GRAY = 'rgb(100, 116, 139)' // unavailable data

/** Resolves a route's edge color from its state and, for the normal case, its relative cost
 * (0=cheapest shown, 1=most expensive shown) — a smooth pale-blue-gray -> amber -> orange -> red
 * ramp so the most expensive routes are unmistakably hot without needing a legend lookup, while
 * state (selected/stale/unavailable) always overrides cost-based coloring since those describe
 * something more urgent than "how expensive is this route" (a selected route must look
 * selected regardless of its cost). */
export function routeColor(state: RouteColorState, relativeCost = 0): string {
  if (state === 'selected') return STRONG_BLUE
  if (state === 'stale') return AMBER
  if (state === 'unavailable') return GRAY
  const t = Math.max(0, Math.min(1, relativeCost))
  if (t < 0.01) return PALE_BLUE_GRAY
  if (t < 0.4) return lerpColor(PALE_BLUE_GRAY, AMBER, t / 0.4)
  if (t < 0.75) return lerpColor(AMBER, ORANGE, (t - 0.4) / 0.35)
  return lerpColor(ORANGE, RED, (t - 0.75) / 0.25)
}

export function sameAZColor(): string {
  return GREEN
}

function parseRgb(s: string): [number, number, number] {
  const m = s.match(/rgb\((\d+),\s*(\d+),\s*(\d+)\)/)
  if (!m) return [0, 0, 0]
  return [Number(m[1]), Number(m[2]), Number(m[3])]
}

function lerpColor(a: string, b: string, t: number): string {
  const [r1, g1, b1] = parseRgb(a)
  const [r2, g2, b2] = parseRgb(b)
  const r = Math.round(r1 + (r2 - r1) * t)
  const g = Math.round(g1 + (g2 - g1) * t)
  const bl = Math.round(b1 + (b2 - b1) * t)
  return `rgb(${r}, ${g}, ${bl})`
}

export const ROUTE_COLOR_LEGEND = [
  { color: PALE_BLUE_GRAY, label: 'Low cost / low volume' },
  { color: AMBER, label: 'Moderate cost, or warning/stale/incomplete data' },
  { color: ORANGE, label: 'Elevated cost' },
  { color: RED, label: 'Highest cost (hotspot)' },
  { color: STRONG_BLUE, label: 'Selected route' },
  { color: GREEN, label: 'Same-AZ / zero-cost comparison' },
  { color: GRAY, label: 'Unavailable data' },
]
