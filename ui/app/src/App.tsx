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

import type { CostsResponse, HistoryRange, MapEntry, MapRange } from './types'
import { buildZoneFlow, buildWorkloadFlow, buildWorkloadPairBreakdown, srcWorkloadKey, dstWorkloadKey, type FlowGraph, type WorkloadPairBreakdown } from './flowGraph'
import { fmtGB, fmtUSD } from './format'
import { nodeMatchesQuery, neighborhoodOf } from './mapSearch'
import { useHistory } from './useHistory'
import { useMapData } from './useMapData'
import {
  emptyFilters,
  applyFilters,
  buildFilterOptions,
  extractZones,
  extractNamespaces,
  extractWorkloads,
  extractSrcWorkloads,
  extractDstWorkloads,
  extractRoutes,
  type MapFilters,
} from './mapFilters'
import { applyMapMode, MAP_MODES, type MapMode } from './mapModes'
import { computeMapSummary } from './mapSummary'
import { routeColor } from './routeColors'
import { FlowBoxNode, type FlowBoxNodeData } from './components/FlowBoxNode'
import { FlowGraphEdge, type FlowGraphEdgeData } from './components/FlowGraphEdge'
import { EdgeDrillDownPanel, type DrillDownSelection } from './components/EdgeDrillDownPanel'
import { MapLegend } from './components/MapLegend'
import { KpiPanel } from './components/KpiPanel'
import { CostHistoryChart } from './components/CostHistoryChart'
import { Toolbar } from './components/Toolbar'
import { FilterPanel, type FilterGroupDef, type RangeFilterDef, type ActiveChip } from './components/FilterPanel'
import { SummaryStrip } from './components/SummaryStrip'
import { RouteDetailsPanel } from './components/RouteDetailsPanel'
import { TopOffendersTable } from './components/TopOffendersTable'

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
    const id = requestAnimationFrame(() => fitView({ padding: 0.15, duration: 200 }))
    return () => cancelAnimationFrame(id)
  }, [viewMode, nodeCount, fitView])
  return null
}

/** Nodes are laid out on a static, deterministic circle (or side-by-side row for <=2 nodes) —
 * no force simulation. See flowGraph.ts's doc for the original rationale; unchanged here. */
