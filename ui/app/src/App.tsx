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
import { buildZoneFlow, workloadKey } from './zoneFlow'
import { costColor, fmtAgo, fmtGB, fmtUSD } from './format'
import { ZoneNode, type ZoneNodeData } from './components/ZoneNode'
import { ZoneFlowEdge, type ZoneEdgeData } from './components/ZoneFlowEdge'

const POLL_MS = 10_000

const nodeTypes = { zone: ZoneNode }
const edgeTypes = { zoneFlow: ZoneFlowEdge }

/** Zones are laid out on a static, deterministic circle (or side-by-side row for <=2 zones) —
 * no force simulation. This is a closed-form function of (index, count), so it can never
 * converge badly, jump between renders, or produce NaN positions; it's also directly testable
 * with pure geometry, unlike a physics sim. The user can still drag zones anywhere afterward —
 * ReactFlow persists that position in its own node state — this is just the initial layout. */
function initialZonePosition(_zone: string, index: number, total: number): { x: number; y: number } {
  const width = 900
  const height = 520
  if (total <= 2) {
    const x = total === 1 ? width / 2 : index === 0 ? width * 0.25 : width * 0.75
    return { x, y: height / 2 }
  }
  const cx = width / 2
  const cy = height / 2
  const r = Math.min(width, height) / 2 - 90
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
  // Zone positions persist across data refreshes (keyed by zone name) so a user's drag isn't
  // undone by the next 10s poll — the same "sticky node position" requirement from every prior
  // iteration of this diagram, now backed by ReactFlow's own node position state instead of a
  // hand-rolled position cache.
  const zonePositionsRef = useRef<Map<string, { x: number; y: number }>>(new Map())

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
      const key = workloadKey(e)
      const cur = totals.get(key) || { label: e.src_workload || '(unknown)', cost: 0 }
      cur.cost += e.cost_usd
      totals.set(key, cur)
    }
    return Array.from(totals.entries()).sort((a, b) => b[1].cost - a[1].cost)
  }, [positiveEntries])

  const scopedEntries = useMemo(
    () => positiveEntries.filter((e) => !filterKey || workloadKey(e) === filterKey),
    [positiveEntries, filterKey],
  )

  const zoneFlow = useMemo(() => buildZoneFlow(scopedEntries), [scopedEntries])

  // Node positions must be real React state (not a plain memo) and flow through ReactFlow's
  // onNodesChange, or ReactFlow treats `nodes` as fully controlled and silently ignores drag
  // gestures entirely — this was a real bug caught by browser-driven verification (Playwright
  // drag simulation moved the pointer correctly but the node never moved) before shipping.
  // zonePositionsRef still exists to survive a full zone-list rebuild (e.g. switching the
  // workload filter recomputes zoneFlow.zones from scratch) without resetting a dragged
  // position back to the default layout.
  const [nodes, setNodes] = useState<Node<ZoneNodeData, 'zone'>[]>([])

  useEffect(() => {
    setNodes((prev) => {
      const prevById = new Map(prev.map((n) => [n.id, n]))
      return zoneFlow.zones.map((zone, i) => {
        const existingNode = prevById.get(zone)
        const existingPos = zonePositionsRef.current.get(zone)
        const position = existingNode?.position ?? existingPos ?? initialZonePosition(zone, i, zoneFlow.zones.length)
        zonePositionsRef.current.set(zone, position)
        const totals = zoneFlow.zoneTotals.get(zone) ?? { out: 0, in: 0 }
        return {
          id: zone,
          type: 'zone' as const,
          position,
          data: { zone, out: totals.out, in: totals.in, touched: true },
        }
      })
    })
  }, [zoneFlow])

  const onNodesChange = useCallback((changes: NodeChange<Node<ZoneNodeData, 'zone'>>[]) => {
    setNodes((nds) => applyNodeChanges(changes, nds))
  }, [])

  const edges = useMemo(() => {
    return zoneFlow.pairs.map((pair) => {
      const color = costColor(pair.cost, zoneFlow.maxPairCost)
      const widthPx = Math.max(1.5, Math.min(7, 1.5 + 5.5 * Math.sqrt(pair.cost / (zoneFlow.maxPairCost || 1))))
      const marker: EdgeMarker = { type: MarkerType.ArrowClosed, color, width: 22, height: 22 }
      return {
        id: `${pair.src}>${pair.dst}`,
        source: pair.src,
        target: pair.dst,
        type: 'zoneFlow' as const,
        markerEnd: marker,
        data: { pair, color, widthPx } satisfies ZoneEdgeData,
      }
    })
  }, [zoneFlow])

  const totalCost = zoneFlow.pairs.reduce((s, p) => s + p.cost, 0)
  const totalGB = zoneFlow.pairs.reduce((s, p) => s + p.gb, 0)

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

      <main>
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
            <h2>Zone-to-zone traffic map</h2>
            <select value={filterKey} onChange={(e) => setFilterKey(e.target.value)}>
              <option value="">All workloads</option>
              {workloadOptions.map(([key, v]) => (
                <option key={key} value={key}>
                  {v.label} — {fmtUSD(v.cost)}
                </option>
              ))}
            </select>
          </div>
          <div className="flow-summary">
            {zoneFlow.zones.length} zone{zoneFlow.zones.length === 1 ? '' : 's'} ·{' '}
            {zoneFlow.pairs.length} route{zoneFlow.pairs.length === 1 ? '' : 's'} · {fmtUSD(totalCost)} ·{' '}
            {fmtGB(totalGB)}
          </div>
          <div className="legend">
            <span>Drag zones · scroll or pinch to zoom</span>
          </div>
          <div className="flow-canvas">
            {zoneFlow.pairs.length === 0 ? (
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
                  zonePositionsRef.current.set(node.id, node.position)
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

        <div className="card">
          <h2>Top offenders</h2>
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
