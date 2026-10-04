import { useEffect, useRef } from 'react'
import type { MapEntry } from '../types'
import { fmtExact, fmtGB, fmtUSD } from '../format'

export interface RouteDetailsPanelProps {
  entry: MapEntry
  totalCrossAZCostUSD: number
  pricePerGBUSD: number
  rangeStartUTC: string
  rangeEndUTC: string
  serverTimeUTC: string
  onClose: () => void
}

/** Details for one workload-to-workload route. Uses the API's price, never recomputes billing. */
export function RouteDetailsPanel({ entry, totalCrossAZCostUSD, pricePerGBUSD, rangeStartUTC, rangeEndUTC, serverTimeUTC, onClose }: RouteDetailsPanelProps) {
  const pct = totalCrossAZCostUSD > 0 ? (100 * entry.cost_usd) / totalCrossAZCostUSD : null
  const closeRef = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    closeRef.current?.focus()
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <section className="panel" role="dialog" aria-modal="false" aria-labelledby="rd-title" style={{ bottom: 'auto', maxHeight: 'calc(100% - 24px)' }}>
      <div className="panel-head">
        <div>
          <h3 id="rd-title">{entry.src_workload} → {entry.dst_workload}</h3>
          <div className="panel-sub">{entry.src_zone} → {entry.dst_zone}</div>
        </div>
        <button ref={closeRef} type="button" className="btn btn-ghost btn-icon close" onClick={onClose} aria-label="Close route details">✕</button>
      </div>
      <dl className="kv">
        <div><dt>Cost</dt><dd className="num">{fmtUSD(entry.cost_usd)}</dd></div>
        <div><dt>Share of spend</dt><dd className="num">{pct !== null ? pct.toFixed(1) + '%' : '—'}</dd></div>
        <div><dt>Traffic</dt><dd className="num">{fmtGB(entry.gb)}</dd></div>
        <div><dt>Price</dt><dd className="num">{fmtUSD(pricePerGBUSD)}/GB <small>(both directions)</small></dd></div>
        <div><dt>Source</dt><dd>{entry.src_workload}<br /><small>{entry.src_namespace} · {entry.src_zone}</small></dd></div>
        <div><dt>Destination</dt><dd>{entry.dst_workload}<br /><small>{entry.dst_namespace} · {entry.dst_zone}</small></dd></div>
        <div className="wide"><dt>Window</dt><dd>{fmtExact(rangeStartUTC)} – {fmtExact(rangeEndUTC)}</dd></div>
        <div className="wide"><dt>Collected</dt><dd>{fmtExact(serverTimeUTC)}</dd></div>
      </dl>
    </section>
  )
}
