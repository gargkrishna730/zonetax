import { Handle, Position, type NodeProps, type Node } from '@xyflow/react'
import { fmtUSDShort } from '../format'

export type ZoneNodeData = {
  zone: string
  out: number
  in: number
  touched: boolean // true when this zone participates in the currently-focused workload's routes
}

export type ZoneNode = Node<ZoneNodeData, 'zone'>

/** Custom node representing one availability zone. Rendered as a plain box (matches the
 * reference APM service-map screenshot's node style) with Handles on all four sides so
 * ReactFlow can compute connections; the actual edge attachment point is recomputed per-render
 * by the floating-edge math in floatingEdgeUtils.ts, so these Handles' exact position doesn't
 * matter much beyond satisfying ReactFlow's requirement that a node have at least one. */
export function ZoneNode({ data }: NodeProps<ZoneNode>) {
  return (
    <div className={`zone-node${data.touched ? ' zone-node-touched' : ''}`}>
      <Handle type="source" position={Position.Top} style={{ visibility: 'hidden', pointerEvents: 'none' }} />
      <Handle type="target" position={Position.Top} style={{ visibility: 'hidden', pointerEvents: 'none' }} />
      <div className="zone-node-label">{data.zone}</div>
      <div className="zone-node-sub">
        out {fmtUSDShort(data.out)} · in {fmtUSDShort(data.in)}
      </div>
    </div>
  )
}