function initialNodePosition(index: number, total: number): { x: number; y: number } {
  const boxWidth = 190
  if (total <= 2) {
    const width = 900
    const x = total === 1 ? width / 2 : index === 0 ? width * 0.25 : width * 0.75
    return { x, y: 260 }
  }
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
  // Legacy /api/v1/costs poll — still backs the collector-session KpiPanel/CostHistoryChart
  // section retained below the Service Map (cumulative "this session" metrics remain useful
  // alongside the new time-windowed map, and neither replaces the other).
  const [costs, setCosts] = useState<CostsResponse | null>(null)
  const [fetchError, setFetchError] = useState<string | null>(null)
  const [historyRange, setHistoryRange] = useState<HistoryRange>('24h')
  const history = useHistory(historyRange)

  // --- Cross-AZ Service Map state ---------------------------------------------------------
  const [mapRange, setMapRange] = useState<MapRange>('24h')
  const [customSince, setCustomSince] = useState<string | null>(null)
  const [customUntil, setCustomUntil] = useState<string | null>(null)
  const [autoRefresh, setAutoRefresh] = useState(true)
  const mapData = useMapData(mapRange, customSince, customUntil, autoRefresh ? 15_000 : 0x7fffffff)
  const [filters, setFilters] = useState<MapFilters>(emptyFilters())
  const [mapMode, setMapMode] = useState<MapMode>('all')
  const [maxConnections, setMaxConnections] = useState<number>(50)
  const [viewMode, setViewMode] = useState<ViewMode>('zone')
  const [selectedRoute, setSelectedRoute] = useState<MapEntry | null>(null)
  const [offendersCollapsed, setOffendersCollapsed] = useState(false)
  const [pairFilter, setPairFilter] = useState<{ srcKey: string; dstKey: string; srcLabel: string; dstLabel: string } | null>(null)
  const [drillDown, setDrillDown] = useState<DrillDownSelection | null>(null)
  const [mapFullscreen, setMapFullscreen] = useState(false)
  const [mapSearch, setMapSearch] = useState('')
  const [focusedNodeId, setFocusedNodeId] = useState<string | null>(null)
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

  useEffect(() => {
    if (!mapFullscreen) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setMapFullscreen(false)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [mapFullscreen])

  const allEntries: MapEntry[] = useMemo(() => mapData.data?.entries ?? [], [mapData.data])

  // Filters are applied BEFORE the map mode/cap — filtering must never be silently overridden
  // by "top N" truncation, and mode/cap operate on whatever the user has already scoped to.
  const filteredEntries = useMemo(() => applyFilters(allEntries, filters), [allEntries, filters])
  const pairScopedEntries = useMemo(
    () =>
      pairFilter
        ? filteredEntries.filter((e) => srcWorkloadKey(e) === pairFilter.srcKey && dstWorkloadKey(e) === pairFilter.dstKey)
        : filteredEntries,
    [filteredEntries, pairFilter],
  )
  const { shown: modeShownEntries, hiddenCount, totalRoutes } = useMemo(
    () =>
      applyMapMode(
        pairScopedEntries,
        mapMode,
        maxConnections,
        // Route granularity must match what's actually drawn for the ACTIVE view — zone-pair in
        // Zone->Zone, workload-pair in Workload->Workload. See mapModes.ts's doc for the real
        // bug this fixes: capping was always keyed by zone-pair, silently leaving the Workload
        // view's much larger route count completely uncapped.
        viewMode === 'zone' ? (e) => e.src_zone + '>' + e.dst_zone : (e) => srcWorkloadKey(e) + '>' + dstWorkloadKey(e),
      ),
    [pairScopedEntries, mapMode, maxConnections, viewMode],
  )

  const summary = useMemo(() => computeMapSummary(filteredEntries), [filteredEntries])
  const crossAZTrafficPercent =
    costs?.totals && costs.totals.cross_az_gb + costs.totals.same_az_gb > 0
      ? (100 * costs.totals.cross_az_gb) / (costs.totals.cross_az_gb + costs.totals.same_az_gb)
      : null

  // --- Filter panel option groups (counts reflect the CURRENT filtered set, honestly live) ---
  const zoneOptions = useMemo(() => buildFilterOptions(allEntries, extractZones, (v) => v), [allEntries])
  const namespaceOptions = useMemo(() => buildFilterOptions(allEntries, extractNamespaces, (v) => v), [allEntries])
  const workloadLabelOf = useCallback(
    (key: string) => {
      const found = allEntries.find((e) => srcWorkloadKey(e) === key || dstWorkloadKey(e) === key)
      if (!found) return key
      return srcWorkloadKey(found) === key ? found.src_workload : found.dst_workload
    },
    [allEntries],
  )
  const workloadOptions = useMemo(() => buildFilterOptions(allEntries, extractWorkloads, workloadLabelOf), [allEntries, workloadLabelOf])
  const srcWorkloadOptions = useMemo(() => buildFilterOptions(allEntries, extractSrcWorkloads, workloadLabelOf), [allEntries, workloadLabelOf])
  const dstWorkloadOptions = useMemo(() => buildFilterOptions(allEntries, extractDstWorkloads, workloadLabelOf), [allEntries, workloadLabelOf])
  const routeOptions = useMemo(() => buildFilterOptions(allEntries, extractRoutes, (v) => v.replace('>', ' → ')), [allEntries])

  function toggleInSet(setKey: keyof MapFilters, value: string) {
    setFilters((f) => {
      const next = { ...f, [setKey]: new Set(f[setKey] as Set<string>) } as MapFilters
      const s = next[setKey] as Set<string>
      if (s.has(value)) s.delete(value)
      else s.add(value)
      return next
    })
  }
  function selectAllIn(setKey: keyof MapFilters, values: string[]) {
    setFilters((f) => ({ ...f, [setKey]: new Set(values) }))
  }
  function clearSet(setKey: keyof MapFilters) {
    setFilters((f) => ({ ...f, [setKey]: new Set() }))
  }

  const filterGroups: FilterGroupDef[] = [
    { key: 'zones', title: 'Availability zone', options: zoneOptions, selected: filters.zones, onToggle: (v) => toggleInSet('zones', v), onSelectAll: () => selectAllIn('zones', zoneOptions.map((o) => o.value)), onClearGroup: () => clearSet('zones'), searchable: true },
    { key: 'namespaces', title: 'Namespace', options: namespaceOptions, selected: filters.namespaces, onToggle: (v) => toggleInSet('namespaces', v), onSelectAll: () => selectAllIn('namespaces', namespaceOptions.map((o) => o.value)), onClearGroup: () => clearSet('namespaces'), searchable: true },
    { key: 'workloads', title: 'Workload / service', options: workloadOptions, selected: filters.workloads, onToggle: (v) => toggleInSet('workloads', v), onSelectAll: () => selectAllIn('workloads', workloadOptions.map((o) => o.value)), onClearGroup: () => clearSet('workloads'), searchable: true },
    { key: 'srcWorkloads', title: 'Source workload', options: srcWorkloadOptions, selected: filters.srcWorkloads, onToggle: (v) => toggleInSet('srcWorkloads', v), onSelectAll: () => selectAllIn('srcWorkloads', srcWorkloadOptions.map((o) => o.value)), onClearGroup: () => clearSet('srcWorkloads'), searchable: true },
    { key: 'dstWorkloads', title: 'Destination workload', options: dstWorkloadOptions, selected: filters.dstWorkloads, onToggle: (v) => toggleInSet('dstWorkloads', v), onSelectAll: () => selectAllIn('dstWorkloads', dstWorkloadOptions.map((o) => o.value)), onClearGroup: () => clearSet('dstWorkloads'), searchable: true },
    { key: 'routes', title: 'Cross-AZ route', options: routeOptions, selected: filters.routes, onToggle: (v) => toggleInSet('routes', v), onSelectAll: () => selectAllIn('routes', routeOptions.map((o) => o.value)), onClearGroup: () => clearSet('routes'), searchable: true },
  ]
  const rangeFilters: RangeFilterDef[] = [
    { key: 'cost', title: 'Cost range', min: filters.costMin, max: filters.costMax, onMinChange: (v) => setFilters((f) => ({ ...f, costMin: v })), onMaxChange: (v) => setFilters((f) => ({ ...f, costMax: v })), unit: 'USD' },
    { key: 'traffic', title: 'Traffic range', min: filters.trafficMinGB, max: filters.trafficMaxGB, onMinChange: (v) => setFilters((f) => ({ ...f, trafficMinGB: v })), onMaxChange: (v) => setFilters((f) => ({ ...f, trafficMaxGB: v })), unit: 'GB' },
  ]
  const activeChips: ActiveChip[] = useMemo(() => {
    const chips: ActiveChip[] = []
    const addAll = (values: Set<string>, key: string, labelOf: (v: string) => string, clear: (v: string) => void) => {
      for (const v of values) chips.push({ key: key + ':' + v, label: `${key}: ${labelOf(v)}`, onRemove: () => clear(v) })
    }
    addAll(filters.zones, 'AZ', (v) => v, (v) => toggleInSet('zones', v))
    addAll(filters.namespaces, 'ns', (v) => v, (v) => toggleInSet('namespaces', v))
    addAll(filters.workloads, 'workload', workloadLabelOf, (v) => toggleInSet('workloads', v))
    addAll(filters.srcWorkloads, 'src', workloadLabelOf, (v) => toggleInSet('srcWorkloads', v))
    addAll(filters.dstWorkloads, 'dst', workloadLabelOf, (v) => toggleInSet('dstWorkloads', v))
    addAll(filters.routes, 'route', (v) => v.replace('>', '→'), (v) => toggleInSet('routes', v))
    if (filters.costMin !== null && filters.costMin > 0) chips.push({ key: 'costMin', label: `cost ≥ $${filters.costMin}`, onRemove: () => setFilters((f) => ({ ...f, costMin: null })) })
    if (filters.costMax !== null && filters.costMax > 0) chips.push({ key: 'costMax', label: `cost ≤ $${filters.costMax}`, onRemove: () => setFilters((f) => ({ ...f, costMax: null })) })
    if (filters.trafficMinGB !== null && filters.trafficMinGB > 0) chips.push({ key: 'trafficMin', label: `traffic ≥ ${filters.trafficMinGB}GB`, onRemove: () => setFilters((f) => ({ ...f, trafficMinGB: null })) })
    if (filters.trafficMaxGB !== null && filters.trafficMaxGB > 0) chips.push({ key: 'trafficMax', label: `traffic ≤ ${filters.trafficMaxGB}GB`, onRemove: () => setFilters((f) => ({ ...f, trafficMaxGB: null })) })
    return chips
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters, workloadLabelOf])

  // --- Map graph (reuses flowGraph.ts's existing aggregation — MapEntry is structurally
  // identical to CostEntry, so buildZoneFlow/buildWorkloadFlow apply unchanged, satisfying the
  // "reuse existing calculations, don't recompute in the UI" constraint). ---------------------
  const flowGraph: FlowGraph = useMemo(
    () => (viewMode === 'zone' ? buildZoneFlow(modeShownEntries) : buildWorkloadFlow(modeShownEntries)),
    [modeShownEntries, viewMode],
  )

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
        return { id: n.id, type: 'flowBox' as const, position, data: { label: n.label, sublabel: n.sublabel } }
      })
    })
  }, [flowGraph, viewMode])

  const onNodesChange = useCallback((changes: NodeChange<Node<FlowBoxNodeData, 'flowBox'>>[]) => {
    setNodes((nds) => applyNodeChanges(changes, nds))
  }, [])

  useEffect(() => {
    setFocusedNodeId(null)
  }, [viewMode, pairFilter, mapMode])

  const nodesById = useMemo(() => new Map(flowGraph.nodes.map((n) => [n.id, n])), [flowGraph.nodes])
  const focusedNeighborhood = useMemo(
    () => (focusedNodeId ? neighborhoodOf(focusedNodeId, flowGraph.pairs) : null),
    [focusedNodeId, flowGraph.pairs],
  )
  const isNodeDimmed = useCallback(
    (id: string) => {
      const node = nodesById.get(id)
      if (mapSearch.trim() && (!node || !nodeMatchesQuery(node, mapSearch))) return true
      if (focusedNeighborhood && !focusedNeighborhood.has(id)) return true
      return false
    },
    [nodesById, mapSearch, focusedNeighborhood],
  )

  const maxPairCostForRamp = flowGraph.maxPairCost || 1
  const edges = useMemo(() => {
    const breakdownHeading = viewMode === 'zone' ? 'Top workloads' : 'Top zone routes'
    return flowGraph.pairs.map((pair) => {
      const isSelected = selectedRoute && viewMode === 'zone' && pair.srcId === selectedRoute.src_zone && pair.dstId === selectedRoute.dst_zone
      const color = isSelected ? routeColor('selected') : mapData.error ? routeColor('unavailable') : !mapData.data?.complete ? routeColor('stale', pair.cost / maxPairCostForRamp) : routeColor('normal', pair.cost / maxPairCostForRamp)
      const widthPx = Math.max(1.5, Math.min(7, 1.5 + 5.5 * Math.sqrt(pair.cost / maxPairCostForRamp)))
      const marker: EdgeMarker = { type: MarkerType.ArrowClosed, color, width: 22, height: 22 }
      const srcNode = flowGraph.nodes.find((n) => n.id === pair.srcId)
      const dstNode = flowGraph.nodes.find((n) => n.id === pair.dstId)
      // Zone view: clicking a route opens the drill-down panel (richer — a single zone-pair can
      // span many real workload pairs, and the panel is a full sortable/clickable list, not just
      // a route-details snapshot). Workload view: a route IS already the most granular thing, so
      // clicking it opens RouteDetailsPanel directly instead. These are deliberately mutually
      // exclusive per click — a real bug, caught via a live screenshot, had both panels able to
      // be open simultaneously in zone view, with RouteDetailsPanel's higher z-index silently
      // and completely hiding the drill-down panel underneath it, making the drill-down feature
      // unreachable by clicking a zone route. Also fixes a second issue found alongside it:
      // workload-view edges previously had NO click handler at all (onSelect was zone-view-only),
      // so clicking a route there did nothing.
      const onSelect =
        viewMode === 'zone'
          ? () => {
              setSelectedRoute(null)
              setDrillDown({
                srcLabel: srcNode?.label ?? pair.srcId,
                dstLabel: dstNode?.label ?? pair.dstId,
                totalCost: pair.cost,
                totalGb: pair.gb,
                pairs: buildWorkloadPairBreakdown(pair.entries),
              })
            }
          : () => {
              const first = pair.entries[0]
              if (first) {
                setDrillDown(null)
                setSelectedRoute(first)
              }
            }
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
  }, [flowGraph, viewMode, isNodeDimmed, selectedRoute, mapData.error, mapData.data?.complete, maxPairCostForRamp])

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

  const topOffenders = useMemo(() => [...pairScopedEntries].sort((a, b) => b.cost_usd - a.cost_usd).slice(0, 15), [pairScopedEntries])

  const dataStatus: 'loading' | 'error' | 'no-data' | 'no-match' | 'ok' = mapData.error
    ? 'error'
    : mapData.loading && !mapData.data
      ? 'loading'
      : !mapData.data?.has_data
        ? 'no-data'
        : filteredEntries.length === 0 && allEntries.length > 0
          ? 'no-match'
          : 'ok'

  return (
    <div className="app app-servicemap">
      <Toolbar
        cloud={mapData.data?.cloud || costs?.cloud}
        region={mapData.data?.region || costs?.region}
        range={mapRange}
        onRangeChange={setMapRange}
        customSince={customSince}
        customUntil={customUntil}
        onCustomRangeChange={(s, u) => {
          setCustomSince(s)
          setCustomUntil(u)
        }}
        data={mapData.data}
        loading={mapData.loading}
        error={mapData.error}
        autoRefresh={autoRefresh}
        onAutoRefreshChange={setAutoRefresh}
        onRefreshNow={mapData.refetch}
      />

      <div className="servicemap-body">
        <FilterPanel
          groups={filterGroups}
          ranges={rangeFilters}
          activeChips={activeChips}
          onResetAll={() => setFilters(emptyFilters())}
          maxConnections={maxConnections}
          onMaxConnectionsChange={setMaxConnections}
          hiddenRouteCount={hiddenCount}
          totalRouteCount={totalRoutes}
        />

        <main className="servicemap-main">
          <SummaryStrip
            summary={summary}
            range={mapRange}
            crossAZTrafficPercent={crossAZTrafficPercent}
            pricePerGBUSD={mapData.data?.price_per_gb_usd ?? costs?.totals.price_per_gb_usd ?? 0}
            pricePerGBDirectionUSD={mapData.data?.price_per_gb_direction_usd ?? costs?.totals.price_per_gb_direction_usd ?? 0}
            windowLabel={mapData.data && !mapData.data.complete ? 'Note: the current window is only partially observed — see the toolbar status.' : ''}
          />

          <div className={`card flow-card${mapFullscreen ? ' flow-fullscreen-anchor' : ''}`}>
            <div className="card-head">
              <h2>Traffic map</h2>
              <div className="head-controls">
                <div className="view-toggle" role="tablist" aria-label="Flow map view">
                  <button type="button" className={viewMode === 'zone' ? 'active' : ''} onClick={() => { setViewMode('zone'); setPairFilter(null) }} role="tab" aria-selected={viewMode === 'zone'}>
                    Zone → Zone
                  </button>
                  <button type="button" className={viewMode === 'workload' ? 'active' : ''} onClick={() => { setViewMode('workload'); setPairFilter(null) }} role="tab" aria-selected={viewMode === 'workload'}>
                    Workload → Workload
                  </button>
                </div>
                <select className="mode-select" value={mapMode} onChange={(e) => setMapMode(e.target.value as MapMode)} title={MAP_MODES.find((m) => m.value === mapMode)?.hint}>
                  {MAP_MODES.map((m) => (
                    <option key={m.value} value={m.value}>{m.label}</option>
                  ))}
                </select>
                <input type="text" className="map-search" placeholder="Search workload, namespace, or zone…" value={mapSearch} onChange={(e) => setMapSearch(e.target.value)} aria-label="Search the traffic map" />
                {pairFilter && (
                  <div className="pair-filter-chip">
                    <span>{pairFilter.srcLabel} → {pairFilter.dstLabel}</span>
                    <button type="button" onClick={() => setPairFilter(null)} aria-label="Clear pair filter">×</button>
                  </div>
                )}
                <button type="button" className="fullscreen-btn" onClick={() => setMapFullscreen((v) => !v)} title={mapFullscreen ? 'Exit fullscreen (Esc)' : 'Expand to fullscreen'} aria-pressed={mapFullscreen}>
                  {mapFullscreen ? '⤡ Exit fullscreen' : '⤢ Fullscreen'}
                </button>
              </div>
            </div>
            <div className="flow-summary">
              {flowGraph.nodes.length} {viewMode === 'zone' ? 'zone' : 'workload'}{flowGraph.nodes.length === 1 ? '' : 's'} · {flowGraph.pairs.length} route{flowGraph.pairs.length === 1 ? '' : 's'} · {fmtUSD(summary.totalCostUSD)} · {fmtGB(summary.totalGB)}
              {hiddenCount > 0 && <> · {hiddenCount} route{hiddenCount === 1 ? '' : 's'} beyond the Max connections limit (see filter panel)</>}
            </div>
            <MapLegend viewMode={viewMode} />
            {mapFullscreen && <div className="fullscreen-backdrop" onClick={() => setMapFullscreen(false)} />}
            <div className={`flow-canvas${mapFullscreen ? ' flow-canvas-fullscreen' : ''}`}>
              {focusedNodeId && (
                <div className="focus-chip">
                  <span>Focused: {nodesById.get(focusedNodeId)?.label ?? focusedNodeId}</span>
                  <button type="button" onClick={() => setFocusedNodeId(null)} aria-label="Clear focus">×</button>
                </div>
              )}
              {dataStatus === 'loading' ? (
                <div className="empty">Loading Cross-AZ Service Map…</div>
              ) : dataStatus === 'error' ? (
                <div className="empty empty-error">API error: {mapData.error}</div>
              ) : dataStatus === 'no-data' ? (
                <div className="empty">No cross-AZ traffic observed in this time window.</div>
              ) : dataStatus === 'no-match' ? (
                <div className="empty">No routes match the current filters — try Reset in the filter panel.</div>
              ) : flowGraph.pairs.length === 0 ? (
                <div className="empty">No cross-AZ traffic in the current window.</div>
              ) : (
                <ReactFlowProvider>
                  <ReactFlow
                    nodes={displayNodes}
                    edges={edges}
                    nodeTypes={nodeTypes}
                    edgeTypes={edgeTypes}
                    onNodesChange={onNodesChange}
                    onNodeDragStop={(_, node) => { nodePositionsRef.current.set(viewMode + ':' + node.id, node.position) }}
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
                    setPairFilter({ srcKey: p.srcKey, dstKey: p.dstKey, srcLabel: p.srcLabel, dstLabel: p.dstLabel })
                    setViewMode('workload')
                    setDrillDown(null)
                  }}
                />
              )}
              {selectedRoute && mapData.data && (
                <RouteDetailsPanel
                  entry={selectedRoute}
                  totalCrossAZCostUSD={summary.totalCostUSD}
                  pricePerGBUSD={mapData.data.price_per_gb_usd}
                  rangeStartUTC={mapData.data.range_start_utc}
                  rangeEndUTC={mapData.data.range_end_utc}
                  serverTimeUTC={mapData.data.server_time_utc}
                  onClose={() => setSelectedRoute(null)}
                />
              )}
            </div>
          </div>

          <TopOffendersTable
            entries={topOffenders}
            totalCrossAZCostUSD={summary.totalCostUSD}
            range={mapRange}
            onSelectRoute={(e) => {
              setDrillDown(null)
              setSelectedRoute(e)
            }}
            collapsed={offendersCollapsed}
            onToggleCollapsed={() => setOffendersCollapsed((c) => !c)}
          />

          <details className="session-history-details">
            <summary>Collector session &amp; history (cumulative-since-restart metrics)</summary>
            <KpiPanel costs={costs} fetchError={fetchError} />
            <CostHistoryChart range={historyRange} onRangeChange={setHistoryRange} history={history.data} loading={history.loading} error={history.error} />
          </details>
        </main>
      </div>

      <footer>
        ZoneTax · polling every 10s ·{' '}
        <a href="https://github.com/gargkrishna730/zonetax" target="_blank" rel="noreferrer">
          github.com/gargkrishna730/zonetax
        </a>
      </footer>
    </div>
  )
}
