import type { MapSummary } from '../mapSummary'
import type { MapRange } from '../types'
import { fmtGB, fmtUSD } from '../format'
import { RANGE_LABEL } from '../labels'

export interface SummaryStripProps {
  summary: MapSummary
  range: MapRange
  partial: boolean
  observedHours: number | null
  crossAZTrafficPercent: number | null
}

const RANGE_DAYS: Partial<Record<MapRange, number>> = { '1h': 1 / 24, '6h': 0.25, '24h': 1, '7d': 7 }

/** Four headline KPIs. Each states its window; partial windows are flagged in words, and the
 * daily run-rate is only shown when it can be computed honestly from observed time. */
export function SummaryStrip({ summary, range, partial, observedHours, crossAZTrafficPercent }: SummaryStripProps) {
  const label = RANGE_LABEL[range]
  const days = RANGE_DAYS[range]
  const hours = partial ? observedHours : days ? days * 24 : null
  const perDay = hours && hours >= 1 ? (summary.totalCostUSD / hours) * 24 : null
  const top = summary.highestCostRoute
  const topShare = top && summary.totalCostUSD > 0 ? (100 * top.cost_usd) / summary.totalCostUSD : null

  return (
    <section className="kpi-row" aria-label="Summary">
      <div className="card kpi">
        <span className="kpi-label">Cross-AZ spend</span>
        <span className="kpi-value">{fmtUSD(summary.totalCostUSD)}</span>
        <span className="kpi-sub">
          {label}
          {partial && <span className="partial-tag"> · partial window</span>}
        </span>
      </div>
      <div className="card kpi">
        <span className="kpi-label">Projected per day</span>
        <span className="kpi-value">{perDay !== null ? fmtUSD(perDay) : '—'}</span>
        <span className="kpi-sub">
          {perDay !== null
            ? `${fmtUSD(perDay * 30)} / month at this rate${partial && hours ? `, from ${hours.toFixed(1)} h observed` : ''}`
            : 'needs at least 1 hour of observed data'}
        </span>
      </div>
      <div className="card kpi">
        <span className="kpi-label">Cross-AZ traffic</span>
        <span className="kpi-value">{fmtGB(summary.totalGB)}</span>
        <span className="kpi-sub">
          {summary.routeCount} zone route{summary.routeCount === 1 ? '' : 's'} · {summary.affectedWorkloadCount} workloads
          {crossAZTrafficPercent !== null && ` · ${crossAZTrafficPercent.toFixed(0)}% of all traffic`}
        </span>
      </div>
      <div className="card kpi">
        <span className="kpi-label">Top cost driver</span>
        <span className="kpi-value small" title={top ? `${top.src_namespace}/${top.src_workload} → ${top.dst_namespace}/${top.dst_workload}` : undefined}>
          {top ? `${top.src_workload} → ${top.dst_workload}` : '—'}
        </span>
        <span className="kpi-sub">
          {top ? `${fmtUSD(top.cost_usd)}${topShare !== null ? ` · ${topShare.toFixed(0)}% of spend` : ''} · ${top.src_zone} → ${top.dst_zone}` : 'no cross-AZ traffic'}
        </span>
      </div>
    </section>
  )
}
