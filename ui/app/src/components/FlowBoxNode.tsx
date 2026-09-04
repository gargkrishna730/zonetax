import { Handle, Position, type NodeProps, type Node } from '@xyflow/react'

export type FlowBoxNodeData = {
  label: string
  sublabel?: string
}

export type FlowBoxNode = Node<FlowBoxNodeData, 'flowBox'>

/** Generic box node used by both the zone-to-zone and workload-to-workload views — a plain
 * label + small sublabel, matching the reference APM service-map's node style. Rendered as a
 * plain box with Handles on all four sides so ReactFlow can compute connections; the actual
 * edge attachment point is recomputed per-render by the floating-edge math in
 * floatingEdgeUtils.ts, so these Handles' exact position doesn't matter beyond satisfying
 * ReactFlow's requirement that a node have at least one. */
export function FlowBoxNode({ data }: NodeProps<FlowBoxNode>) {
  return (
    <div className="flow-box-node">
      <Handle type="source" position={Position.Top} style={{ visibility: 'hidden', pointerEvents: 'none' }} />
      <Handle type="target" position={Position.Top} style={{ visibility: 'hidden', pointerEvents: 'none' }} />
      <div className="flow-box-label">{data.label}</div>
      {data.sublabel && <div className="flow-box-sublabel">{data.sublabel}</div>}
    </div>
  )
}
