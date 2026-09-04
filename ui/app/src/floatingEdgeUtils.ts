import { Position, type InternalNode } from '@xyflow/react'

// Standard "floating edge" geometry helpers (this is the well-known xyflow pattern for edges
// that connect to the correct point on a node's border regardless of where the node currently
// is — required here because zone nodes are draggable, so a fixed handle position would produce
// edges that visibly detach from the box the moment you drag it). Given two node's current
// absolute positions + measured dimensions, this computes where a straight line between their
// centers crosses each node's rectangular border.

function getNodeIntersection(intersectionNode: InternalNode, targetNode: InternalNode) {
  const { width: intersectionNodeWidth, height: intersectionNodeHeight } = intersectionNode.measured
  const intersectionNodePosition = intersectionNode.internals.positionAbsolute
  const targetPosition = targetNode.internals.positionAbsolute

  const w = (intersectionNodeWidth ?? 0) / 2
  const h = (intersectionNodeHeight ?? 0) / 2

  const x2 = intersectionNodePosition.x + w
  const y2 = intersectionNodePosition.y + h
  const x1 = targetPosition.x + ((targetNode.measured.width ?? 0)) / 2
  const y1 = targetPosition.y + ((targetNode.measured.height ?? 0)) / 2

  const xx1 = (x1 - x2) / (2 * w) - (y1 - y2) / (2 * h)
  const yy1 = (x1 - x2) / (2 * w) + (y1 - y2) / (2 * h)
  const a = 1 / (Math.abs(xx1) + Math.abs(yy1) || 1)
  const xx3 = a * xx1
  const yy3 = a * yy1
  const x = w * (xx3 + yy3) + x2
  const y = h * (-xx3 + yy3) + y2

  return { x, y }
}

function getEdgePosition(node: InternalNode, intersectionPoint: { x: number; y: number }) {
  const n = node.internals.positionAbsolute
  const nx = Math.round(n.x)
  const ny = Math.round(n.y)
  const px = Math.round(intersectionPoint.x)
  const py = Math.round(intersectionPoint.y)
  const w = node.measured.width ?? 0
  const h = node.measured.height ?? 0

  if (px <= nx + 1) return Position.Left
  if (px >= nx + w - 1) return Position.Right
  if (py <= ny + 1) return Position.Top
  if (py >= ny + h - 1) return Position.Bottom
  return Position.Top
}

/** Returns the two border-intersection points (and which side of each box they land on) for
 * the straight line between the centers of `source` and `target`. Recomputed on every render
 * from each node's live position, so dragging a zone box keeps every edge correctly attached. */
export function getFloatingEdgeParams(source: InternalNode, target: InternalNode) {
  const sourceIntersection = getNodeIntersection(source, target)
  const targetIntersection = getNodeIntersection(target, source)
  const sourcePos = getEdgePosition(source, sourceIntersection)
  const targetPos = getEdgePosition(target, targetIntersection)
  return {
    sx: sourceIntersection.x,
    sy: sourceIntersection.y,
    tx: targetIntersection.x,
    ty: targetIntersection.y,
    sourcePos,
    targetPos,
  }
}
