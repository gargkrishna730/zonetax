import { useCallback, useState } from 'react'
import { BaseEdge, EdgeLabelRenderer, getBezierPath, useInternalNode, useStore, type EdgeProps, type Edge } from '@xyflow/react'
import { getFloatingEdgeParams } from '../floatingEdgeUtils'
import { fmtGB, fmtUSD, fmtUSDShort } from '../format'
import type { FlowPair } from '../flowGraph'
import type { FlowBoxNode } from './FlowBoxNode'

export type FlowGraphEdgeData = {
  pair: FlowPair
  color: string
  widthPx: number
  breakdownHeading: string
  onSelect?: () => void
  dimmed?: boolean
  partial?: boolean
  selected?: boolean
  /** Many-route views: label only routes that matter; the rest show a small dot (still a
   * focusable button with full details on hover/focus). */
  compact?: boolean
}

export type FlowGraphEdge = Edge<FlowGraphEdgeData, 'flowGraph'>

/** Floating bezier edge. Cost is encoded three ways (colour, width, label) so colour is never the
 * only cue. Partial windows draw dashed; a selected route gets a focus-coloured halo. The pill is
 * a real button so it is reachable by keyboard; its hover details also show on focus. */
export function FlowGraphEdge({ id, source, target, data, markerEnd }: EdgeProps<FlowGraphEdge>) {
  const sourceNode = useInternalNode<FlowBoxNode>(source)
  const targetNode = useInternalNode<FlowBoxNode>(target)
  const [hovered, setHovered] = useState(false)
  const hasReverse = useStore((st) => st.edges.some((e) => e.source === target && e.target === source))
  const show = useCallback(() => setHovered(true), [])
  const hide = useCallback(() => setHovered(false), [])

  if (!sourceNode || !targetNode || !data) return null

  const { sx, sy, tx, ty, sourcePos, targetPos } = getFloatingEdgeParams(sourceNode, targetNode)
  let edgePath: string, labelX: number, labelY: number
  if (hasReverse) {
    // Traffic flows both ways between these two nodes: bow each direction to its own side
    // (perpendicular offset from the s->t normal, which flips for the reverse edge) so the two
    // routes and their cost labels never sit on top of each other.
    const dx = tx - sx, dy = ty - sy
    const len = Math.hypot(dx, dy) || 1
    const off = Math.min(60, len * 0.18)
    const cx = (sx + tx) / 2 + (-dy / len) * off
    const cy = (sy + ty) / 2 + (dx / len) * off
    edgePath = `M ${sx},${sy} Q ${cx},${cy} ${tx},${ty}`
    labelX = 0.25 * sx + 0.5 * cx + 0.25 * tx
    labelY = 0.25 * sy + 0.5 * cy + 0.25 * ty
  } else {
    ;[edgePath, labelX, labelY] = getBezierPath({ sourceX: sx, sourceY: sy, sourcePosition: sourcePos, targetX: tx, targetY: ty, targetPosition: targetPos, curvature: 0.35 })
  }

  const { pair, color, widthPx, breakdownHeading, onSelect, dimmed, partial, selected, compact } = data
  const top = Array.from(pair.breakdown.values()).sort((a, b) => b.cost - a.cost).slice(0, 5)
  const more = pair.breakdown.size - top.length
  const srcLabel = sourceNode.data.label
  const dstLabel = targetNode.data.label
  const opacity = dimmed ? 0.12 : 1
  // Dash length scales with width so thick partial edges read as dashed, not as a broken line.
  const dash = `${Math.round(widthPx * 3 + 6)} ${Math.round(widthPx * 1.5 + 5)}`

  return (
    <>
      {selected && <path d={edgePath} fill="none" stroke="var(--focus)" strokeWidth={widthPx + 6} strokeOpacity={0.45} />}
      {/* Contrast casing: a thin outline in the canvas ink colour keeps pale low-cost edges visible on light backgrounds. */}
      <path d={edgePath} fill="none" stroke="var(--edge-casing)" strokeWidth={widthPx + 2} strokeOpacity={dimmed ? 0.05 : 1} strokeLinecap="round" />
      <BaseEdge
        id={id}
        path={edgePath}
        markerEnd={markerEnd}
        className="edge-main"
        style={{ stroke: color, strokeWidth: widthPx, strokeDasharray: partial ? dash : undefined, strokeLinecap: 'round', opacity, transition: 'opacity 150ms', ['--w' as string]: `${widthPx}px` }}
      />
      <path d={edgePath} fill="none" stroke="transparent" strokeWidth={18} onMouseEnter={show} onMouseLeave={hide} onClick={onSelect} style={{ cursor: onSelect ? 'pointer' : 'default' }} />
      <EdgeLabelRenderer>
        <button
          type="button"
          className={`pill nodrag nopan${compact && !hovered && !selected ? ' mini' : ''}${partial ? ' partial' : ''}${selected ? ' selected' : ''}${dimmed ? ' dimmed' : ''}`}
          style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`, borderColor: color }}
          onMouseEnter={show}
          onMouseLeave={hide}
          onFocus={show}
          onBlur={hide}
          onClick={onSelect}
          tabIndex={dimmed ? -1 : 0}
          aria-label={`${srcLabel} to ${dstLabel}: ${fmtUSD(pair.cost)}, ${fmtGB(pair.gb)}${partial ? ', partial window' : ''}. Open breakdown.`}
        >
          <i style={{ background: color }} aria-hidden="true" />
          {(!compact || hovered || selected) && fmtUSDShort(pair.cost)}
        </button>
        {hovered && (
          <div className="tooltip" role="tooltip" style={{ transform: `translate(-50%, 16px) translate(${labelX}px, ${labelY}px)` }}>
            <div className="tt-title">{srcLabel} → {dstLabel}</div>
            <div className="tt-row"><span>Cost</span><b>{fmtUSD(pair.cost)}</b></div>
            <div className="tt-row"><span>Traffic</span><b>{fmtGB(pair.gb)}</b></div>
            {top.length > 1 && (
              <>
                <div className="tt-sub">{breakdownHeading}</div>
                {top.map((b) => (
                  <div className="tt-row" key={b.label}><span>{b.label}</span><b>{fmtUSDShort(b.cost)}</b></div>
                ))}
              </>
            )}
            {onSelect && <div className="tt-hint">{more > 0 ? `+${more} more · ` : ''}Click for full breakdown</div>}
          </div>
        )}
      </EdgeLabelRenderer>
    </>
  )
}
