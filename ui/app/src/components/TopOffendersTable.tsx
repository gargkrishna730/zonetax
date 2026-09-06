import type { MapEntry, MapRange } from '../types'
import { fmtGB, fmtUSD } from '../format'

export interface TopOffendersTableProps {
  entries: MapEntry[] // pre-sorted by cost descending, pre-limited by the caller
  totalCrossAZCostUSD: number
  range: MapRange
  onSelectRoute: (entry: MapEntry) => void
  collapsed: boolean
  onToggleCollapsed: () => void
}

const RANGE_LABEL: Record<MapRange, string> = {
  '15m': 'last 15 minutes',
  '1h': 'last 1 hour',
  '6h': 'last 6 hours',
  '24h': 'last 24 hours',
  '7d': 'last 7 days',
  custom: 'custom range',
}

/** Top offenders: rank, source, destination, AZ path, namespace, cross-AZ traffic, cost, % of
 * total spend, and the active time window — clicking a row focuses that exact route on the map
 * (via onSelectRoute) rather than only being a read-only table. */
export function TopOffendersTable({ entries, totalCrossAZCostUSD, range, onSelectRoute, collapsed, onToggleCollapsed }: TopOffendersTableProps) {
  return (
    <div className={`card offenders-card${collapsed ? ' collapsed' : ''}`}>
      <div className="card-head">
        <h2>Top offenders</h2>
        <button type="button" className="collapse-btn" onClick={onToggleCollapsed} aria-expanded={!collapsed}>
          {collapsed ? 'Show' : 'Hide'}
        </button>
      </div>
      {!collapsed &&
        (entries.length === 0 ? (
          <div className="empty">No cross-AZ traffic in {RANGE_LABEL[range]}.</div>
        ) : (
          <div className="offenders-table-scroll">
            <table className="offenders-table">
              <thead>
                <tr>
                  <th>#</th>
                  <th>Source</th>
                  <th>Destination</th>
                  <th>AZ path</th>
                  <th>Namespace</th>
                  <th>Cross-AZ traffic</th>
                  <th>Est. cost</th>
                  <th>% of total</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((e, i) => {
                  const pct = totalCrossAZCostUSD > 0 ? (100 * e.cost_usd) / totalCrossAZCostUSD : 0
                  return (
                    <tr key={i} className="offender-row" onClick={() => onSelectRoute(e)} tabIndex={0} role="button">
                      <td>{i + 1}</td>
                      <td>{e.src_workload}</td>
                      <td>{e.dst_workload}</td>
                      <td>
                        {e.src_zone} → {e.dst_zone}
                      </td>
                      <td>
                        {e.src_namespace === e.dst_namespace ? e.src_namespace : `${e.src_namespace} → ${e.dst_namespace}`}
                      </td>
                      <td>{fmtGB(e.gb)}</td>
                      <td className="cost">{fmtUSD(e.cost_usd)}</td>
                      <td>{pct.toFixed(1)}%</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
            <div className="offenders-window-note">Time window: {RANGE_LABEL[range]}</div>
          </div>
        ))}
    </div>
  )
}
