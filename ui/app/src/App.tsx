import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ReactFlow,
  Background,
  Controls,
  applyNodeChanges,
  type Node,
  type NodeChange,
  type EdgeMarker,
  MarkerType,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import './dashboard.css'

import type { CostsResponse, CostEntry } from './types'
import { buildZoneFlow, buildWorkloadFlow, srcWorkloadKey, type FlowGraph } from './flowGraph'
import { costColor, fmtAgo, fmtGB, fmtUSD } from './format'
import { FlowBoxNode, type FlowBoxNodeData } from './components/FlowBoxNode'
import { FlowGraphEdge, type FlowGraphEdgeData } from './components/FlowGraphEdge'

const POLL_MS = 10_000

const nodeTypes = { flowBox: FlowBoxNode }
const edgeTypes = { flowGraph: FlowGraphEdge }

type ViewMode = 'zone' | 'workload'

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
    () => positiveEntries.filter((e) => !filterKey || srcWorkloadKey(e) === filterKey),
    [positiveEntries, filterKey],
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

  const edges = useMemo(() => {
    const breakdownHeading = viewMode === 'zone' ? 'Top workloads' : 'Top zone routes'
    return flowGraph.pairs.map((pair) => {
      const color = costColor(pair.cost, flowGraph.maxPairCost)
      const widthPx = Math.max(1.5, Math.min(7, 1.5 + 5.5 * Math.sqrt(pair.cost / (flowGraph.maxPairCost || 1))))
      const marker: EdgeMarker = { type: MarkerType.ArrowClosed, color, width: 22, height: 22 }
      return {
        id: `${pair.srcId}>${pair.dstId}`,
        source: pair.srcId,
        target: pair.dstId,
        type: 'flowGraph' as const,
        markerEnd: marker,
        data: { pair, color, widthPx, breakdownHeading } satisfies FlowGraphEdgeData,
      }
    })
  }, [flowGraph, viewMode])

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
        <div className="card kpis">
          <div className="kpi">
            <div className="label">Cross-AZ spend (tracked)</div>
            <div className="value cost">{fmtUSD(costs?.totals.cross_az_cost_usd ?? 0)}</div>
            <div className="sub">since agents last restarted</div>
          </div>
          <div className="kpi">
            <div className="label">Cross-AZ traffic</div>
            <div className="value">{fmtGB(costs?.totals.cross_az_gb ?? 0)}</div>
            <div className="sub">
              {costs
                ? (
                    (100 * costs.totals.cross_az_gb) /
                    Math.max(1e-9, costs.totals.cross_az_gb + costs.totals.same_az_gb)
                  ).toFixed(1)
                : '0'}
              % of all traffic is cross-AZ
            </div>
          </div>
          <div className="kpi">
            <div className="label">Same-AZ traffic</div>
            <div className="value">{fmtGB(costs?.totals.same_az_gb ?? 0)}</div>
            <div className="sub">free, for comparison</div>
          </div>
          <div className="kpi">
            <div className="label">Price / GB</div>
            <div className="value">{fmtUSD(costs?.totals.price_per_gb_usd ?? 0)}/GB</div>
            <div className="sub">
              {costs?.totals.price_per_gb_direction_usd
                ? `$${costs.totals.price_per_gb_direction_usd.toFixed(3)}/GB each direction, billed twice`
                : '—'}
            </div>
          </div>
        </div>

        <div className="card flow-card">
          <div className="card-head">
            <h2>Traffic map</h2>
            <div className="head-controls">
              <div className="view-toggle" role="tablist" aria-label="Flow map view">
                <button
                  type="button"
                  className={viewMode === 'zone' ? 'active' : ''}
                  onClick={() => setViewMode('zone')}
                  role="tab"
                  aria-selected={viewMode === 'zone'}
                >
                  Zone → Zone
                </button>
                <button
                  type="button"
                  className={viewMode === 'workload' ? 'active' : ''}
                  onClick={() => setViewMode('workload')}
                  role="tab"
                  aria-selected={viewMode === 'workload'}
                >
                  Workload → Workload
                </button>
              </div>
              <select value={filterKey} onChange={(e) => setFilterKey(e.target.value)}>
                <option value="">All workloads</option>
                {workloadOptions.map(([key, v]) => (
                  <option key={key} value={key}>
                    {v.label} — {fmtUSD(v.cost)}
                  </option>
                ))}
              </select>
            </div>
          </div>
          <div className="flow-summary">
            {flowGraph.nodes.length} {viewMode === 'zone' ? 'zone' : 'workload'}
            {flowGraph.nodes.length === 1 ? '' : 's'} · {flowGraph.pairs.length} route
            {flowGraph.pairs.length === 1 ? '' : 's'} · {fmtUSD(totalCost)} · {fmtGB(totalGB)}
          </div>
          <div className="legend">
            <span>
              {viewMode === 'zone'
                ? 'Arrow = source zone → destination zone'
                : 'Arrow = source workload → destination workload'}
            </span>
            <span>Drag boxes · scroll or pinch to zoom</span>
          </div>
          <div className="flow-canvas">
            {flowGraph.pairs.length === 0 ? (
              <div className="empty">
                {filterKey
                  ? 'This workload has no cross-AZ traffic in the current window.'
                  : 'No cross-AZ traffic observed yet.'}
              </div>
            ) : (
              <ReactFlow
                nodes={nodes}
                edges={edges}
                nodeTypes={nodeTypes}
                edgeTypes={edgeTypes}
                onNodesChange={onNodesChange}
                onNodeDragStop={(_, node) => {
                  nodePositionsRef.current.set(viewMode + ':' + node.id, node.position)
                }}
                fitView
                minZoom={0.2}
                maxZoom={3}
                proOptions={{ hideAttribution: true }}
              >
                <Background gap={24} color="rgba(255,255,255,0.04)" />
                <Controls showInteractive={false} />
              </ReactFlow>
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
