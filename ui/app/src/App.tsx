import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  applyNodeChanges,
  useReactFlow,
  type Node,
  type NodeChange,
  type EdgeMarker,
  MarkerType,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import './dashboard.css'

import type { CostsResponse, CostEntry, HistoryRange } from './types'
import {
  buildZoneFlow,
  buildWorkloadFlow,
  buildWorkloadPairBreakdown,
  srcWorkloadKey,
  dstWorkloadKey,
  type FlowGraph,
  type WorkloadPairBreakdown,
} from './flowGraph'
import { costColor, fmtAgo, fmtGB, fmtUSD } from './format'
import { nodeMatchesQuery, neighborhoodOf } from './mapSearch'
import { useHistory } from './useHistory'
import { FlowBoxNode, type FlowBoxNodeData } from './components/FlowBoxNode'
import { FlowGraphEdge, type FlowGraphEdgeData } from './components/FlowGraphEdge'
import { EdgeDrillDownPanel, type DrillDownSelection } from './components/EdgeDrillDownPanel'
import { MapLegend } from './components/MapLegend'
import { KpiPanel } from './components/KpiPanel'
import { CostHistoryChart } from './components/CostHistoryChart'

const POLL_MS = 10_000

const nodeTypes = { flowBox: FlowBoxNode }
const edgeTypes = { flowGraph: FlowGraphEdge }

type ViewMode = 'zone' | 'workload'

/** ReactFlow's `fitView` prop only runs once, on the component's initial mount — switching
 * viewMode later re-renders with all-new nodes/edges but does NOT re-run fitView, so the
 * viewport stays framed for whichever view was active first. Caught this via a real headless
 * browser screenshot against live 15-workload data: switching to the workload view left most
 * boxes rendered off-canvas, framed for the much smaller zone view instead. Fixed by calling
 * fitView() imperatively (via useReactFlow, which requires living inside a ReactFlowProvider)
 * whenever viewMode or the node count changes. */
function RefitOnViewChange({ viewMode, nodeCount }: { viewMode: ViewMode; nodeCount: number }) {
  const { fitView } = useReactFlow()
  useEffect(() => {
    // Deferred one tick so this runs after ReactFlow has measured the newly-swapped node set —
    // calling fitView() in the same tick as the nodes prop changing can compute bounds from the
    // previous (stale) node positions.
    const id = requestAnimationFrame(() => fitView({ padding: 0.15, duration: 200 }))
    return () => cancelAnimationFrame(id)
  }, [viewMode, nodeCount, fitView])
  return null
}

/** Nodes are laid out on a static, deterministic circle (or side-by-side row for <=2 nodes) —
 * no force simulation. This is a closed-form function of (index, count), so it can never
 * converge badly, jump between renders, or produce NaN positions; it's also directly testable
 * with pure geometry, unlike a physics sim. The user can still drag nodes anywhere afterward —
 * ReactFlow persists that position in its own node state — this is just the initial layout.
 *
 * The circle's radius and the virtual canvas it's drawn on both grow with node count (rather
 * than staying fixed at whatever fit 3 zones) — the workload view can easily have a dozen-plus
 * nodes, and a fixed-radius circle at that count visibly overlapped boxes in testing.
 * ReactFlow's fitView then zooms/pans the actual viewport to fit whatever this returns, so a
 * bigger virtual canvas here doesn't shrink anything on screen — it just gives crowded layouts
 * room to spread out before fitView frames them. */
function initialNodePosition(index: number, total: number): { x: number; y: number } {
  const boxWidth = 190 // approx rendered width of a flow-box-node, incl. padding — used to
  // pick a radius that keeps adjacent boxes from visually overlapping as count grows.
  if (total <= 2) {
    const width = 900
    const x = total === 1 ? width / 2 : index === 0 ? width * 0.25 : width * 0.75
    return { x, y: 260 }
  }
  // Circumference needed for `total` boxes side-by-side, plus headroom, converted back to a
  // radius; floored at a sane minimum so small counts (3-4) don't get an unnecessarily tiny
  // circle.
  const r = Math.max(230, (total * boxWidth * 1.15) / (2 * Math.PI))
  const cx = r + 160
  const cy = r + 120
  const angle = -Math.PI / 2 + index * ((2 * Math.PI) / total)
  return { x: cx + r * Math.cos(angle), y: cy + r * Math.sin(angle) }
}

async function fetchCosts(): Promise<CostsResponse> {
  const res = await fetch('/api/v1/costs', { cache: 'no-store' })
  if (!res.ok) throw new Error(`/api/v1/costs: HTTP ${res.status}`)
  return res.json()
}

