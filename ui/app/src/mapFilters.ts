// Pure filter model for the Cross-AZ Service Map: builds selectable filter options (with real
// counts) from the current MapEntry set, and applies the active filter selection. Kept
// dependency-free and framework-free so it's directly unit-testable without React/DOM.
import type { MapEntry } from './types'

export function workloadKey(namespace: string, workload: string): string {
  return (namespace || '') + '|' + (workload || '(unknown)')
}
export function srcWorkloadKey(e: MapEntry): string {
  return workloadKey(e.src_namespace, e.src_workload)
}
export function dstWorkloadKey(e: MapEntry): string {
  return workloadKey(e.dst_namespace, e.dst_workload)
}
export function routeKey(e: MapEntry): string {
  return e.src_zone + '>' + e.dst_zone
}

/** One selectable option in a filter group: a real value that appears in the current entry set,
 * with a count of how many entries it matches (BEFORE other filters are applied — see
 * buildFilterOptions' doc for why counts are computed against the pre-this-group-filtered set,
 * matching how the reference service map's sidebar counts behave). */
export interface FilterOption {
  value: string
  label: string
  count: number
}

export interface MapFilters {
  zones: Set<string>
  namespaces: Set<string>
  workloads: Set<string> // workloadKey() of either src or dst
  srcWorkloads: Set<string>
  dstWorkloads: Set<string>
  routes: Set<string> // routeKey()
  costMin: number | null
  costMax: number | null
  trafficMinGB: number | null
  trafficMaxGB: number | null
}

export function emptyFilters(): MapFilters {
  return {
    zones: new Set(),
    namespaces: new Set(),
    workloads: new Set(),
    srcWorkloads: new Set(),
    dstWorkloads: new Set(),
    routes: new Set(),
    costMin: null,
    costMax: null,
    trafficMinGB: null,
    trafficMaxGB: null,
  }
}

export function isFiltersEmpty(f: MapFilters): boolean {
  return (
    f.zones.size === 0 &&
    f.namespaces.size === 0 &&
    f.workloads.size === 0 &&
    f.srcWorkloads.size === 0 &&
    f.dstWorkloads.size === 0 &&
    f.routes.size === 0 &&
    f.costMin === null &&
    f.costMax === null &&
    f.trafficMinGB === null &&
    f.trafficMaxGB === null
  )
}

/** Number of active filter chips — used both for "Clear all" visibility and to tell a user how
 * many constraints are currently narrowing the map. Multi-select group sizes each count once
 * per selected value (matching how each is rendered as its own removable chip), range filters
 * count once each if set to a POSITIVE value — a non-positive bound is treated as unset (see
 * matchesFilters' doc), so it must not count as active here either, or the chip count/"Reset"
 * visibility would disagree with what's actually being filtered. */
export function countActiveFilters(f: MapFilters): number {
  return (
    f.zones.size +
    f.namespaces.size +
    f.workloads.size +
    f.srcWorkloads.size +
    f.dstWorkloads.size +
    f.routes.size +
    (f.costMin !== null && f.costMin > 0 ? 1 : 0) +
    (f.costMax !== null && f.costMax > 0 ? 1 : 0) +
    (f.trafficMinGB !== null && f.trafficMinGB > 0 ? 1 : 0) +
    (f.trafficMaxGB !== null && f.trafficMaxGB > 0 ? 1 : 0)
  )
}

/** True if entry `e` matches every active constraint in `f`. Cost/traffic ranges are inclusive.
 * A non-positive min/max (<=0) is treated as UNSET, not as an active "at least $0" constraint —
 * cost/traffic can never legitimately be negative, so a stray negative bound (e.g. from an
 * arrow-key/scroll-wheel nudge on an empty number input — a real bug caught via browser testing:
 * one ArrowDown press in an empty field produced "-0.01" and silently became a permanently-true,
 * misleadingly-"active" filter chip) must not silently become a no-op filter that LOOKS active
 * but changes nothing. See FilterPanel.tsx's `min={0}` + clamped onChange for the input-level
 * fix; this is the defense-in-depth check for any other caller of matchesFilters/applyFilters.
 * An entry matches the zones/namespaces/workloads groups if EITHER its source or destination
 * side is in the selected set (an SRE filtering by "us-east-1a" wants every route touching that
 * zone, not just ones where it's specifically the source). */
export function matchesFilters(e: MapEntry, f: MapFilters): boolean {
  if (f.zones.size > 0 && !f.zones.has(e.src_zone) && !f.zones.has(e.dst_zone)) return false
  if (f.namespaces.size > 0 && !f.namespaces.has(e.src_namespace) && !f.namespaces.has(e.dst_namespace)) return false
  if (f.workloads.size > 0 && !f.workloads.has(srcWorkloadKey(e)) && !f.workloads.has(dstWorkloadKey(e))) return false
  if (f.srcWorkloads.size > 0 && !f.srcWorkloads.has(srcWorkloadKey(e))) return false
  if (f.dstWorkloads.size > 0 && !f.dstWorkloads.has(dstWorkloadKey(e))) return false
  if (f.routes.size > 0 && !f.routes.has(routeKey(e))) return false
  if (f.costMin !== null && f.costMin > 0 && e.cost_usd < f.costMin) return false
  if (f.costMax !== null && f.costMax > 0 && e.cost_usd > f.costMax) return false
  if (f.trafficMinGB !== null && f.trafficMinGB > 0 && e.gb < f.trafficMinGB) return false
  if (f.trafficMaxGB !== null && f.trafficMaxGB > 0 && e.gb > f.trafficMaxGB) return false
  return true
}

export function applyFilters(entries: MapEntry[], f: MapFilters): MapEntry[] {
  if (isFiltersEmpty(f)) return entries
  return entries.filter((e) => matchesFilters(e, f))
}

/** One collapsible filter group's real, count-annotated options, sorted by count descending
 * then label — matches the reference's per-group "(N/M selected)" + count-per-row pattern.
 * Counts are computed against `entries` AFTER every OTHER active filter is applied (but not
 * this group's own selection) — so narrowing e.g. Namespace still shows honest zone counts
 * reflecting the current namespace selection, matching the reference screenshot's behavior
 * where counts update live as sibling filters change. */
export function buildFilterOptions(
  entries: MapEntry[],
  extract: (e: MapEntry) => string[],
  labelFor: (value: string) => string,
): FilterOption[] {
  const counts = new Map<string, number>()
  for (const e of entries) {
    for (const v of new Set(extract(e))) {
      counts.set(v, (counts.get(v) || 0) + 1)
    }
  }
  return Array.from(counts.entries())
    .map(([value, count]) => ({ value, label: labelFor(value), count }))
    .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label))
}

export const extractZones = (e: MapEntry) => [e.src_zone, e.dst_zone]
export const extractNamespaces = (e: MapEntry) => [e.src_namespace, e.dst_namespace]
export const extractWorkloads = (e: MapEntry) => [srcWorkloadKey(e), dstWorkloadKey(e)]
export const extractSrcWorkloads = (e: MapEntry) => [srcWorkloadKey(e)]
export const extractDstWorkloads = (e: MapEntry) => [dstWorkloadKey(e)]
export const extractRoutes = (e: MapEntry) => [routeKey(e)]

/** Case-insensitive substring search within a filter group's option list — used by each
 * collapsible group's own search-within box (distinct from the map's own search). */
export function searchOptions(options: FilterOption[], query: string): FilterOption[] {
  if (!query.trim()) return options
  const q = query.trim().toLowerCase()
  return options.filter((o) => o.label.toLowerCase().includes(q) || o.value.toLowerCase().includes(q))
}
