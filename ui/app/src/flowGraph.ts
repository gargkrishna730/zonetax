import type { CostEntry } from './types'

/** Stable identity for a (namespace, workload) pair — two different namespaces could in
 * principle run a same-named workload, so namespace must be part of the key. */
export function workloadKey(namespace: string, workload: string): string {
  return (namespace || '') + '|' + (workload || '(unknown)')
}
export function srcWorkloadKey(e: CostEntry): string {
  return workloadKey(e.src_namespace, e.src_workload)
}
export function dstWorkloadKey(e: CostEntry): string {
  return workloadKey(e.dst_namespace, e.dst_workload)
}

/** One box in the flow diagram — either an availability zone or a workload, depending on which
 * view is active. `sublabel` is the small secondary line under the main label (e.g. a
 * namespace, or an out/in cost summary). */
export interface FlowNode {
  id: string
  label: string
  sublabel?: string
}

/** One directed edge between two FlowNodes, aggregated from every raw CostEntry that shares the
 * same (source, destination) pair. `breakdown` holds a human-readable detail list shown on
 * hover — contributing workloads for the zone view, contributing zone-routes for the workload
 * view — so the two views stay complementary instead of duplicating the same information. */
export interface FlowPair {
  srcId: string
  dstId: string
  cost: number
  gb: number
  breakdown: Map<string, { label: string; cost: number }>
}

export interface FlowGraph {
  nodes: FlowNode[]
  pairs: FlowPair[]
  maxPairCost: number
}

/** Aggregates raw cost entries into a directed graph of `cost` between whatever `nodeId`
 * resolves each entry's source/destination to. This is the one piece of aggregation logic
 * shared by both the zone-to-zone view and the workload-to-workload view — previously each was
 * going to need its own near-identical copy of this exact grouping/summing logic, which is
 * exactly the kind of duplication that drifts out of sync over time (e.g. one view accidentally
 * not excluding same-node self-loops while the other does). Same-node entries (srcId == dstId)
 * are dropped since a workload/zone can't meaningfully be "cross" itself. */
function buildFlowGraph(
  entries: CostEntry[],
  nodeId: (e: CostEntry, side: 'src' | 'dst') => string,
  nodeLabel: (e: CostEntry, side: 'src' | 'dst') => string,
  nodeSublabel: (e: CostEntry, side: 'src' | 'dst') => string | undefined,
  breakdownKey: (e: CostEntry) => string,
  breakdownLabel: (e: CostEntry) => string,
): FlowGraph {
  const nodeLabels = new Map<string, { label: string; sublabel?: string }>()
  const pairTotals = new Map<string, FlowPair>()

  for (const e of entries) {
    const srcId = nodeId(e, 'src')
    const dstId = nodeId(e, 'dst')
    if (!nodeLabels.has(srcId)) nodeLabels.set(srcId, { label: nodeLabel(e, 'src'), sublabel: nodeSublabel(e, 'src') })
    if (!nodeLabels.has(dstId)) nodeLabels.set(dstId, { label: nodeLabel(e, 'dst'), sublabel: nodeSublabel(e, 'dst') })
    if (srcId === dstId) continue

    const key = srcId + '>' + dstId
    let cur = pairTotals.get(key)
    if (!cur) {
      cur = { srcId, dstId, cost: 0, gb: 0, breakdown: new Map() }
      pairTotals.set(key, cur)
    }
    cur.cost += e.cost_usd
    cur.gb += e.gb
    const bKey = breakdownKey(e)
    const b = cur.breakdown.get(bKey) || { label: breakdownLabel(e), cost: 0 }
    b.cost += e.cost_usd
    cur.breakdown.set(bKey, b)
  }

  const pairs = Array.from(pairTotals.values()).filter((p) => p.cost > 0)
  const maxPairCost = Math.max(0, ...pairs.map((p) => p.cost))

  // Node totals (out/in cost) become each node's sublabel, computed after pairs so it reflects
  // only cost-positive pairs — matches what's actually drawn, not raw entry counts.
  const outIn = new Map<string, { out: number; in: number }>()
  for (const [id] of nodeLabels) outIn.set(id, { out: 0, in: 0 })
  for (const p of pairs) {
    outIn.get(p.srcId)!.out += p.cost
    outIn.get(p.dstId)!.in += p.cost
  }

  const nodes: FlowNode[] = Array.from(nodeLabels.entries())
    .map(([id, { label, sublabel }]) => ({ id, label, sublabel: sublabel ?? formatOutIn(outIn.get(id)) }))
    .sort((a, b) => a.label.localeCompare(b.label))

  return { nodes, pairs, maxPairCost }
}

function formatOutIn(t?: { out: number; in: number }): string | undefined {
  if (!t) return undefined
  return `out ${fmtUSDShortLocal(t.out)} · in ${fmtUSDShortLocal(t.in)}`
}
// Tiny local copy to avoid a circular import with format.ts's fuller fmtUSDShort (which is
// identical for this use — kept in sync manually since it's a two-line pure function).
function fmtUSDShortLocal(v: number): string {
  if (v < 0.01 && v > 0) return '<$0.01'
  if (v >= 1000) return '$' + (v / 1000).toFixed(1) + 'k'
  return '$' + v.toFixed(v < 10 ? 2 : 0)
}

/** The zone-to-zone view: nodes are availability zones, edges are the real AWS billing
 * dimension (source AZ -> destination AZ cost), hover breakdown is by contributing workload.
 * Same-zone entries are dropped since they're not cross-AZ cost. */
export function buildZoneFlow(entries: CostEntry[]): FlowGraph {
  return buildFlowGraph(
    entries,
    (e, side) => (side === 'src' ? e.src_zone : e.dst_zone),
    (e, side) => (side === 'src' ? e.src_zone : e.dst_zone),
    () => undefined, // sublabel comes from the computed out/in cost instead
    (e) => srcWorkloadKey(e),
    (e) => e.src_workload || '(unknown)',
  )
}

/** The workload-to-workload view: nodes are workloads (namespace/name), edges are which
 * workload's traffic actually lands on which other workload — the literal "pod A talks to pod
 * B, this much cost" picture that the zone view structurally can't show (it only knows which
 * zone a workload lives in, not who it's talking to). Hover breakdown is by contributing
 * zone-route, since a single workload pair's traffic can span more than one AZ route (e.g. a
 * multi-replica service). */
export function buildWorkloadFlow(entries: CostEntry[]): FlowGraph {
  return buildFlowGraph(
    entries,
    (e, side) => (side === 'src' ? srcWorkloadKey(e) : dstWorkloadKey(e)),
    (e, side) => (side === 'src' ? e.src_workload || '(unknown)' : e.dst_workload || '(unknown)'),
    (e, side) => (side === 'src' ? e.src_namespace : e.dst_namespace),
    (e) => e.src_zone + '>' + e.dst_zone,
    (e) => `${e.src_zone} → ${e.dst_zone}`,
  )
}
