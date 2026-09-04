import { fmtGB, fmtUSD, costColor } from '../format'
import type { WorkloadPairBreakdown } from '../flowGraph'

export interface DrillDownSelection {
  srcLabel: string
  dstLabel: string
  totalCost: number
  totalGb: number
  pairs: WorkloadPairBreakdown[]
}

/** Persistent drill-down panel for one zone-to-zone (or workload-to-workload) route, opened by
 * clicking an edge in the traffic map. Deliberately NOT another tooltip: an SRE investigating a
 * real cost spike needs the complete list of contributing workload pairs (not capped at 5), the
 * ability to actually read it without holding a mouse hover, and a way to act on a row —
 * clicking one pivots straight into the Workload -> Workload view already scoped to that exact
 * pair, so "I want to see the workload-level cost between zone A and zone B" is one click away
 * instead of a manual dropdown hunt. */
export function EdgeDrillDownPanel({
  selection,
  onClose,
  onSelectPair,
}: {
  selection: DrillDownSelection
  onClose: () => void
  onSelectPair: (pair: WorkloadPairBreakdown) => void
}) {
  const maxCost = selection.pairs[0]?.cost ?? 0

  return (
    <div className="drilldown-panel">
      <div className="drilldown-head">
        <div>
          <div className="drilldown-title">
            {selection.srcLabel} → {selection.dstLabel}
          </div>
          <div className="drilldown-subtitle">
            {fmtUSD(selection.totalCost)} · {fmtGB(selection.totalGb)} · {selection.pairs.length} workload
            pair{selection.pairs.length === 1 ? '' : 's'}
          </div>
        </div>
        <button type="button" className="drilldown-close" onClick={onClose} aria-label="Close">
          ×
        </button>
      </div>
      <div className="drilldown-hint">Click a row to see that pair alone in Workload → Workload</div>
      <div className="drilldown-list">
        {selection.pairs.map((p) => {
          const color = costColor(p.cost, maxCost)
          return (
            <button
              type="button"
              key={p.srcKey + '>' + p.dstKey}
              className="drilldown-row"
              onClick={() => onSelectPair(p)}
            >
              <span className="drilldown-row-route">
                <span className="drilldown-row-workload">{p.srcLabel}</span>
                <span className="drilldown-row-arrow">→</span>
                <span className="drilldown-row-workload">{p.dstLabel}</span>
              </span>
              <span className="drilldown-row-metrics">
                <span className="drilldown-row-gb">{fmtGB(p.gb)}</span>
                <span className="drilldown-row-cost" style={{ color }}>
                  <span className="cost-dot" style={{ background: color }} />
                  {fmtUSD(p.cost)}
                </span>
              </span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
