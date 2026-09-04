import { Handle, Position, type NodeProps, type Node } from '@xyflow/react'

export type FlowBoxNodeData = {
  label: string
  sublabel?: string
  /** Visual state for search/focus interactions: 'dimmed' when a search query or a focused
   * node's neighborhood excludes this node, 'focused' for the explicitly clicked node itself,
   * undefined/'normal' otherwise. Kept as a single enum (not two booleans) so a node is never
   * accidentally both dimmed and focused at once. */
  emphasis?: 'dimmed' | 'focused'
  onClick?: () => void
}

export type FlowBoxNode = Node<FlowBoxNodeData, 'flowBox'>

/** Generic box node used by both the zone-to-zone and workload-to-workload views — a plain
 * label + small sublabel, matching the reference APM service-map's node style. Rendered as a
 * plain box with Handles on all four sides so ReactFlow can compute connections; the actual
 * edge attachment point is recomputed per-render by the floating-edge math in
 * floatingEdgeUtils.ts, so these Handles' exact position doesn't matter beyond satisfying
 * ReactFlow's requirement that a node have at least one. Clicking a node (not dragging it)
 * fires onClick, used by the map's "focus on node" feature to highlight just its inbound/
 * outbound routes. */
export function FlowBoxNode({ data }: NodeProps<FlowBoxNode>) {
  return (
    <div
      className={`flow-box-node${data.emphasis ? ` emphasis-${data.emphasis}` : ''}`}
      onClick={(e) => {
        e.stopPropagation()
        data.onClick?.()
      }}
    >
      <Handle type="source" position={Position.Top} style={{ visibility: 'hidden', pointerEvents: 'none' }} />
      <Handle type="target" position={Position.Top} style={{ visibility: 'hidden', pointerEvents: 'none' }} />
      <div className="flow-box-label">{data.label}</div>
      {data.sublabel && <div className="flow-box-sublabel">{data.sublabel}</div>}
    </div>
  )
}
