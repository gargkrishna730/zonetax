import { useState } from 'react'
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
}

/** One collapsible, independently-scrollable, searchable filter group — checkbox + label +
 * real count per row, a group-local search box for long lists, and Select all / Clear controls.
 * Matches the reference service map's per-group pattern (e.g. "Namespace (8/8)"). */
function FilterGroup({ group }: { group: FilterGroupDef }) {
  const [expanded, setExpanded] = useState(true)
  const [query, setQuery] = useState('')
  const visible = group.searchable ? searchOptions(group.options, query) : group.options

  return (
    <div className="filter-group">
      <button type="button" className="filter-group-head" onClick={() => setExpanded((v) => !v)} aria-expanded={expanded}>
        <span className={`chevron ${expanded ? 'open' : ''}`}>▸</span>
        <span className="filter-group-title">{group.title}</span>
        <span className="filter-group-count">
          {group.selected.size}/{group.options.length}
        </span>
      </button>
      {expanded && (
        <div className="filter-group-body">
          {group.searchable && group.options.length > 6 && (
            <input
              type="text"
              className="filter-search"
              placeholder={`Search ${group.title.toLowerCase()}…`}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          )}
          <div className="filter-group-actions">
            <button type="button" onClick={group.onSelectAll}>
              Select all
            </button>
            <button type="button" onClick={group.onClearGroup}>
              Clear
            </button>
          </div>
          <div className="filter-options-scroll">
            {visible.length === 0 ? (
              <div className="filter-empty">No matches</div>
            ) : (
              visible.map((opt) => (
                <label key={opt.value} className="filter-option-row">
                  <input type="checkbox" checked={group.selected.has(opt.value)} onChange={() => group.onToggle(opt.value)} />
                  <span className="filter-option-label" title={opt.label}>
                    {opt.label}
                  </span>
                  <span className="filter-option-count">{opt.count}</span>
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
}

function RangeFilterGroup({ range }: { range: RangeFilterDef }) {
  const [expanded, setExpanded] = useState(true)
  return (
    <div className="filter-group">
      <button type="button" className="filter-group-head" onClick={() => setExpanded((v) => !v)} aria-expanded={expanded}>
        <span className={`chevron ${expanded ? 'open' : ''}`}>▸</span>
        <span className="filter-group-title">{range.title}</span>
        <span className="filter-group-count">{range.min !== null || range.max !== null ? 'active' : 'any'}</span>
      </button>
      {expanded && (
        <div className="filter-group-body">
          <div className="range-inputs">
            <label>
              Min
              <input
                type="number"
                step={range.step ?? 0.01}
                value={range.min ?? ''}
                placeholder="any"
                onChange={(e) => range.onMinChange(e.target.value === '' ? null : Number(e.target.value))}
              />
              {range.unit}
            </label>
            <label>
              Max
              <input
                type="number"
                step={range.step ?? 0.01}
                value={range.max ?? ''}
                placeholder="any"
                onChange={(e) => range.onMaxChange(e.target.value === '' ? null : Number(e.target.value))}
              />
              {range.unit}
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
  ranges: RangeFilterDef[]
  activeChips: ActiveChip[]
  onResetAll: () => void
  maxConnections: number
  onMaxConnectionsChange: (n: number) => void
  hiddenRouteCount: number
  totalRouteCount: number
}

/** The left filter panel: fixed width, independently scrollable from the main content, every
 * group collapsible/searchable with real counts, active-filter chips, and the Max-connections
 * control with an honest "N of M routes shown" explanation of what's aggregated/hidden. */
export function FilterPanel({
  groups,
  ranges,
  activeChips,
  onResetAll,
  maxConnections,
  onMaxConnectionsChange,
  hiddenRouteCount,
  totalRouteCount,
}: FilterPanelProps) {
  return (
    <aside className="filter-panel">
      <div className="filter-panel-head">
        <h2>Filters</h2>
        {activeChips.length > 0 && (
          <button type="button" className="reset-all-btn" onClick={onResetAll}>
            Reset
          </button>
        )}
      </div>

      {activeChips.length > 0 && (
        <div className="active-chips">
          {activeChips.map((chip) => (
            <span key={chip.key} className="active-chip">
              {chip.label}
              <button type="button" onClick={chip.onRemove} aria-label={`Remove filter ${chip.label}`}>
                ×
              </button>
            </span>
          ))}
        </div>
      )}

      <div className="filter-panel-scroll">
        {groups.map((g) => (
          <FilterGroup key={g.key} group={g} />
        ))}
        {ranges.map((r) => (
          <RangeFilterGroup key={r.key} range={r} />
        ))}
      </div>

      <div className="max-connections-control">
        <label>
          Max connections
          <select value={maxConnections} onChange={(e) => onMaxConnectionsChange(Number(e.target.value))}>
            {[10, 25, 50, 100, 250].map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
            <option value={Infinity}>No limit</option>
          </select>
        </label>
        <div className="max-connections-hint">
          {hiddenRouteCount > 0
            ? `Showing the top ${totalRouteCount - hiddenRouteCount} of ${totalRouteCount} routes by the active sort — raise this limit to see the rest. No route is hidden below a cost/traffic threshold.`
            : `Showing all ${totalRouteCount} route${totalRouteCount === 1 ? '' : 's'} — none hidden.`}
        </div>
      </div>
    </aside>
  )
}
