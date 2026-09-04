import { useMemo, useState } from 'react'
import type { HistoryBucket, HistoryRange, HistoryResponse } from '../types'
import { foldHourlyToDaily } from '../bucketFolding'
import { fmtBucketLabel, fmtGB, fmtUSD } from '../format'

export interface CostHistoryChartProps {
  range: HistoryRange
  onRangeChange: (r: HistoryRange) => void
  history: HistoryResponse | null
  loading: boolean
  error: string | null
}

const RANGE_OPTIONS: { value: HistoryRange; label: string }[] = [
  { value: '1h', label: 'Last 1 hour' },
  { value: '6h', label: 'Last 6 hours' },
  { value: '24h', label: 'Last 24 hours' },
  { value: '7d', label: 'Last 7 days' },
]

/** Daily/hourly cross-AZ cost bar chart. Granularity auto-follows the selected range (hourly
 * bars for <=24h ranges, daily bars for 7d) since a 7-day view with 168 hourly bars is
 * unreadable, and a 1-hour view folded to daily would just be one bar. Every bar's exact
 * value/window is available via hover; incomplete/no-data buckets are visually distinct rather
 * than looking identical to a real zero. */
export function CostHistoryChart({ range, onRangeChange, history, loading, error }: CostHistoryChartProps) {
  const [hovered, setHovered] = useState<HistoryBucket | null>(null)

  const granularity: 'hour' | 'day' = range === '7d' ? 'day' : 'hour'
  const buckets = useMemo(() => {
    if (!history) return []
    return granularity === 'day' ? foldHourlyToDaily(history.buckets) : history.buckets
  }, [history, granularity])

  const maxCost = Math.max(1e-9, ...buckets.map((b) => b.cross_az_cost_usd))

  return (
    <div className="card history-card">
      <div className="history-header">
        <div className="history-title">Cross-AZ spend over time</div>
        <div className="range-picker" role="tablist" aria-label="Time range">
          {RANGE_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              type="button"
              role="tab"
              aria-selected={range === opt.value}
              className={range === opt.value ? 'active' : ''}
              onClick={() => onRangeChange(opt.value)}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </div>

      {history && (
        <div className="history-window-label">
          Window: {new Date(history.range_start_utc).toLocaleString()} &rarr;{' '}
          {new Date(history.range_end_utc).toLocaleString()}
          {history.history_start_utc && new Date(history.history_start_utc) > new Date(history.range_start_utc) && (
            <span className="history-note">
              {' '}
              &middot; collector history only goes back to {new Date(history.history_start_utc).toLocaleString()}
            </span>
          )}
        </div>
      )}

      {loading && !history && <div className="history-empty">Loading history…</div>}
      {error && (
        <div className="history-empty history-error">Couldn't load history: {error}</div>
      )}
      {!loading && !error && history && buckets.length === 0 && (
        <div className="history-empty">
          No history yet for this range — the collector hasn't completed a collection cycle in this
          window.
        </div>
      )}

      {buckets.length > 0 && (
        <div className="history-chart" role="img" aria-label="Cross-AZ spend by time bucket">
          {buckets.map((b, i) => {
            const heightPct = b.has_data ? Math.max(2, (b.cross_az_cost_usd / maxCost) * 100) : 0
            return (
              <div
                key={b.start_utc + i}
                className={`history-bar-col ${!b.has_data ? 'no-data' : !b.complete ? 'partial' : ''}`}
                onMouseEnter={() => setHovered(b)}
                onMouseLeave={() => setHovered((h) => (h === b ? null : h))}
              >
                <div className="history-bar-track">
                  {b.has_data && <div className="history-bar" style={{ height: `${heightPct}%` }} />}
                </div>
                <div className="history-bar-label">{fmtBucketLabel(b.start_utc, granularity)}</div>
              </div>
            )
          })}
        </div>
      )}

      {hovered && (
        <div className="history-tooltip">
          <div className="tt-title">
            {new Date(hovered.start_utc).toLocaleString()} &rarr; {new Date(hovered.end_utc).toLocaleTimeString()}
          </div>
          {hovered.has_data ? (
            <>
              <div className="tt-row">
                <span>Cross-AZ spend</span>
                <b>{fmtUSD(hovered.cross_az_cost_usd)}</b>
              </div>
              <div className="tt-row">
                <span>Cross-AZ traffic</span>
                <b>{fmtGB(hovered.cross_az_gb)}</b>
              </div>
              <div className="tt-row">
                <span>Same-AZ traffic</span>
                <b>{fmtGB(hovered.same_az_gb)}</b>
              </div>
              {!hovered.complete && (
                <div className="tt-row tt-warn">Partial window — still in progress or history started mid-bucket</div>
              )}
            </>
          ) : (
            <div className="tt-row tt-warn">No data — before collector history began</div>
          )}
        </div>
      )}

      <div className="history-legend">
        <span>
          <i className="swatch swatch-full" /> complete
        </span>
        <span>
          <i className="swatch swatch-partial" /> partial / in progress
        </span>
        <span>
          <i className="swatch swatch-none" /> no data
        </span>
      </div>
    </div>
  )
}
