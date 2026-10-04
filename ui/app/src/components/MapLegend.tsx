import { costLegendStops } from '../theme'

/** Map legend generated from the same cost scale the edges use, so it can never drift. */
export function MapLegend({ viewMode }: { viewMode: 'zone' | 'workload' }) {
  return (
    <div className="legend" aria-label="Map legend">
      <span className="legend-item">
        <span aria-hidden="true">→</span> {viewMode === 'zone' ? 'source zone to destination zone' : 'source workload to destination'}
      </span>
      <span className="legend-item">
        <span className="legend-ramp" aria-hidden="true">
          {costLegendStops().map((c) => (
            <i key={c} style={{ background: c }} />
          ))}
        </span>
        low to high cost
      </span>
      <span className="legend-item">
        <span className="legend-width" aria-hidden="true">
          <i style={{ height: 2 }} />
          <i style={{ height: 6 }} />
        </span>
        more traffic
      </span>
      <span className="legend-item">
        <span className="legend-dash" aria-hidden="true" />
        partial window
      </span>
    </div>
  )
}
