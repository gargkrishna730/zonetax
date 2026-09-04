// Formatting + coloring helpers shared across the dashboard. Kept dependency-free (no d3-format
// etc.) since these are simple enough to hand-roll and it avoids another bundle dependency.

/** Green (cheap) -> yellow -> red (expensive), matching the previous vanilla-JS dashboard's
 * d3.interpolateRdYlGn(1 - ratio), reimplemented by hand so this package doesn't need d3 just
 * for one color ramp. Anchor stops are exact values sampled directly from
 * d3.interpolateRdYlGn(1), (0.75), (0.5), (0.25), (0) — verified via `node -e` against the real
 * d3 package, not guessed, so the two dashboards' color scales actually match. */
const RDYLGN_STOPS: [number, number, number][] = [
  [0, 104, 55], // ratio=0, cheapest  == d3.interpolateRdYlGn(1)
  [133, 203, 103], // ratio=0.25       == d3.interpolateRdYlGn(0.75)
  [249, 247, 174], // ratio=0.5, mid   == d3.interpolateRdYlGn(0.5)
  [248, 141, 82], // ratio=0.75        == d3.interpolateRdYlGn(0.25)
  [165, 0, 38], // ratio=1, priciest  == d3.interpolateRdYlGn(0)
]

function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t
}

export function costColor(cost: number, maxCost: number): string {
  if (!maxCost || maxCost <= 0) return 'rgb(74, 222, 128)' // fallback: --good green
  const ratio = Math.min(1, Math.max(0, cost / maxCost)) // 0 = cheapest, 1 = most expensive
  const scaled = ratio * (RDYLGN_STOPS.length - 1)
  const i = Math.min(RDYLGN_STOPS.length - 2, Math.floor(scaled))
  const localT = scaled - i
  const [r1, g1, b1] = RDYLGN_STOPS[i]
  const [r2, g2, b2] = RDYLGN_STOPS[i + 1]
  const r = Math.round(lerp(r1, r2, localT))
  const g = Math.round(lerp(g1, g2, localT))
  const b = Math.round(lerp(b1, b2, localT))
  return `rgb(${r}, ${g}, ${b})`
}

export function fmtUSD(v: number): string {
  if (v < 0.01 && v > 0) return '$' + v.toFixed(6)
  return '$' + v.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

export function fmtUSDShort(v: number): string {
  if (v < 0.01 && v > 0) return '<$0.01'
  if (v >= 1000) return '$' + (v / 1000).toFixed(1) + 'k'
  return '$' + v.toFixed(v < 10 ? 2 : 0)
}

export function fmtGB(v: number): string {
  if (v < 0.001 && v > 0) return (v * 1000).toFixed(3) + ' MB'
  return v.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + ' GB'
}

export function fmtAgo(iso?: string): string {
  if (!iso) return 'never'
  const secs = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000))
  if (secs < 5) return 'just now'
  if (secs < 60) return secs + 's ago'
  return Math.round(secs / 60) + 'm ago'
}
