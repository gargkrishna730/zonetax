import { useEffect, useRef } from 'react'
import { fmtGB, fmtUSD } from '../format'
import { costScale } from '../theme'
import type { WorkloadPairBreakdown } from '../flowGraph'

export interface DrillDownSelection {
  srcLabel: string
  dstLabel: string
  totalCost: number
  totalGb: number
  pairs: WorkloadPairBreakdown[]
}

/** Route breakdown panel: every workload pair behind a zone route, sorted by cost. Focus moves
 * into the panel when it opens and Escape closes it. */
export function EdgeDrillDownPanel({ selection, onClose, onSelectPair }: { selection: DrillDownSelection; onClose: () => void; onSelectPair: (pair: WorkloadPairBreakdown) => void }) {
  const maxCost = selection.pairs[0]?.cost ?? 0
  const closeRef = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    closeRef.current?.focus()
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <section className="panel" role="dialog" aria-modal="false" aria-labelledby="dd-title">
      <div className="panel-head">
        <div>
          <h3 id="dd-title">{selection.srcLabel} → {selection.dstLabel}</h3>
          <div className="panel-sub num">
            {fmtUSD(selection.totalCost)} · {fmtGB(selection.totalGb)} · {selection.pairs.length} workload pair{selection.pairs.length === 1 ? '' : 's'}
          </div>
        </div>
        <button ref={closeRef} type="button" className="btn btn-ghost btn-icon close" onClick={onClose} aria-label="Close breakdown">✕</button>
      </div>
      <div className="panel-hint">Select a pair to view it on the workload map.</div>
      <div className="panel-body">
        {selection.pairs.map((p) => {
          const color = costScale(maxCost > 0 ? p.cost / maxCost : 0)
          const share = selection.totalCost > 0 ? (100 * p.cost) / selection.totalCost : 0
          return (
            <button type="button" key={p.srcKey + '>' + p.dstKey} className="panel-row" onClick={() => onSelectPair(p)}>
              <span className="dot" style={{ background: color }} aria-hidden="true" />
              <span className="route">
                <b title={`${p.srcLabel} → ${p.dstLabel}`}>{p.srcLabel} → {p.dstLabel}</b>
                <span>{share.toFixed(1)}% of this route</span>
              </span>
              <span className="metric">
                <b>{fmtUSD(p.cost)}</b>
                <span>{fmtGB(p.gb)}</span>
              </span>
            </button>
          )
        })}
      </div>
    </section>
  )
}
