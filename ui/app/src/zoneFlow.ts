import type { CostEntry } from './types'

/** Stable identity for a (namespace, workload) pair — two different namespaces could in
 * principle run a same-named workload, so namespace must be part of the key. */
export function workloadKey(e: CostEntry): string {
  return (e.src_namespace || '') + '|' + (e.src_workload || '(unknown)')
}

export interface ZonePairFlow {
  src: string
  dst: string
  cost: number
  gb: number
  workloads: Map<string, { label: string; cost: number }>
}

export interface ZoneFlow {
  zones: string[]
  pairs: ZonePairFlow[]
  maxPairCost: number
  zoneTotals: Map<string, { out: number; in: number }>
}

/** Aggregates raw cost entries into zone-to-zone traffic flows — the real AWS billing
 * dimension (source AZ -> destination AZ), as opposed to which zone a workload happens to run
 * in. This is the same aggregation validated against live cluster data in the previous
 * vanilla-JS dashboard (see git history for the Node-harness verification); ported to
 * TypeScript unchanged in behavior. Same-zone entries are dropped since they're not cross-AZ
 * cost. */
export function buildZoneFlow(entries: CostEntry[]): ZoneFlow {
  const zones = Array.from(new Set(entries.flatMap((e) => [e.src_zone, e.dst_zone]))).sort()
  const pairTotals = new Map<string, ZonePairFlow>()

  for (const e of entries) {
    if (e.src_zone === e.dst_zone) continue
    const key = e.src_zone + '>' + e.dst_zone
    let cur = pairTotals.get(key)
    if (!cur) {
      cur = { src: e.src_zone, dst: e.dst_zone, cost: 0, gb: 0, workloads: new Map() }
      pairTotals.set(key, cur)
    }
    cur.cost += e.cost_usd
    cur.gb += e.gb
    const wKey = workloadKey(e)
    const w = cur.workloads.get(wKey) || { label: e.src_workload || '(unknown)', cost: 0 }
    w.cost += e.cost_usd
    cur.workloads.set(wKey, w)
  }

  const pairs = Array.from(pairTotals.values()).filter((p) => p.cost > 0)
  const maxPairCost = Math.max(0, ...pairs.map((p) => p.cost))

  const zoneTotals = new Map<string, { out: number; in: number }>()
  zones.forEach((z) => zoneTotals.set(z, { out: 0, in: 0 }))
  for (const p of pairs) {
    zoneTotals.get(p.src)!.out += p.cost
    zoneTotals.get(p.dst)!.in += p.cost
  }

  return { zones, pairs, maxPairCost, zoneTotals }
}
