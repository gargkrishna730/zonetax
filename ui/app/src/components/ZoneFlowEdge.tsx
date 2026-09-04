import { useCallback, useState } from 'react'
import {
  BaseEdge,
  EdgeLabelRenderer,
  getBezierPath,
  useInternalNode,
  type EdgeProps,
  type Edge,
} from '@xyflow/react'
import { getFloatingEdgeParams } from '../floatingEdgeUtils'
import { fmtGB, fmtUSD, fmtUSDShort } from '../format'
import type { ZonePairFlow } from '../zoneFlow'

export type ZoneEdgeData = {
  pair: ZonePairFlow
  color: string
  widthPx: number
}

export type ZoneEdge = Edge<ZoneEdgeData, 'zoneFlow'>

/** A "floating" edge (attaches to whichever side of each zone box currently faces the other
 * box, recomputed live as nodes are dragged — see floatingEdgeUtils.ts) drawn as a curved
 * bezier with an arrowhead, a $-cost pill at its midpoint, and a hover tooltip breaking the
 * route down by contributing workload. This is the direct replacement for the earlier
 * hand-rolled SVG path math — ReactFlow handles the attach-point/redraw-on-drag bookkeeping
 * that was previously reimplemented by hand across three prior iterations. */
export function ZoneFlowEdge({ id, source, target, data, markerEnd }: EdgeProps<ZoneEdge>) {
  const sourceNode = useInternalNode(source)
  const targetNode = useInternalNode(target)
  const [hovered, setHovered] = useState(false)

  const showTooltip = useCallback(() => setHovered(true), [])
  const hideTooltip = useCallback(() => setHovered(false), [])

  if (!sourceNode || !targetNode || !data) return null

  const { sx, sy, tx, ty, sourcePos, targetPos } = getFloatingEdgeParams(sourceNode, targetNode)
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX: sx,
    sourceY: sy,
    sourcePosition: sourcePos,
    targetX: tx,
    targetY: ty,
    targetPosition: targetPos,
    curvature: 0.35,
  })

  const { pair, color, widthPx } = data
  const topWorkloads = Array.from(pair.workloads.values())
    .sort((a, b) => b.cost - a.cost)
    .slice(0, 5)

  return (
    <>
      <BaseEdge
        id={id}
        path={edgePath}
        markerEnd={markerEnd}
        style={{ stroke: color, strokeWidth: widthPx, cursor: 'pointer' }}
      />
      {/* Wide invisible hit-path so thin, low-cost edges are still easy to hover — a thin
          visible stroke is bad UX to hover precisely, especially once zoomed out. */}
      <path
        d={edgePath}
        fill="none"
        stroke="transparent"
        strokeWidth={18}
        onMouseEnter={showTooltip}
        onMouseLeave={hideTooltip}
        style={{ cursor: 'pointer' }}
      />
      <EdgeLabelRenderer>
        <div
          className="edge-pill"
          style={{
            transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
            background: color,
          }}
          onMouseEnter={showTooltip}
          onMouseLeave={hideTooltip}
        >
          {fmtUSDShort(pair.cost)}
        </div>
        {hovered && (
          <div
            className="edge-tooltip"
            style={{ transform: `translate(-50%, 12px) translate(${labelX}px, ${labelY}px)` }}
          >
            <div className="tt-title">
              {pair.src} → {pair.dst}
            </div>
            <div className="tt-row">
              <span>Total cost</span>
              <b>{fmtUSD(pair.cost)}</b>
            </div>
            <div className="tt-row">
              <span>Traffic</span>
              <b>{fmtGB(pair.gb)}</b>
            </div>
            <div className="tt-subhead">Top workloads</div>
            {topWorkloads.map((w) => (
              <div className="tt-row" key={w.label}>
                <span>{w.label}</span>
                <b>{fmtUSD(w.cost)}</b>
              </div>
            ))}
          </div>
        )}
      </EdgeLabelRenderer>
    </>
  )
}
