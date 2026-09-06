// Pure summary-strip calculations for the Cross-AZ Service Map — total spend, traffic, route
// count, affected workload count, highest-cost route, average cost/GB. Kept separate from
// rendering so every number is independently testable against known entry sets, and so the UI
// never recomputes billing math itself (all of this derives directly from MapEntry.cost_usd/gb,
// which the backend already computed via costengine — see internal/costengine/costengine.go).
import type { MapEntry } from './types'
import { srcWorkloadKey, dstWorkloadKey } from './mapFilters'

export interface MapSummary {
  totalCostUSD: number
  totalGB: number
  routeCount: number
  affectedWorkloadCount: number
  highestCostRoute: MapEntry | null
  avgCostPerGB: number | null
}

export function computeMapSummary(entries: MapEntry[]): MapSummary {
  let totalCostUSD = 0
  let totalGB = 0
  const routeKeys = new Set<string>()
  const workloadKeys = new Set<string>()
  let highestCostRoute: MapEntry | null = null

  for (const e of entries) {
    totalCostUSD += e.cost_usd
    totalGB += e.gb
    routeKeys.add(e.src_zone + '>' + e.dst_zone)
    workloadKeys.add(srcWorkloadKey(e))
    workloadKeys.add(dstWorkloadKey(e))
    if (!highestCostRoute || e.cost_usd > highestCostRoute.cost_usd) highestCostRoute = e
  }

  return {
    totalCostUSD,
    totalGB,
    routeCount: routeKeys.size,
    affectedWorkloadCount: workloadKeys.size,
    highestCostRoute,
    avgCostPerGB: totalGB > 0 ? totalCostUSD / totalGB : null,
  }
}
