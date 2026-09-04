import type { FlowNode, FlowPair } from './flowGraph'

/** Case-insensitive substring match across a node's id/label/sublabel — used by the map's
 * search box to find a workload/namespace/zone by name. Pure so it's independently testable
 * from the rendering layer. */
export function nodeMatchesQuery(node: FlowNode, query: string): boolean {
  if (!query.trim()) return true
  const q = query.trim().toLowerCase()
  return (
    node.id.toLowerCase().includes(q) ||
    node.label.toLowerCase().includes(q) ||
    (node.sublabel ?? '').toLowerCase().includes(q)
  )
}

/** A route (pair) matches a search query if either endpoint node matches, or the query looks
 * like a "src -> dst" / "src dst" route search. */
export function pairMatchesQuery(pair: FlowPair, nodesById: Map<string, FlowNode>, query: string): boolean {
  if (!query.trim()) return true
  const src = nodesById.get(pair.srcId)
  const dst = nodesById.get(pair.dstId)
  return (src ? nodeMatchesQuery(src, query) : false) || (dst ? nodeMatchesQuery(dst, query) : false)
}

/** Returns the set of node ids reachable from a focused node id via any inbound or outbound
 * edge in `pairs`, plus the focused node itself — the neighborhood highlighted/kept-bright when
 * a user clicks a box to "focus" it. */
export function neighborhoodOf(focusedId: string, pairs: FlowPair[]): Set<string> {
  const ids = new Set<string>([focusedId])
  for (const p of pairs) {
    if (p.srcId === focusedId) ids.add(p.dstId)
    if (p.dstId === focusedId) ids.add(p.srcId)
  }
  return ids
}
