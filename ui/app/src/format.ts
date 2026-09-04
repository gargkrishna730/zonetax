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
  if (secs < 3600) return Math.round(secs / 60) + 'm ago'
  return Math.round(secs / 3600) + 'h ago'
}

/** Local IANA timezone abbreviation for this browser, e.g. "IST", "PDT" — used so every exact
 * timestamp shown in the UI states unambiguously which zone it's in rather than leaving the
 * viewer to guess whether a time is local or UTC. */
export function localTzAbbrev(): string {
  try {
    const parts = new Intl.DateTimeFormat(undefined, { timeZoneName: 'short' }).formatToParts(new Date())
    return parts.find((p) => p.type === 'timeZoneName')?.value ?? 'local'
  } catch {
    return 'local'
  }
}

/** Exact local timestamp, e.g. "Sep 5, 5:45:12 PM IST" — the "exact collection time" the
 * dashboard now shows alongside every relative "Ns ago" freshness label. Returns null for a
 * missing/invalid ISO string so callers can render an explicit empty state instead of "Invalid
 * Date". */
export function fmtExact(iso?: string): string | null {
  if (!iso) return null
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return null
  const datePart = d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
  const timePart = d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit', second: '2-digit' })
  return `${datePart}, ${timePart} ${localTzAbbrev()}`
}

/** Formats a duration in seconds as a short human string, e.g. "45s", "3m", "2h 15m" — used for
 * "scrape every Ns" and "since agent started Nh Nm" style session/collector metrics, distinct
 * from calendar-day framing. */
export function fmtDuration(seconds: number): string {
  if (seconds < 60) return Math.round(seconds) + 's'
  const mins = Math.floor(seconds / 60)
  if (mins < 60) return mins + 'm'
  const hrs = Math.floor(mins / 60)
  const remMins = mins % 60
  if (hrs < 24) return remMins > 0 ? `${hrs}h ${remMins}m` : `${hrs}h`
  const days = Math.floor(hrs / 24)
  const remHrs = hrs % 24
  return remHrs > 0 ? `${days}d ${remHrs}h` : `${days}d`
}

/** Compact date label for a chart axis / bucket, e.g. "Sep 5" or "5 PM" depending on span. */
export function fmtBucketLabel(iso: string, granularity: 'hour' | 'day'): string {
  const d = new Date(iso)
  if (granularity === 'day') return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
  return d.toLocaleTimeString(undefined, { hour: 'numeric' })
}
