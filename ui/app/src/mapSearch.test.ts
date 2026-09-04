import { describe, expect, it } from 'vitest'
import { neighborhoodOf, nodeMatchesQuery, pairMatchesQuery } from './mapSearch'
import type { FlowNode, FlowPair } from './flowGraph'

function node(id: string, label: string, sublabel?: string): FlowNode {
  return { id, label, sublabel }
}

function pair(srcId: string, dstId: string): FlowPair {
  return { srcId, dstId, cost: 1, gb: 1, breakdown: new Map(), entries: [] }
}

describe('nodeMatchesQuery', () => {
  it('matches an empty query against everything', () => {
    expect(nodeMatchesQuery(node('a', 'signoz-otel-collector'), '')).toBe(true)
  })
  it('matches case-insensitively against the label', () => {
    expect(nodeMatchesQuery(node('a', 'signoz-otel-collector'), 'OTEL')).toBe(true)
  })
  it('matches against the sublabel (e.g. namespace)', () => {
    expect(nodeMatchesQuery(node('a', 'collector', 'monitoring'), 'monitoring')).toBe(true)
  })
  it('does not match an unrelated query', () => {
    expect(nodeMatchesQuery(node('a', 'collector'), 'zzz-nomatch')).toBe(false)
  })
})

describe('pairMatchesQuery', () => {
  const nodesById = new Map([
    ['src1', node('src1', 'frontend')],
    ['dst1', node('dst1', 'backend')],
  ])
  it('matches when the source node matches', () => {
    expect(pairMatchesQuery(pair('src1', 'dst1'), nodesById, 'front')).toBe(true)
  })
  it('matches when the destination node matches', () => {
    expect(pairMatchesQuery(pair('src1', 'dst1'), nodesById, 'back')).toBe(true)
  })
  it('does not match when neither endpoint matches', () => {
    expect(pairMatchesQuery(pair('src1', 'dst1'), nodesById, 'database')).toBe(false)
  })
})

describe('neighborhoodOf', () => {
  const pairs = [pair('a', 'b'), pair('b', 'c'), pair('d', 'e')]
  it('includes the focused node itself', () => {
    expect(neighborhoodOf('a', pairs).has('a')).toBe(true)
  })
  it('includes outbound-connected nodes', () => {
    expect(neighborhoodOf('a', pairs).has('b')).toBe(true)
  })
  it('includes inbound-connected nodes', () => {
    expect(neighborhoodOf('c', pairs).has('b')).toBe(true)
  })
  it('excludes unconnected nodes', () => {
    expect(neighborhoodOf('a', pairs).has('d')).toBe(false)
    expect(neighborhoodOf('a', pairs).has('e')).toBe(false)
  })
})
