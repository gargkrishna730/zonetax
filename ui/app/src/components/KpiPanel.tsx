import type { CostsResponse } from '../types'
import { fmtAgo, fmtDuration, fmtExact, fmtGB, fmtUSD } from '../format'

export interface KpiPanelProps {
  costs: CostsResponse | null
  fetchError: string | null
}

/** Top KPI row. Every value now states its measurement period explicitly instead of a bare
 * number: "tracked" totals are labeled as a SESSION metric (since this collector process last
 * completed a cycle, i.e. since it or the agents last restarted) — deliberately not framed as a
 * calendar day, since it isn't one. The exact collector-start timestamp and current exact time
 * are both shown (not just relative "ago" text) so a viewer can do their own reconciliation
 * against AWS billing or anything else without guessing what "recently" means. */
export function KpiPanel({ costs, fetchError }: KpiPanelProps) {
  const totals = costs?.totals
  const startedAt = costs?.collector_started_at_utc
  const exactStart = fmtExact(startedAt)
  const exactServerTime = fmtExact(costs?.server_time_utc)
  const sessionSeconds = startedAt ? (Date.now() - new Date(startedAt).getTime()) / 1000 : null

  return (
    <div className="card kpis">
      <div className="kpi">
        <div className="kpi-head">
          <span className="label">Cross-AZ spend</span>
          <span className="period-chip" title="Measured since this collector session began — see the session panel below for exact start time.">
            this session
          </span>
        </div>
        <div className="value cost">{fmtUSD(totals?.cross_az_cost_usd ?? 0)}</div>
        <div className="sub">
          {sessionSeconds !== null ? (
            <>tracked over {fmtDuration(sessionSeconds)} of collection, not a calendar day</>
          ) : (
            'no completed collection cycle yet'
          )}
        </div>
      </div>
      <div className="kpi">
        <div className="kpi-head">
          <span className="label">Cross-AZ traffic</span>
          <span className="period-chip">this session</span>
        </div>
        <div className="value">{fmtGB(totals?.cross_az_gb ?? 0)}</div>
        <div className="sub">
          {totals
            ? ((100 * totals.cross_az_gb) / Math.max(1e-9, totals.cross_az_gb + totals.same_az_gb)).toFixed(1)
            : '0'}
          % of all tracked traffic is cross-AZ (billed)
        </div>
      </div>
      <div className="kpi">
        <div className="kpi-head">
          <span className="label">Same-AZ traffic</span>
          <span className="period-chip">this session</span>
        </div>
        <div className="value">{fmtGB(totals?.same_az_gb ?? 0)}</div>
        <div className="sub">free — same availability zone, no data-transfer charge</div>
      </div>
      <div className="kpi">
        <div className="kpi-head">
          <span className="label">Price / GB</span>
        </div>
        <div className="value">{fmtUSD(totals?.price_per_gb_usd ?? 0)}/GB</div>
        <div className="sub">
          {totals?.price_per_gb_direction_usd
            ? `$${totals.price_per_gb_direction_usd.toFixed(3)}/GB each direction, billed on both send + receive`
            : '—'}
        </div>
      </div>

      <div className="kpi kpi-wide session-panel">
        <div className="kpi-head">
          <span className="label">Data freshness &amp; collection window</span>
        </div>
        <div className="session-grid">
          <div>
            <span className="session-k">Last collected</span>
            <span className="session-v">
              {fmtAgo(costs?.updated_at)}
              {fmtExact(costs?.updated_at) && <span className="session-exact"> · {fmtExact(costs?.updated_at)}</span>}
            </span>
          </div>
          <div>
            <span className="session-k">Server time now</span>
            <span className="session-v">{exactServerTime ?? '—'}</span>
          </div>
          <div>
            <span className="session-k">Collector session started</span>
            <span className="session-v">{exactStart ?? 'no completed cycle yet'}</span>
          </div>
          <div>
            <span className="session-k">Collection interval</span>
            <span className="session-v">
              {costs?.scrape_interval_seconds ? `every ${fmtDuration(costs.scrape_interval_seconds)}` : '—'}
            </span>
          </div>
        </div>
        {(costs?.stale_error || fetchError) && (
          <div className="session-warning">
            <span className="dot dot-bad" />
            {costs?.stale_error ? `Last collection cycle failed: ${costs.stale_error} — showing the last known-good data.` : `Dashboard fetch failed: ${fetchError}`}
          </div>
        )}
      </div>
    </div>
  )
}
