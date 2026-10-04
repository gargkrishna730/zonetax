// Colour system for the dashboard. One sequential, colour-blind-safe scale encodes route cost
// everywhere (map edges, cost pills, drill-down dots, legend), so a colour always means the same
// thing. State is never encoded by recolouring: selected routes get a thicker outline,
// partial/incomplete data gets a dashed stroke, and red is reserved for errors only.
//
// The cost ramp is a perceptually ordered light-to-dark "magma"-style scale (pale yellow ->
// orange -> magenta -> purple). It stays distinguishable under deuteranopia/protanopia/tritanopia
// because it varies mostly in lightness, unlike the old red/green ramp.

export type Theme = 'dark' | 'light'

// Anchor stops sampled from matplotlib's magma (0.95 -> 0.25), reversed so cheap = light.
const COST_STOPS: [number, number, number][] = [
  [252, 236, 168], // cheapest
  [254, 176, 120],
  [241, 96, 93],
  [183, 55, 121],
  [114, 31, 129], // most expensive
]

function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t
}

/** Cost colour for a value relative to the most expensive item shown (0 = cheapest, 1 = most
 * expensive). Uses a sqrt curve so a single dominant route doesn't leave everything else at the
 * palest stop. */
export function costScale(relative: number): string {
  const t = Math.sqrt(Math.max(0, Math.min(1, Number.isFinite(relative) ? relative : 0)))
  const scaled = t * (COST_STOPS.length - 1)
  const i = Math.min(COST_STOPS.length - 2, Math.floor(scaled))
  const local = scaled - i
  const [r1, g1, b1] = COST_STOPS[i]
  const [r2, g2, b2] = COST_STOPS[i + 1]
  return `rgb(${Math.round(lerp(r1, r2, local))}, ${Math.round(lerp(g1, g2, local))}, ${Math.round(lerp(b1, b2, local))})`
}

// Note: text is never drawn directly on a cost colour. Any light-to-dark scale passes through a
// mid-luminance band where neither black nor white text reaches 4.5:1, so cost pills use a
// neutral theme surface with a cost-coloured border and dot instead.

/** Five evenly spaced legend swatches taken from the exact same scale the map uses. */
export function costLegendStops(): string[] {
  return [0, 0.0625, 0.25, 0.5625, 1].map(costScale) // sqrt-spaced so swatches look even
}

// --- WCAG helpers (exported for tests) ---------------------------------------------------------

export function relativeLuminance([r, g, b]: [number, number, number]): number {
  const ch = (v: number) => {
    const s = v / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * ch(r) + 0.7152 * ch(g) + 0.0722 * ch(b)
}

export function contrastRatio(l1: number, l2: number): number {
  const [a, b] = l1 > l2 ? [l1, l2] : [l2, l1]
  return (a + 0.05) / (b + 0.05)
}

export function hexToRgb(hex: string): [number, number, number] {
  const h = hex.replace('#', '')
  const full = h.length === 3 ? h.split('').map((c) => c + c).join('') : h
  return [parseInt(full.slice(0, 2), 16), parseInt(full.slice(2, 4), 16), parseInt(full.slice(4, 6), 16)]
}

/** Theme tokens mirrored from tokens.css, so tests can assert contrast on the real values. */
export const THEME_TOKENS: Record<Theme, Record<string, string>> = {
  dark: {
    bg: '#0b1018',
    surface: '#121926',
    surface2: '#1a2333',
    border: '#2a3548',
    text: '#e8edf5',
    textMuted: '#9aa7bb',
    accent: '#2dd4bf',
    danger: '#f87171',
    warning: '#fbbf24',
  },
  light: {
    bg: '#f5f7fa',
    surface: '#ffffff',
    surface2: '#eef2f7',
    border: '#b8c4d4',
    text: '#0f172a',
    textMuted: '#4a5568',
    accent: '#0f766e',
    danger: '#b91c1c',
    warning: '#92400e',
  },
}
