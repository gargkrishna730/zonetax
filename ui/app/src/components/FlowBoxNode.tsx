import { Handle, Position, type NodeProps, type Node } from '@xyflow/react'

export type FlowBoxNodeData = {
  label: string
  sublabel?: string
  namespace?: string
  emphasis?: 'dimmed' | 'focused'
  onClick?: () => void
}

export type FlowBoxNode = Node<FlowBoxNodeData, 'flowBox'>

/** Box node for both map views. Focusable (ReactFlow sets tabindex); Enter/Space is handled by the
 * canvas in App.tsx (ReactFlow's wrapper receives the key event, not this div). Handles are hidden: edges float to the nearest border. */
export function FlowBoxNode({ data }: NodeProps<FlowBoxNode>) {
  return (
    <div
      className={`node${data.emphasis ? ` ${data.emphasis}` : ''}`}
      onClick={(e) => {
        e.stopPropagation()
        data.onClick?.()
      }}
    >
      <Handle type="source" position={Position.Top} style={{ visibility: 'hidden', pointerEvents: 'none' }} />
      <Handle type="target" position={Position.Top} style={{ visibility: 'hidden', pointerEvents: 'none' }} />
      {data.namespace && <div className="node-ns">{data.namespace}</div>}
      <div className="node-label" title={data.label}>{data.label}</div>
      {data.sublabel && <div className="node-sub">{data.sublabel}</div>}
    </div>
  )
}
