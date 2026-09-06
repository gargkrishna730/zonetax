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

/** Route details panel shown on selecting an edge/route: every field the brief requires
 * (source/destination workload+namespace+AZ, traffic, cross-AZ traffic, price/GB, estimated
 * cost, % of total spend, time window, collection timestamp) — reusing the existing
 * price_per_gb_usd from the API response rather than recomputing a rate in the UI. */
export function RouteDetailsPanel({ entry, totalCrossAZCostUSD, pricePerGBUSD, rangeStartUTC, rangeEndUTC, serverTimeUTC, onClose }: RouteDetailsPanelProps) {
  const pctOfTotal = totalCrossAZCostUSD > 0 ? (100 * entry.cost_usd) / totalCrossAZCostUSD : null
  return (
    <div className="route-details-panel" role="dialog" aria-label="Route details">
      <div className="route-details-head">
        <h3>
          {entry.src_zone} → {entry.dst_zone}
        </h3>
        <button type="button" onClick={onClose} aria-label="Close route details">
          ×
        </button>
      </div>
      <div className="route-details-grid">
        <div>
          <span className="rd-k">Source workload</span>
          <span className="rd-v">
            {entry.src_workload} <span className="rd-ns">({entry.src_namespace})</span>
          </span>
        </div>
        <div>
          <span className="rd-k">Destination workload</span>
          <span className="rd-v">
            {entry.dst_workload} <span className="rd-ns">({entry.dst_namespace})</span>
          </span>
        </div>
        <div>
          <span className="rd-k">Source AZ</span>
          <span className="rd-v">{entry.src_zone}</span>
        </div>
        <div>
          <span className="rd-k">Destination AZ</span>
          <span className="rd-v">{entry.dst_zone}</span>
        </div>
        <div>
          <span className="rd-k">Traffic</span>
          <span className="rd-v">{fmtGB(entry.gb)}</span>
        </div>
        <div>
          <span className="rd-k">Cross-AZ traffic</span>
          <span className="rd-v">{fmtGB(entry.gb)} (100% — this route is by definition cross-AZ)</span>
        </div>
        <div>
          <span className="rd-k">Price / GB</span>
          <span className="rd-v">{fmtUSD(pricePerGBUSD)}/GB</span>
        </div>
        <div>
          <span className="rd-k">Estimated route cost</span>
          <span className="rd-v cost">{fmtUSD(entry.cost_usd)}</span>
        </div>
        <div>
          <span className="rd-k">% of total cross-AZ spend</span>
          <span className="rd-v">{pctOfTotal !== null ? pctOfTotal.toFixed(1) + '%' : '—'}</span>
        </div>
        <div className="rd-wide">
          <span className="rd-k">Time window</span>
          <span className="rd-v">
            {fmtExact(rangeStartUTC)} → {fmtExact(rangeEndUTC)}
          </span>
        </div>
        <div className="rd-wide">
          <span className="rd-k">Collection timestamp</span>
          <span className="rd-v">{fmtExact(serverTimeUTC)}</span>
        </div>
      </div>
    </div>
  )
}
