import type { MapSummary } from '../mapSummary'
import type { MapRange } from '../types'
import { fmtGB, fmtUSD, fmtUSDShort } from '../format'

export interface SummaryStripProps {
  summary: MapSummary
  range: MapRange
  crossAZTrafficPercent: number | null // null when unavailable (no same-AZ baseline in this window)
  pricePerGBUSD: number
  pricePerGBDirectionUSD: number
  windowLabel: string
}

const RANGE_LABEL: Record<MapRange, string> = {
  '15m': 'last 15 minutes',
  '1h': 'last 1 hour',
  '6h': 'last 6 hours',
  '24h': 'last 24 hours',
  '7d': 'last 7 days',
  custom: 'custom range',
}

/** The ZoneTax summary strip: every KPI states its unit and time window explicitly, and the
 * billing-model explanation is always visible (not a hidden tooltip) since it directly explains
 * why traffic*price != cost naively — cross-AZ transfer is billed on BOTH the egress leg and the
 * ingress leg (see internal/costengine's crossAZBillingMultiplier), a genuinely easy thing to
 * get wrong when eyeballing the numbers. */
export function SummaryStrip({ summary, range, crossAZTrafficPercent, pricePerGBUSD, pricePerGBDirectionUSD, windowLabel }: SummaryStripProps) {
  const rangeLabel = RANGE_LABEL[range]
  return (
    <div className="summary-strip">
      <div className="summary-kpi">
        <span className="summary-label">Total cross-AZ spend</span>
        <span className="summary-value cost">{fmtUSD(summary.totalCostUSD)}</span>
        <span className="summary-sub">{rangeLabel}</span>
      </div>
      <div className="summary-kpi">
        <span className="summary-label">Total cross-AZ traffic</span>
        <span className="summary-value">{fmtGB(summary.totalGB)}</span>
        <span className="summary-sub">{rangeLabel}</span>
      </div>
      <div className="summary-kpi">
        <span className="summary-label">Cross-AZ routes</span>
        <span className="summary-value">{summary.routeCount}</span>
        <span className="summary-sub">distinct zone-to-zone paths</span>
      </div>
      <div className="summary-kpi">
        <span className="summary-label">Affected workloads</span>
        <span className="summary-value">{summary.affectedWorkloadCount}</span>
        <span className="summary-sub">sending or receiving cross-AZ traffic</span>
      </div>
      <div className="summary-kpi">
        <span className="summary-label">Highest-cost route</span>
        <span className="summary-value">
          {summary.highestCostRoute ? (
            <>
              {summary.highestCostRoute.src_zone} → {summary.highestCostRoute.dst_zone}
              <span className="summary-value-secondary"> · {fmtUSDShort(summary.highestCostRoute.cost_usd)}</span>
            </>
          ) : (
            '—'
          )}
        </span>
        <span className="summary-sub">{rangeLabel}</span>
      </div>
      <div className="summary-kpi">
        <span className="summary-label">Avg cost / GB</span>
        <span className="summary-value">{summary.avgCostPerGB !== null ? fmtUSD(summary.avgCostPerGB) : '—'}</span>
        <span className="summary-sub">effective rate, this window</span>
      </div>
      {crossAZTrafficPercent !== null && (
        <div className="summary-kpi">
          <span className="summary-label">Cross-AZ traffic %</span>
          <span className="summary-value">{crossAZTrafficPercent.toFixed(1)}%</span>
          <span className="summary-sub">of all tracked traffic (vs. same-AZ)</span>
        </div>
      )}

      <div className="billing-explainer">
        <b>How ZoneTax bills:</b> AWS charges for cross-AZ transfer on BOTH directions of a
        connection — ${pricePerGBDirectionUSD.toFixed(3)}/GB as egress from the sender's AZ, plus
        another ${pricePerGBDirectionUSD.toFixed(3)}/GB as ingress to the receiver's AZ — for a
        real effective rate of <b>{fmtUSD(pricePerGBUSD)}/GB</b> on every GB ZoneTax observes.
        Same-AZ traffic is free and not billed. {windowLabel}
      </div>
    </div>
  )
}
