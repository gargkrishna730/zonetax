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
import type { FlowPair } from '../flowGraph'
import type { FlowBoxNode } from './FlowBoxNode'

export type FlowGraphEdgeData = {
  pair: FlowPair
  color: string
  widthPx: number
  /** "Route" for the zone view (labels a workload breakdown), "Zone route" for the workload
   * view (labels a zone-pair breakdown) — keeps the hover tooltip's terminology correct for
   * whichever view is active instead of always saying one or the other. */
  breakdownHeading: string
  /** Opens the persistent drill-down panel for this edge — a hover tooltip alone can't hold a
   * full, sortable, click-through-able list, and caps at "top 5" which isn't enough for a real
   * investigation. Undefined when there's nothing further to drill into (e.g. already at the
   * most granular workload-to-workload view). */
  onSelect?: () => void
  /** True when a node-focus filter is active and this edge does NOT touch the focused node —
   * rendered faded so the focused node's actual inbound/outbound routes stand out clearly. */
  dimmed?: boolean
}

export type FlowGraphEdge = Edge<FlowGraphEdgeData, 'flowGraph'>

/** A "floating" edge (attaches to whichever side of each box currently faces the other node,
 * recomputed live as nodes are dragged — see floatingEdgeUtils.ts) drawn as a curved bezier with
 * an arrowhead, a $-cost pill at its midpoint, and a hover tooltip breaking the route down by
 * contributing sub-item (workload for the zone view, zone-route for the workload view). Shared
 * by both views so drag/zoom/tooltip behavior can't drift between them. */
export function FlowGraphEdge({ id, source, target, data, markerEnd }: EdgeProps<FlowGraphEdge>) {
  const sourceNode = useInternalNode<FlowBoxNode>(source)
  const targetNode = useInternalNode<FlowBoxNode>(target)
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

  const { pair, color, widthPx, breakdownHeading, onSelect, dimmed } = data
  const topBreakdown = Array.from(pair.breakdown.values())
    .sort((a, b) => b.cost - a.cost)
    .slice(0, 5)
  const moreCount = pair.breakdown.size - topBreakdown.length

  return (
    <>
      <BaseEdge
        id={id}
        path={edgePath}
        markerEnd={markerEnd}
        style={{
          stroke: color,
          strokeWidth: widthPx,
          cursor: onSelect ? 'pointer' : 'default',
          opacity: dimmed ? 0.12 : 1,
          transition: 'opacity 150ms',
        }}
      />
      {/* Wide invisible hit-path so thin, low-cost edges are still easy to hover/click — a thin
          visible stroke is bad UX to target precisely, especially once zoomed out. */}
      <path
        d={edgePath}
        fill="none"
        stroke="transparent"
        strokeWidth={18}
        onMouseEnter={showTooltip}
        onMouseLeave={hideTooltip}
        onClick={onSelect}
        style={{ cursor: onSelect ? 'pointer' : 'default' }}
      />
      <EdgeLabelRenderer>
        <div
          className="edge-pill"
          style={{
            transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
            background: color,
            cursor: onSelect ? 'pointer' : 'default',
            opacity: dimmed ? 0.12 : 1,
          }}
          onMouseEnter={showTooltip}
          onMouseLeave={hideTooltip}
          onClick={onSelect}
        >
          {fmtUSDShort(pair.cost)}
        </div>
        {hovered && (
          <div
            className="edge-tooltip"
            style={{ transform: `translate(-50%, 12px) translate(${labelX}px, ${labelY}px)` }}
          >
            <div className="tt-title">
              {sourceNode.data.label} → {targetNode.data.label}
            </div>
            <div className="tt-row">
              <span>Total cost</span>
              <b>{fmtUSD(pair.cost)}</b>
            </div>
            <div className="tt-row">
              <span>Traffic</span>
              <b>{fmtGB(pair.gb)}</b>
            </div>
            <div className="tt-subhead">{breakdownHeading}</div>
            {topBreakdown.map((b) => (
              <div className="tt-row" key={b.label}>
                <span>{b.label}</span>
                <b>{fmtUSD(b.cost)}</b>
              </div>
            ))}
            {onSelect && (
              <div className="tt-clickhint">
                {moreCount > 0 ? `+${moreCount} more · ` : ''}Click for full breakdown →
              </div>
            )}
          </div>
        )}
      </EdgeLabelRenderer>
    </>
  )
}
