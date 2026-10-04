import { useMemo, useState } from 'react'
import type { MapEntry, MapRange } from '../types'
import { fmtGB, fmtUSD } from '../format'
import { costScale } from '../theme'
import { RANGE_LABEL } from '../labels'

export interface TopOffendersTableProps {
  entries: MapEntry[]
  totalCrossAZCostUSD: number
  range: MapRange
  onSelectRoute: (entry: MapEntry) => void
  collapsed: boolean
  onToggleCollapsed: () => void
}

type SortKey = 'cost' | 'gb'

function SortBtn({ k, sort, onSort, children }: { k: SortKey; sort: SortKey; onSort: (k: SortKey) => void; children: string }) {
  return (
    <button type="button" onClick={() => onSort(k)}>
      {children}
      <span aria-hidden="true">{sort === k ? '↓' : ''}</span>
    </button>
  )
}

/** Top cost drivers. Sortable column headers (aria-sort), rows activate with Enter/Space, and a
 * share bar gives a non-colour cue for each route's weight. */
export function TopOffendersTable({ entries, totalCrossAZCostUSD, range, onSelectRoute, collapsed, onToggleCollapsed }: TopOffendersTableProps) {
  const [sort, setSort] = useState<SortKey>('cost')
  const rows = useMemo(() => [...entries].sort((a, b) => (sort === 'cost' ? b.cost_usd - a.cost_usd : b.gb - a.gb)).slice(0, 15), [entries, sort])
  const max = rows.reduce((m, e) => Math.max(m, e.cost_usd), 0)

  return (
    <section className="card" aria-labelledby="offenders-title">
      <div className="card-head">
        <h2 id="offenders-title">Top cost drivers</h2>
        <span className="card-sub">{RANGE_LABEL[range]} · top {Math.min(15, entries.length)} of {entries.length} routes</span>
        <span className="spacer" />
        <button type="button" className="btn btn-ghost" onClick={onToggleCollapsed} aria-expanded={!collapsed} aria-controls="offenders-body">
          {collapsed ? 'Show' : 'Hide'}
        </button>
      </div>
      {!collapsed && (
        <div className="card-body table-wrap" id="offenders-body">
          {rows.length === 0 ? (
            <div className="empty-state" style={{ height: 120 }}>No cross-AZ traffic in the {RANGE_LABEL[range]}.</div>
          ) : (
            <table>
              <caption className="sr-only">Cross-AZ routes ranked by {sort === 'cost' ? 'cost' : 'traffic'}. Press Enter on a row to show it on the map.</caption>
              <thead>
                <tr>
                  <th scope="col">#</th>
                  <th scope="col">Source</th>
                  <th scope="col">Destination</th>
                  <th scope="col">Zones</th>
                  <th scope="col" className="r" aria-sort={sort === 'gb' ? 'descending' : 'none'}><SortBtn k="gb" sort={sort} onSort={setSort}>Traffic</SortBtn></th>
                  <th scope="col" className="r" aria-sort={sort === 'cost' ? 'descending' : 'none'}><SortBtn k="cost" sort={sort} onSort={setSort}>Cost</SortBtn></th>
                  <th scope="col" className="r">Share</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((e, i) => {
                  const pct = totalCrossAZCostUSD > 0 ? (100 * e.cost_usd) / totalCrossAZCostUSD : 0
                  const activate = () => onSelectRoute(e)
                  return (
                    <tr
                      key={`${e.src_namespace}/${e.src_workload}>${e.dst_namespace}/${e.dst_workload}@${e.src_zone}>${e.dst_zone}`}
                      className="clickable"
                      tabIndex={0}
                      onClick={activate}
                      onKeyDown={(ev) => {
                        if (ev.key === 'Enter' || ev.key === ' ') {
                          ev.preventDefault()
                          activate()
                        }
                      }}
                      aria-label={`${e.src_workload} to ${e.dst_workload}, ${fmtUSD(e.cost_usd)}`}
                    >
                      <td className="muted num">{i + 1}</td>
                      <td className="wl">{e.src_workload}<small>{e.src_namespace}</small></td>
                      <td className="wl">{e.dst_workload}<small>{e.dst_namespace}</small></td>
                      <td className="zpath">{e.src_zone} → {e.dst_zone}</td>
                      <td className="r num">{fmtGB(e.gb)}</td>
                      <td className="r"><span className="cost-cell"><i style={{ background: costScale(max > 0 ? e.cost_usd / max : 0) }} aria-hidden="true" />{fmtUSD(e.cost_usd)}</span></td>
                      <td className="r"><span className="share"><span className="share-bar" aria-hidden="true"><i style={{ width: `${Math.max(1, pct)}%` }} /></span><span className="num">{pct < 0.1 && pct > 0 ? '<0.1' : pct.toFixed(1)}%</span></span></td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          )}
        </div>
      )}
    </section>
  )
}
