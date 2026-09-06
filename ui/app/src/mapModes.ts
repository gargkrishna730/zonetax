// Route selection "modes" for the Cross-AZ Service Map, plus the Max-connections aggregation
// cap. Pure functions so they're independently testable from ReactFlow/rendering concerns —
// mirrors the reference service map's "Max connections" control, generalized into named modes
// since ZoneTax's data is exclusively cross-AZ cost/traffic (no connection-status/error axis to
// mode-switch on the way the reference's APM data has).
import type { MapEntry } from './types'
import { srcWorkloadKey, dstWorkloadKey } from './mapFilters'

export type MapMode = 'all' | 'top-cost' | 'top-traffic' | 'highest-cost-workloads'

export const MAP_MODES: { value: MapMode; label: string; hint: string }[] = [
  { value: 'all', label: 'All routes', hint: 'Every route with any tracked cost or traffic — nothing hidden.' },
  { value: 'top-cost', label: 'Top cost routes', hint: 'The highest-cost routes, up to the Max connections limit below.' },
  { value: 'top-traffic', label: 'Top traffic routes', hint: 'The highest-traffic (GB) routes, up to the Max connections limit below.' },
  { value: 'highest-cost-workloads', label: 'Highest-cost workloads', hint: 'Every route touching the top 5 highest-total-cost workloads (by combined outbound + inbound spend).' },
]

/** Applies a mode + Max-connections cap to an entry set, returning the routes to actually draw
 * PLUS how many were left out (so the UI can show "X of Y routes shown, raise Max connections to
 * see more" rather than silently dropping data — the brief's explicit "expose any aggregation
 * threshold" requirement). `maxConnections` caps how many AGGREGATED ROUTES are drawn — matching
 * the reference's own "Max connections" semantics, which caps rendered graph edges, not
 * underlying data rows. Pass Infinity for no cap.
 *
 * `routeKeyOf` identifies what one "route" (one graph edge) means for the CURRENTLY ACTIVE view
 * — (src_zone,dst_zone) in Zone->Zone view, (src workload,dst workload) in Workload->Workload
 * view. These are genuinely different granularities (3 zones can have at most 6 directed
 * routes, but a workload view can easily have dozens) — a real bug caught via Playwright against
 * live data: capping always by zone-pair left the Workload view's 16 real distinct routes
 * completely uncapped (and reported a stale, wrong "6 routes" hint) whenever the cap was set
 * below 16, since the code was silently applying the ZONE route count/cap regardless of which
 * view was actually being drawn. Defaults to the zone-pair key for backward compatibility with
 * existing zone-view-only callers/tests.
 *
 * Note: there is deliberately no "Cross-AZ only" mode — GET /api/v1/map (this data's source)
 * only ever returns billed cross-AZ entries by construction (same-AZ traffic is never included
 * as a routed entry there), so such a mode could never differ from "All routes". An earlier
 * version of this file had one; it was removed as dead/misleading UI rather than kept "for
 * clarity" once that was noticed — a control that can never change its own output is worse than
 * no control. */
export function applyMapMode(
  entries: MapEntry[],
  mode: MapMode,
  maxConnections: number,
  routeKeyOf: (e: MapEntry) => string = (e) => e.src_zone + '>' + e.dst_zone,
): { shown: MapEntry[]; hiddenCount: number; totalRoutes: number } {
  let candidates = entries
  if (mode === 'highest-cost-workloads') {
    const workloadCost = new Map<string, number>()
    for (const e of entries) {
      workloadCost.set(srcWorkloadKey(e), (workloadCost.get(srcWorkloadKey(e)) || 0) + e.cost_usd)
      workloadCost.set(dstWorkloadKey(e), (workloadCost.get(dstWorkloadKey(e)) || 0) + e.cost_usd)
    }
    const top5 = new Set(
      Array.from(workloadCost.entries())
        .sort((a, b) => b[1] - a[1])
        .slice(0, 5)
        .map(([k]) => k),
    )
    candidates = entries.filter((e) => top5.has(srcWorkloadKey(e)) || top5.has(dstWorkloadKey(e)))
  }

  const sorted =
    mode === 'top-cost'
      ? [...candidates].sort((a, b) => b.cost_usd - a.cost_usd)
      : mode === 'top-traffic'
        ? [...candidates].sort((a, b) => b.gb - a.gb)
        : candidates

  const totalRoutes = countDistinctRoutes(sorted, routeKeyOf)
  if (!Number.isFinite(maxConnections) || totalRoutes <= maxConnections) {
    return { shown: sorted, hiddenCount: 0, totalRoutes }
  }
  // Cap by DISTINCT route (per routeKeyOf), keeping every entry belonging to an admitted
  // route — never truncate mid-route, which would silently understate one route's real total.
  const seen = new Set<string>()
  const shown: MapEntry[] = []
  for (const e of sorted) {
    const key = routeKeyOf(e)
    if (!seen.has(key)) {
      if (seen.size >= maxConnections) continue
      seen.add(key)
    }
    shown.push(e)
  }
  return { shown, hiddenCount: totalRoutes - seen.size, totalRoutes }
}

function countDistinctRoutes(entries: MapEntry[], routeKeyOf: (e: MapEntry) => string): number {
  const seen = new Set<string>()
  for (const e of entries) seen.add(routeKeyOf(e))
  return seen.size
}
