import type { MapRange, MapResponse } from '../types'
import { fmtAgo, fmtExact } from '../format'

export interface ToolbarProps {
  cloud?: string
  region?: string
  range: MapRange
  onRangeChange: (r: MapRange) => void
  customSince: string | null
  customUntil: string | null
  onCustomRangeChange: (since: string | null, until: string | null) => void
  data: MapResponse | null
  loading: boolean
  error: string | null
  autoRefresh: boolean
  onAutoRefreshChange: (v: boolean) => void
  onRefreshNow: () => void
}

const RANGE_OPTIONS: { value: MapRange; label: string }[] = [
  { value: '15m', label: 'Last 15 minutes' },
  { value: '1h', label: 'Last 1 hour' },
  { value: '6h', label: 'Last 6 hours' },
  { value: '24h', label: 'Last 24 hours' },
  { value: '7d', label: 'Last 7 days' },
  { value: 'custom', label: 'Custom range' },
]

/** Top toolbar: page title, cluster/cloud/region identity, data status, time-range picker
 * (including a custom RFC3339 date/time range), refresh controls, and exact freshness
 * timestamps. Every value here is either a real API field or an honest empty-state — cluster
 * name is intentionally NOT shown (ZoneTax's backend has no cluster-identity concept today;
 * showing one would mean fabricating it). */
export function Toolbar({
  cloud,
  region,
  range,
  onRangeChange,
  customSince,
  customUntil,
  onCustomRangeChange,
  data,
  loading,
  error,
  autoRefresh,
  onAutoRefreshChange,
  onRefreshNow,
}: ToolbarProps) {
  const status: 'loading' | 'error' | 'stale' | 'incomplete' | 'measured' = error
    ? 'error'
    : loading && !data
      ? 'loading'
      : data && !data.has_data
        ? 'stale'
        : data && !data.complete
          ? 'incomplete'
          : 'measured'

  const statusLabel: Record<typeof status, string> = {
    loading: 'Loading…',
    error: 'API error',
    stale: 'No data in this window',
    incomplete: 'Incomplete window (partial data)',
    measured: 'Measured',
  }

  return (
    <div className="map-toolbar">
      <div className="map-toolbar-row map-toolbar-top">
        <h1 className="map-title">Cross-AZ Service Map</h1>
        <div className="map-identity">
          <span className="identity-chip">{cloud || '—'}</span>
          <span className="identity-chip">{region || '—'}</span>
          <span className={`identity-status status-${status}`}>
            <span className="dot" />
            {statusLabel[status]}
          </span>
        </div>
        <div className="map-toolbar-actions">
          <label className="auto-refresh-toggle">
            <input type="checkbox" checked={autoRefresh} onChange={(e) => onAutoRefreshChange(e.target.checked)} />
            Auto-refresh
          </label>
          <button type="button" className="refresh-btn" onClick={onRefreshNow} disabled={loading}>
            {loading ? 'Refreshing…' : '↻ Refresh'}
          </button>
        </div>
      </div>

      <div className="map-toolbar-row map-toolbar-range">
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

        {range === 'custom' && (
          <div className="custom-range-inputs">
            <label>
              Since
              <input
                type="datetime-local"
                value={customSince ? toLocalInputValue(customSince) : ''}
                onChange={(e) => onCustomRangeChange(e.target.value ? fromLocalInputValue(e.target.value) : null, customUntil)}
              />
            </label>
            <label>
              Until
              <input
                type="datetime-local"
                value={customUntil ? toLocalInputValue(customUntil) : ''}
                onChange={(e) => onCustomRangeChange(customSince, e.target.value ? fromLocalInputValue(e.target.value) : null)}
              />
            </label>
          </div>
        )}

        <div className="range-window-label">
          {data ? (
            <>
              <span className="window-exact">
                {fmtExact(data.range_start_utc)} → {fmtExact(data.range_end_utc)}
              </span>
              <span className="window-ago">last collected {fmtAgo(data.server_time_utc)}</span>
            </>
          ) : (
            <span className="window-ago">no data yet</span>
          )}
        </div>
      </div>
    </div>
  )
}

// datetime-local inputs want "YYYY-MM-DDTHH:mm" in LOCAL time with no timezone suffix; we store
// state as full RFC3339 UTC (what the API needs) — these two converters bridge that gap without
// losing precision or silently assuming UTC-as-local.
function toLocalInputValue(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
function fromLocalInputValue(local: string): string {
  return new Date(local).toISOString()
}
