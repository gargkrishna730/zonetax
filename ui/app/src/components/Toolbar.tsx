import type { MapRange, MapResponse } from '../types'
import type { Theme } from '../theme'
import { fmtAgo, fmtExact } from '../format'
import { Segmented } from './Segmented'
import { dataStatusOf, type DataStatus } from '../labels'

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
  theme: Theme
  onThemeChange: (t: Theme) => void
  filtersOpen: boolean
  onToggleFilters: () => void
  activeFilterCount: number
}

const RANGE_OPTIONS: { value: MapRange; label: string; title: string }[] = [
  { value: '15m', label: '15m', title: 'Last 15 minutes' },
  { value: '1h', label: '1h', title: 'Last 1 hour' },
  { value: '6h', label: '6h', title: 'Last 6 hours' },
  { value: '24h', label: '24h', title: 'Last 24 hours' },
  { value: '7d', label: '7d', title: 'Last 7 days' },
  { value: 'custom', label: 'Custom', title: 'Custom date range' },
]

const STATUS_LABEL: Record<DataStatus, string> = {
  loading: 'Loading',
  error: 'API error',
  stale: 'No data in window',
  incomplete: 'Partial window',
  measured: 'Live',
}
const STATUS_TITLE: Record<DataStatus, string> = {
  loading: 'Fetching data from the collector',
  error: 'The collector API returned an error',
  stale: 'The collector has no data for this time window yet',
  incomplete: 'The collector did not observe the whole selected window (it started later, or was down for part of it). Numbers are real but cover less than the full period.',
  measured: 'The whole selected window was observed',
}

/** Sticky top bar (brand, cluster identity, live status, refresh, theme) plus the time bar
 * (range selector, custom range, exact window). */
export function Toolbar(p: ToolbarProps) {
  const status = dataStatusOf(p.data, p.loading, p.error)
  return (
    <>
      <header className="topbar">
        <button
          type="button"
          className="btn btn-ghost btn-icon"
          onClick={p.onToggleFilters}
          aria-expanded={p.filtersOpen}
          aria-controls="filters"
          title={p.filtersOpen ? 'Hide filters' : 'Show filters'}
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true"><path d="M3 5h18M6 12h12M10 19h4" /></svg>
          <span>Filters{p.activeFilterCount > 0 ? ` (${p.activeFilterCount})` : ''}</span>
        </button>
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">Z</span>
          <div>
            <h1>ZoneTax</h1>
            <div className="brand-sub">Cross-AZ network cost</div>
          </div>
        </div>
        <span className="chip" title="Cloud provider">{p.cloud || '—'}</span>
        <span className="chip" title="Region">{p.region || '—'}</span>
        <span className={`status status-${status}`} title={STATUS_TITLE[status]} role="status" aria-live="polite">
          <span className="dot" aria-hidden="true" />
          {STATUS_LABEL[status]}
        </span>
        <span className="topbar-spacer" />
        <label className="switch">
          <input type="checkbox" checked={p.autoRefresh} onChange={(e) => p.onAutoRefreshChange(e.target.checked)} />
          Auto-refresh
        </label>
        <button type="button" className="btn" onClick={p.onRefreshNow} disabled={p.loading}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" aria-hidden="true"><path d="M21 12a9 9 0 1 1-3-6.7L21 8" /><path d="M21 3v5h-5" /></svg>
          {p.loading ? 'Refreshing' : 'Refresh'}
        </button>
        <button
          type="button"
          className="btn btn-icon"
          onClick={() => p.onThemeChange(p.theme === 'dark' ? 'light' : 'dark')}
          aria-label={p.theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}
          title={p.theme === 'dark' ? 'Light theme' : 'Dark theme'}
        >
          {p.theme === 'dark' ? (
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true"><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></svg>
          ) : (
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" /></svg>
          )}
        </button>
      </header>

      <nav className="timebar" aria-label="Time range">
        <Segmented label="Time range" options={RANGE_OPTIONS} value={p.range} onChange={p.onRangeChange} />
        {p.range === 'custom' && (
          <div className="custom-range">
            <label className="field">
              From
              <input
                className="input"
                type="datetime-local"
                value={p.customSince ? toLocalInputValue(p.customSince) : ''}
                onChange={(e) => p.onCustomRangeChange(e.target.value ? fromLocalInputValue(e.target.value) : null, p.customUntil)}
              />
            </label>
            <label className="field">
              To
              <input
                className="input"
                type="datetime-local"
                value={p.customUntil ? toLocalInputValue(p.customUntil) : ''}
                onChange={(e) => p.onCustomRangeChange(p.customSince, e.target.value ? fromLocalInputValue(e.target.value) : null)}
              />
            </label>
          </div>
        )}
        <div className="window-label">
          {p.data ? (
            <>
              <strong className="num">
                {fmtExact(p.data.range_start_utc)} – {fmtExact(p.data.range_end_utc)}
              </strong>
              <br />
              updated {fmtAgo(p.data.server_time_utc)}
            </>
          ) : (
            'No data yet'
          )}
        </div>
      </nav>
    </>
  )
}

// datetime-local wants local "YYYY-MM-DDTHH:mm"; state is stored as RFC3339 UTC for the API.
function toLocalInputValue(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
function fromLocalInputValue(local: string): string {
  return new Date(local).toISOString()
}