export default function App() {
  const [costs, setCosts] = useState<CostsResponse | null>(null)
  const [fetchError, setFetchError] = useState<string | null>(null)
  const [filterKey, setFilterKey] = useState<string>('')
  const [viewMode, setViewMode] = useState<ViewMode>('zone')
  const [offendersCollapsed, setOffendersCollapsed] = useState(false)
  // Set by clicking a row in the edge drill-down panel — narrows the view to exactly this one
  // (source workload, destination workload) pair rather than "all traffic from this source
  // workload" (the coarser `filterKey` above). This is what makes "click zone A->B, click one
  // workload pair, see just that pair in Workload -> Workload" actually land on a single edge
  // instead of that workload's traffic to every zone it happens to talk to.
  const [pairFilter, setPairFilter] = useState<{ srcKey: string; dstKey: string; srcLabel: string; dstLabel: string } | null>(null)
  // The currently-open drill-down panel, or null when closed. Selecting an edge in the zone
  // view populates this with every real workload pair behind that route (see
  // buildWorkloadPairBreakdown) rather than the hover tooltip's capped top-5.
  const [drillDown, setDrillDown] = useState<DrillDownSelection | null>(null)
  // Fullscreen map mode: a substantially larger canvas is the #1 fix for "the map is too small
  // to inspect" — rather than inventing a second diagram, the SAME ReactFlow instance is
  // reparented into an overlay so filters/selection/positions all carry over untouched.
  const [mapFullscreen, setMapFullscreen] = useState(false)
  // Free-text search across node id/label/sublabel (workload, namespace, or zone name) — matched
  // nodes/edges stay full-opacity, everything else dims, rather than actually removing nodes
  // from the graph (removing them would also silently hide their un-searched-for edges/context).
  const [mapSearch, setMapSearch] = useState('')
  // Clicking a node "focuses" it: only that node's own inbound/outbound routes stay bright,
  // everything else dims — the requested "click a workload, see its zone A/B breakdown"
  // interaction, generalized to any node in either view. Click the same node again to unfocus.
  const [focusedNodeId, setFocusedNodeId] = useState<string | null>(null)
  const [historyRange, setHistoryRange] = useState<HistoryRange>('24h')
  const history = useHistory(historyRange)
  // Node positions persist across data refreshes AND across switching view mode back and forth
  // (keyed by "viewMode:nodeId" so a zone id and a workload id can never collide) so a user's
  // drag isn't undone by the next 10s poll or by toggling views.
  const nodePositionsRef = useRef<Map<string, { x: number; y: number }>>(new Map())

  useEffect(() => {
    let cancelled = false
    async function poll() {
      try {
        const data = await fetchCosts()
        if (!cancelled) {
          setCosts(data)
          setFetchError(null)
        }
      } catch (err) {
        if (!cancelled) setFetchError(err instanceof Error ? err.message : String(err))
      }
    }
    poll()
    const id = setInterval(poll, POLL_MS)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [])

  // Escape key exits fullscreen map mode — a required control per the fullscreen spec, not
  // just the visible close button, since a keyboard-first / power user expects it to just work.
  useEffect(() => {
    if (!mapFullscreen) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setMapFullscreen(false)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [mapFullscreen])

  const entries: CostEntry[] = costs?.entries ?? []
  const positiveEntries = useMemo(() => entries.filter((e) => e.cost_usd > 0), [entries])

  const workloadOptions = useMemo(() => {
    const totals = new Map<string, { label: string; cost: number }>()
    for (const e of positiveEntries) {
      const key = srcWorkloadKey(e)
      const cur = totals.get(key) || { label: e.src_workload || '(unknown)', cost: 0 }
      cur.cost += e.cost_usd
      totals.set(key, cur)
    }
    return Array.from(totals.entries()).sort((a, b) => b[1].cost - a[1].cost)
  }, [positiveEntries])

  const scopedEntries = useMemo(
    () =>
      positiveEntries.filter((e) => {
        if (pairFilter) return srcWorkloadKey(e) === pairFilter.srcKey && dstWorkloadKey(e) === pairFilter.dstKey
        return !filterKey || srcWorkloadKey(e) === filterKey
      }),
    [positiveEntries, filterKey, pairFilter],
  )

  const flowGraph: FlowGraph = useMemo(
    () => (viewMode === 'zone' ? buildZoneFlow(scopedEntries) : buildWorkloadFlow(scopedEntries)),
    [scopedEntries, viewMode],
  )

  // Node positions must be real React state (not a plain memo) and flow through ReactFlow's
  // onNodesChange, or ReactFlow treats `nodes` as fully controlled and silently ignores drag
  // gestures entirely — this was a real bug caught by browser-driven verification (Playwright
  // drag simulation moved the pointer correctly but the node never moved) before shipping.
  const [nodes, setNodes] = useState<Node<FlowBoxNodeData, 'flowBox'>[]>([])

  useEffect(() => {
    setNodes((prev) => {
      const prevById = new Map(prev.map((n) => [n.id, n]))
      return flowGraph.nodes.map((n, i) => {
        const posKey = viewMode + ':' + n.id
        const existingNode = prevById.get(n.id)
        const existingPos = nodePositionsRef.current.get(posKey)
        const position = existingNode?.position ?? existingPos ?? initialNodePosition(i, flowGraph.nodes.length)
        nodePositionsRef.current.set(posKey, position)
        return {
          id: n.id,
          type: 'flowBox' as const,
          position,
          data: { label: n.label, sublabel: n.sublabel },
        }
      })
    })
  }, [flowGraph, viewMode])

  const onNodesChange = useCallback((changes: NodeChange<Node<FlowBoxNodeData, 'flowBox'>>[]) => {
    setNodes((nds) => applyNodeChanges(changes, nds))
  }, [])

  // Reset search/focus when the underlying entity set changes shape (view mode or workload
  // filter) so a stale focused-node-id from the zone view doesn't silently persist (and match
  // nothing) after switching to the workload view.
  useEffect(() => {
    setFocusedNodeId(null)
  }, [viewMode, filterKey, pairFilter])

  const nodesById = useMemo(() => new Map(flowGraph.nodes.map((n) => [n.id, n])), [flowGraph.nodes])

  // The neighborhood (self + directly connected nodes) of the focused node, or null when no
  // node is focused — null is treated as "everything is in scope" everywhere this is used.
  const focusedNeighborhood = useMemo(
    () => (focusedNodeId ? neighborhoodOf(focusedNodeId, flowGraph.pairs) : null),
    [focusedNodeId, flowGraph.pairs],
  )

  // A node/edge is dimmed if a search query is active and it doesn't match, OR a node is
  // focused and this node/edge isn't in its neighborhood. Both conditions can combine (e.g.
  // search AND focus active at once) — dim if EITHER excludes it.
  const isNodeDimmed = useCallback(
    (id: string) => {
      const node = nodesById.get(id)
      if (mapSearch.trim() && (!node || !nodeMatchesQuery(node, mapSearch))) return true
      if (focusedNeighborhood && !focusedNeighborhood.has(id)) return true
      return false
    },
    [nodesById, mapSearch, focusedNeighborhood],
  )

  const edges = useMemo(() => {
    const breakdownHeading = viewMode === 'zone' ? 'Top workloads' : 'Top zone routes'
    return flowGraph.pairs.map((pair) => {
      const color = costColor(pair.cost, flowGraph.maxPairCost)
      const widthPx = Math.max(1.5, Math.min(7, 1.5 + 5.5 * Math.sqrt(pair.cost / (flowGraph.maxPairCost || 1))))
      const marker: EdgeMarker = { type: MarkerType.ArrowClosed, color, width: 22, height: 22 }
      const srcNode = flowGraph.nodes.find((n) => n.id === pair.srcId)
      const dstNode = flowGraph.nodes.find((n) => n.id === pair.dstId)
      // Drilling down only makes sense in the zone view — a workload-view edge is already the
      // single most granular thing (one workload pair), there's nothing further to break it
      // into.
      const onSelect =
        viewMode === 'zone'
          ? () =>
              setDrillDown({
                srcLabel: srcNode?.label ?? pair.srcId,
                dstLabel: dstNode?.label ?? pair.dstId,
                totalCost: pair.cost,
                totalGb: pair.gb,
                pairs: buildWorkloadPairBreakdown(pair.entries),
              })
          : undefined
      const dimmed = isNodeDimmed(pair.srcId) || isNodeDimmed(pair.dstId)
      return {
        id: `${pair.srcId}>${pair.dstId}`,
        source: pair.srcId,
        target: pair.dstId,
        type: 'flowGraph' as const,
        markerEnd: marker,
        data: { pair, color, widthPx, breakdownHeading, onSelect, dimmed } satisfies FlowGraphEdgeData,
      }
    })
  }, [flowGraph, viewMode, isNodeDimmed])

  // Injects search/focus emphasis + click-to-focus into the positioned node state, without
  // making emphasis part of that state itself — so a search keystroke doesn't reset drag
  // positions (setNodes above intentionally excludes mapSearch/focusedNodeId from its deps).
  const displayNodes = useMemo(
    () =>
      nodes.map((n) => ({
        ...n,
        data: {
          ...n.data,
          emphasis: focusedNodeId === n.id ? ('focused' as const) : isNodeDimmed(n.id) ? ('dimmed' as const) : undefined,
          onClick: () => setFocusedNodeId((cur) => (cur === n.id ? null : n.id)),
        },
      })),
    [nodes, focusedNodeId, isNodeDimmed],
  )

  const totalCost = flowGraph.pairs.reduce((s, p) => s + p.cost, 0)
  const totalGB = flowGraph.pairs.reduce((s, p) => s + p.gb, 0)

  const topOffenders = useMemo(
    () => [...scopedEntries].sort((a, b) => b.cost_usd - a.cost_usd).slice(0, 15),
    [scopedEntries],
  )
  const maxOffenderCost = topOffenders[0]?.cost_usd ?? 0

  return (
    <div className="app">
      <header>
        <h1>
          Zone<span className="tax">Tax</span>
        </h1>
        <div className="meta">
          <span>
            {costs?.cloud || '—'} · {costs?.region || '—'}
          </span>
          {costs?.stale_error ? (
            <span className="stale">
              <span className="dot dot-bad" />
              stale — {costs.stale_error}
            </span>
          ) : fetchError ? (
            <span className="stale">
              <span className="dot dot-bad" />
              fetch failed: {fetchError}
            </span>
          ) : (
            <span>
              <span className="dot" />
              updated {fmtAgo(costs?.updated_at)}
            </span>
          )}
        </div>
      </header>

      <main className={offendersCollapsed ? 'offenders-collapsed' : ''}>
        <KpiPanel costs={costs} fetchError={fetchError} />

        <CostHistoryChart
          range={historyRange}
          onRangeChange={setHistoryRange}
          history={history.data}
          loading={history.loading}
          error={history.error}
        />


        <div className={`card flow-card${mapFullscreen ? ' flow-fullscreen-anchor' : ''}`}>
          <div className="card-head">
            <h2>Traffic map</h2>
            <div className="head-controls">
              <div className="view-toggle" role="tablist" aria-label="Flow map view">
                <button
                  type="button"
                  className={viewMode === 'zone' ? 'active' : ''}
                  onClick={() => {
                    setViewMode('zone')
                    setPairFilter(null)
                  }}
                  role="tab"
                  aria-selected={viewMode === 'zone'}
                >
                  Zone → Zone
                </button>
                <button
                  type="button"
                  className={viewMode === 'workload' ? 'active' : ''}
                  onClick={() => {
                    setViewMode('workload')
                    setPairFilter(null)
                  }}
                  role="tab"
                  aria-selected={viewMode === 'workload'}
                >
                  Workload → Workload
                </button>
              </div>
              <input
                type="text"
                className="map-search"
                placeholder="Search workload, namespace, or zone…"
                value={mapSearch}
                onChange={(e) => setMapSearch(e.target.value)}
                aria-label="Search the traffic map"
              />
              {pairFilter ? (
                <div className="pair-filter-chip">
                  <span>
                    {pairFilter.srcLabel} → {pairFilter.dstLabel}
                  </span>
                  <button type="button" onClick={() => setPairFilter(null)} aria-label="Clear pair filter">
                    ×
                  </button>
                </div>
              ) : (
                <select
                  className="workload-select"
                  value={filterKey}
                  onChange={(e) => {
                    setFilterKey(e.target.value)
                    setPairFilter(null)
                  }}
                  title={filterKey ? workloadOptions.find(([k]) => k === filterKey)?.[1].label : 'All workloads'}
                >
                  <option value="">All workloads ({workloadOptions.length})</option>
                  {workloadOptions.map(([key, v]) => (
                    <option key={key} value={key}>
                      {v.label} — {fmtUSD(v.cost)}
                    </option>
                  ))}
                </select>
              )}
              <button
                type="button"
                className="fullscreen-btn"
                onClick={() => setMapFullscreen((v) => !v)}
                title={mapFullscreen ? 'Exit fullscreen (Esc)' : 'Expand to fullscreen'}
                aria-pressed={mapFullscreen}
              >
                {mapFullscreen ? '⤡ Exit fullscreen' : '⤢ Fullscreen'}
              </button>
            </div>
          </div>
          <div className="flow-summary">
            {flowGraph.nodes.length} {viewMode === 'zone' ? 'zone' : 'workload'}
            {flowGraph.nodes.length === 1 ? '' : 's'} · {flowGraph.pairs.length} route
            {flowGraph.pairs.length === 1 ? '' : 's'} · {fmtUSD(totalCost)} · {fmtGB(totalGB)}
            {' · '}every route with any tracked cost is shown, none are hidden below a threshold
          </div>
          <MapLegend viewMode={viewMode} />
          {mapFullscreen && <div className="fullscreen-backdrop" onClick={() => setMapFullscreen(false)} />}
          <div className={`flow-canvas${mapFullscreen ? ' flow-canvas-fullscreen' : ''}`}>
            {focusedNodeId && (
              <div className="focus-chip">
                <span>Focused: {nodesById.get(focusedNodeId)?.label ?? focusedNodeId}</span>
                <button type="button" onClick={() => setFocusedNodeId(null)} aria-label="Clear focus">
                  ×
                </button>
              </div>
            )}
            {flowGraph.pairs.length === 0 ? (
              <div className="empty">
                {filterKey
                  ? 'This workload has no cross-AZ traffic in the current window.'
                  : 'No cross-AZ traffic observed yet.'}
              </div>
            ) : (
              <ReactFlowProvider>
                <ReactFlow
                  nodes={displayNodes}
                  edges={edges}
                  nodeTypes={nodeTypes}
                  edgeTypes={edgeTypes}
                  onNodesChange={onNodesChange}
                  onNodeDragStop={(_, node) => {
                    nodePositionsRef.current.set(viewMode + ':' + node.id, node.position)
                  }}
                  onPaneClick={() => setFocusedNodeId(null)}
                  fitView
                  minZoom={0.15}
                  maxZoom={3}
                  proOptions={{ hideAttribution: true }}
                >
                  <Background gap={24} color="rgba(255,255,255,0.04)" />
                  <Controls showInteractive={false} />
                  <RefitOnViewChange viewMode={viewMode} nodeCount={nodes.length} />
                </ReactFlow>
              </ReactFlowProvider>
            )}
            {drillDown && (
              <EdgeDrillDownPanel
                selection={drillDown}
                onClose={() => setDrillDown(null)}
                onSelectPair={(p: WorkloadPairBreakdown) => {
                  // Pivots into the existing Workload -> Workload view, scoped to exactly this
                  // one pair via pairFilter (not the coarser single-workload filterKey) — this
                  // is the "click a workload pair, see just that pair" flow, reusing the view
                  // that's already built rather than inventing a third diagram just for this.
                  setPairFilter({ srcKey: p.srcKey, dstKey: p.dstKey, srcLabel: p.srcLabel, dstLabel: p.dstLabel })
                  setFilterKey('')
                  setViewMode('workload')
                  setDrillDown(null)
                }}
              />
            )}
          </div>
        </div>

        <div className={`card offenders-card${offendersCollapsed ? ' collapsed' : ''}`}>
          <div className="card-head">
            <h2>Top offenders</h2>
            <button
              type="button"
              className="collapse-btn"
              onClick={() => setOffendersCollapsed((c) => !c)}
              aria-expanded={!offendersCollapsed}
              title={offendersCollapsed ? 'Show top offenders' : 'Hide top offenders'}
            >
              {offendersCollapsed ? 'Show' : 'Hide'}
            </button>
          </div>
          {!offendersCollapsed && (
            <>
              <table>
                <thead>
                  <tr>
                    <th>Workload</th>
                    <th>Route</th>
                    <th style={{ textAlign: 'right' }}>GB</th>
                    <th style={{ textAlign: 'right' }}>Cost</th>
                  </tr>
                </thead>
                <tbody>
                  {topOffenders.map((e, i) => {
                    const color = costColor(e.cost_usd, maxOffenderCost)
                    return (
                      <tr key={i}>
                        <td className="workload">
                          {e.src_workload || '(unknown)'}
                          <div className="ns">{e.src_namespace}</div>
                        </td>
                        <td className="zones">
                          {e.src_zone} → {e.dst_zone}
                        </td>
                        <td className="gb">{fmtGB(e.gb)}</td>
                        <td className="cost" style={{ color }}>
                          <span className="cost-dot" style={{ background: color }} />
                          {fmtUSD(e.cost_usd)}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
              {topOffenders.length === 0 && <div className="empty">No data yet.</div>}
            </>
          )}
        </div>
      </main>

      <footer>
        ZoneTax · polling every 10s ·{' '}
        <a href="https://github.com/gargkrishna730/zonetax" target="_blank" rel="noreferrer">
          github.com/gargkrishna730/zonetax
        </a>
      </footer>
    </div>
  )
}
