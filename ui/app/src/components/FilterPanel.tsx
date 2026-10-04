import { useId, useState } from 'react'
import type { FilterOption } from '../mapFilters'
import { searchOptions } from '../mapFilters'

export interface FilterGroupDef {
  key: string
  title: string
  options: FilterOption[]
  selected: Set<string>
  onToggle: (value: string) => void
  onSelectAll: () => void
  onClearGroup: () => void
  searchable: boolean
  defaultOpen?: boolean
}

function FilterGroup({ group }: { group: FilterGroupDef }) {
  const [open, setOpen] = useState(group.defaultOpen ?? true)
  const [query, setQuery] = useState('')
  const bodyId = useId()
  const visible = group.searchable ? searchOptions(group.options, query) : group.options
  const n = group.selected.size

  return (
    <div className="fgroup">
      <button type="button" className="fgroup-head" onClick={() => setOpen((v) => !v)} aria-expanded={open} aria-controls={bodyId}>
        <span className="chev" aria-hidden="true">›</span>
        {group.title}
        <span className={`count${n ? ' on' : ''}`}>{n ? `${n} selected` : `${group.options.length}`}</span>
      </button>
      {open && (
        <div className="fgroup-body" id={bodyId}>
          {group.searchable && group.options.length > 6 && (
            <input
              type="search"
              className="input input-search"
              placeholder={`Search ${group.title.toLowerCase()}`}
              aria-label={`Search ${group.title.toLowerCase()}`}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          )}
          {group.options.length > 1 && (
            <div className="fgroup-actions">
              <button type="button" className="link-btn" onClick={group.onSelectAll}>Select all</button>
              {n > 0 && <button type="button" className="link-btn" onClick={group.onClearGroup}>Clear</button>}
            </div>
          )}
          <div className="options" role="group" aria-label={group.title}>
            {visible.length === 0 ? (
              <div className="option-empty">No matches</div>
            ) : (
              visible.map((opt) => (
                <label key={opt.value} className="option" title={opt.label}>
                  <input type="checkbox" checked={group.selected.has(opt.value)} onChange={() => group.onToggle(opt.value)} />
                  <span className="label">{opt.label}</span>
                  <span className="cnt" aria-label={`${opt.count} routes`}>{opt.count}</span>
                </label>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  )
}

export interface RangeFilterDef {
  key: string
  title: string
  min: number | null
  max: number | null
  onMinChange: (v: number | null) => void
  onMaxChange: (v: number | null) => void
  unit: string
  step?: number
  presets?: { label: string; min: number }[]
}

function RangeFilterGroup({ range }: { range: RangeFilterDef }) {
  const [open, setOpen] = useState(true)
  const bodyId = useId()
  const active = range.min !== null || range.max !== null
  const parse = (v: string) => (v === '' ? null : Math.max(0, Number(v)))
  return (
    <div className="fgroup">
      <button type="button" className="fgroup-head" onClick={() => setOpen((v) => !v)} aria-expanded={open} aria-controls={bodyId}>
        <span className="chev" aria-hidden="true">›</span>
        {range.title}
        <span className={`count${active ? ' on' : ''}`}>{active ? 'active' : 'any'}</span>
      </button>
      {open && (
        <div className="fgroup-body" id={bodyId}>
          {range.presets && (
            <div className="presets" role="group" aria-label={`${range.title} presets`}>
              {range.presets.map((pr) => {
                const on = range.min === pr.min && range.max === null
                return (
                  <button
                    key={pr.label}
                    type="button"
                    className="preset"
                    aria-pressed={on}
                    onClick={() => {
                      range.onMinChange(on ? null : pr.min)
                      range.onMaxChange(null)
                    }}
                  >
                    {pr.label}
                  </button>
                )
              })}
            </div>
          )}
          <div className="range-row">
            <label className="field">
              Min ({range.unit})
              <input className="input" type="number" inputMode="decimal" min={0} step={range.step ?? 0.01} value={range.min ?? ''} placeholder="any" onChange={(e) => range.onMinChange(parse(e.target.value))} />
            </label>
            <label className="field">
              Max ({range.unit})
              <input className="input" type="number" inputMode="decimal" min={0} step={range.step ?? 0.01} value={range.max ?? ''} placeholder="any" onChange={(e) => range.onMaxChange(parse(e.target.value))} />
            </label>
          </div>
        </div>
      )}
    </div>
  )
}

export interface ActiveChip {
  key: string
  label: string
  onRemove: () => void
}

export interface FilterPanelProps {
  groups: FilterGroupDef[]
  advancedGroups: FilterGroupDef[]
  ranges: RangeFilterDef[]
  activeChips: ActiveChip[]
  onResetAll: () => void
  maxConnections: number
  onMaxConnectionsChange: (n: number) => void
  hiddenRouteCount: number
  totalRouteCount: number
  onClose: () => void
}

/** Left filter sidebar. Primary filters (zone, namespace, workload, cost) are always visible;
 * direction-specific and route filters live under "More filters" to keep the panel calm. */
export function FilterPanel(p: FilterPanelProps) {
  const [showAdvanced, setShowAdvanced] = useState(false)
  return (
    <aside className="filters" id="filters" aria-label="Filters">
      <div className="filters-head">
        <h2>Filters</h2>
        {p.activeChips.length > 0 && (
          <button type="button" className="link-btn" onClick={p.onResetAll}>Reset all</button>
        )}
        <button type="button" className="btn btn-ghost btn-icon" onClick={p.onClose} aria-label="Close filters">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" aria-hidden="true"><path d="M15 18l-6-6 6-6" /></svg>
        </button>
      </div>

      {p.activeChips.length > 0 && (
        <ul className="chips" aria-label="Active filters" style={{ listStyle: 'none', margin: 0 }}>
          {p.activeChips.map((chip) => (
            <li key={chip.key} className="filter-chip">
              <span title={chip.label}>{chip.label}</span>
              <button type="button" onClick={chip.onRemove} aria-label={`Remove filter ${chip.label}`}>×</button>
            </li>
          ))}
        </ul>
      )}

      <div className="filters-scroll">
        {p.groups.map((g) => (
          <FilterGroup key={g.key} group={g} />
        ))}
        {p.ranges.map((r) => (
          <RangeFilterGroup key={r.key} range={r} />
        ))}
        <div className="fgroup">
          <button type="button" className="fgroup-head" onClick={() => setShowAdvanced((v) => !v)} aria-expanded={showAdvanced}>
            <span className="chev" aria-hidden="true">›</span>
            More filters
            <span className="count">source, destination, route</span>
          </button>
        </div>
        {showAdvanced && p.advancedGroups.map((g) => <FilterGroup key={g.key} group={{ ...g, defaultOpen: g.selected.size > 0 }} />)}
      </div>

      <div className="filters-foot">
        <label className="field">
          Max routes on map
          <select className="select" value={p.maxConnections} onChange={(e) => p.onMaxConnectionsChange(Number(e.target.value))}>
            {[10, 25, 50, 100, 250].map((n) => (
              <option key={n} value={n}>{n}</option>
            ))}
            <option value={Infinity}>No limit</option>
          </select>
        </label>
        <div className="hint" role="status">
          {p.hiddenRouteCount > 0
            ? `Showing top ${p.totalRouteCount - p.hiddenRouteCount} of ${p.totalRouteCount} routes. Raise the limit to see the rest.`
            : `All ${p.totalRouteCount} route${p.totalRouteCount === 1 ? '' : 's'} shown.`}
        </div>
      </div>
    </aside>
  )
}
