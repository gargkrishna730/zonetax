import { useMemo, useState, type KeyboardEvent } from 'react'
import type { HistoryBucket, HistoryRange, HistoryResponse } from '../types'
import { foldHourlyToDaily } from '../bucketFolding'
import { fmtBucketLabel, fmtGB, fmtUSD } from '../format'
import { Segmented } from './Segmented'

export interface CostHistoryChartProps {
  range: HistoryRange
  onRangeChange: (r: HistoryRange) => void
  history: HistoryResponse | null
  loading: boolean
  error: string | null
}

const RANGE_OPTIONS: { value: HistoryRange; label: string; title: string }[] = [
  { value: '6h', label: '6h', title: 'Last 6 hours, hourly' },
  { value: '24h', label: '24h', title: 'Last 24 hours, hourly' },
  { value: '7d', label: '7d', title: 'Last 7 days, daily' },
]

function bucketState(b: HistoryBucket): 'full' | 'partial' | 'nodata' {
  return !b.has_data ? 'nodata' : b.complete ? 'full' : 'partial'
}
const STATE_TEXT = { full: 'complete', partial: 'partial (collector not running for the whole period)', nodata: 'no data (collector was not running)' }

/** Spend over time. Bars are real buttons: Tab into the chart, arrow keys move between buckets,
 * and the detail row below always shows the selected bucket (no hover-only information). A
 * visually hidden table carries the same data for screen readers. */
export function CostHistoryChart({ range, onRangeChange, history, loading, error }: CostHistoryChartProps) {
  const granularity: 'hour' | 'day' = range === '7d' ? 'day' : 'hour'
  const buckets = useMemo(() => (history ? (granularity === 'day' ? foldHourlyToDaily(history.buckets) : history.buckets) : []), [history, granularity])
  const [sel, setSel] = useState<number | null>(null)
  const active = sel !== null && sel < buckets.length ? sel : buckets.length - 1
  const maxCost = Math.max(1e-9, ...buckets.map((b) => (b.has_data ? b.cross_az_cost_usd : 0)))
  const total = buckets.reduce((s, b) => s + (b.has_data ? b.cross_az_cost_usd : 0), 0)
  const gaps = buckets.filter((b) => !b.complete).length
  const cur = buckets[active]

  function onKey(e: KeyboardEvent, i: number) {
    let n = -1
    if (e.key === 'ArrowRight') n = Math.min(buckets.length - 1, i + 1)
    else if (e.key === 'ArrowLeft') n = Math.max(0, i - 1)
    else if (e.key === 'Home') n = 0
    else if (e.key === 'End') n = buckets.length - 1
    if (n >= 0) {
      e.preventDefault()
      setSel(n)
      ;(e.currentTarget.parentElement?.children[n] as HTMLElement | undefined)?.focus()
    }
  }

  const label = (b: HistoryBucket) => (granularity === 'day' ? fmtBucketLabel(b.start_utc, 'day') : new Date(b.start_utc).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric' }))

  return (
    <section className="card" aria-labelledby="spend-title">
      <div className="card-head">
        <h2 id="spend-title">Spend over time</h2>
        <span className="card-sub">
          {buckets.length > 0 && `${fmtUSD(total)} total`}
          {gaps > 0 && ` · ${gaps} ${granularity === 'day' ? 'day' : 'hour'}${gaps === 1 ? '' : 's'} incomplete`}
        </span>
        <span className="spacer" />
        <Segmented label="Chart range" options={RANGE_OPTIONS} value={range} onChange={(r) => { setSel(null); onRangeChange(r) }} />
      </div>
      <div className="card-body">
        {loading && !history ? (
          <div className="empty-state" style={{ height: 150 }}>Loading history</div>
        ) : error ? (
          <div className="notice notice-error" role="alert"><strong>Could not load history.</strong> {error}</div>
        ) : buckets.length === 0 ? (
          <div className="empty-state" style={{ height: 150 }}>No history yet. The collector records a snapshot every scrape.</div>
        ) : (
          <>
            <div className="chart">
              <div className="chart-axis" aria-hidden="true">
                <span>{fmtUSD(maxCost)}</span>
                <span>{fmtUSD(maxCost / 2)}</span>
                <span>$0</span>
              </div>
              <div className="chart-plot" role="group" aria-label={`Cross-AZ spend per ${granularity}. Use arrow keys to move between bars.`}>
                {buckets.map((b, i) => {
                  const st = bucketState(b)
                  const h = st === 'nodata' ? 0 : Math.max(1.5, (b.cross_az_cost_usd / maxCost) * 100)
                  return (
                    <button
                      key={b.start_utc}
                      type="button"
                      className={`bar-btn ${st}`}
                      aria-pressed={i === active}
                      tabIndex={i === active ? 0 : -1}
                      aria-label={`${label(b)}: ${st === 'nodata' ? 'no data' : fmtUSD(b.cross_az_cost_usd)}, ${STATE_TEXT[st]}`}
                      onClick={() => setSel(i)}
                      onMouseEnter={() => setSel(i)}
                      onFocus={() => setSel(i)}
                      onKeyDown={(e) => onKey(e, i)}
                    >
                      <span className="bar" style={{ height: `${h}%` }} />
                    </button>
                  )
                })}
              </div>
              <div className="chart-x" aria-hidden="true">
                <span>{label(buckets[0])}</span>
                {buckets.length > 2 && <span>{label(buckets[Math.floor(buckets.length / 2)])}</span>}
                <span>{label(buckets[buckets.length - 1])}</span>
              </div>
            </div>
            {cur && (
              <div className="chart-detail" aria-live="polite">
                <span><span className="k">{granularity === 'day' ? 'Day' : 'Hour'}</span><b>{label(cur)}</b></span>
                {cur.has_data ? (
                  <>
                    <span><span className="k">Spend</span><b className="num">{fmtUSD(cur.cross_az_cost_usd)}</b></span>
                    <span><span className="k">Cross-AZ</span><b className="num">{fmtGB(cur.cross_az_gb)}</b></span>
                    <span><span className="k">Same-AZ (free)</span><b className="num">{fmtGB(cur.same_az_gb)}</b></span>
                  </>
                ) : null}
                <span className={bucketState(cur) === 'full' ? 'muted' : ''} style={bucketState(cur) !== 'full' ? { color: 'var(--warning)', fontWeight: 600 } : undefined}>
                  {STATE_TEXT[bucketState(cur)]}
                </span>
              </div>
            )}
            <div className="chart-legend" aria-hidden="true">
              <span><i className="sw sw-full" />Complete</span>
              <span><i className="sw sw-partial" />Partial</span>
              <span><i className="sw sw-none" />No data</span>
              {history?.history_start_utc && new Date(history.history_start_utc) > new Date(history.range_start_utc) && (
                <span>History starts {new Date(history.history_start_utc).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}</span>
              )}
            </div>
            <div className="sr-only"><table>
              <caption>Cross-AZ spend per {granularity}</caption>
              <thead><tr><th>{granularity}</th><th>Spend</th><th>Traffic</th><th>Status</th></tr></thead>
              <tbody>
                {buckets.map((b) => (
                  <tr key={b.start_utc}><td>{label(b)}</td><td>{b.has_data ? fmtUSD(b.cross_az_cost_usd) : '-'}</td><td>{b.has_data ? fmtGB(b.cross_az_gb) : '-'}</td><td>{STATE_TEXT[bucketState(b)]}</td></tr>
                ))}
              </tbody>
            </table></div>
          </>
        )}
      </div>
    </section>
  )
}
