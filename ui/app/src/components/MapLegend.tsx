export interface MapLegendProps {
  viewMode: 'zone' | 'workload'
}

/** Static legend explaining the map's visual encoding: arrow direction, color scale, and what
 * line thickness means — previously only a one-line hint existed with no color/thickness key at
 * all. */
export function MapLegend({ viewMode }: MapLegendProps) {
  return (
    <div className="map-legend">
      <div className="map-legend-row">
        <span className="legend-arrow">→</span>
        <span>Arrow points from source {viewMode === 'zone' ? 'zone' : 'workload'} to destination</span>
      </div>
      <div className="map-legend-row">
        <span className="legend-swatches">
          <i className="legend-swatch" style={{ background: 'rgb(0,104,55)' }} />
          <i className="legend-swatch" style={{ background: 'rgb(249,247,174)' }} />
          <i className="legend-swatch" style={{ background: 'rgb(165,0,38)' }} />
        </span>
        <span>Color: cheap → expensive route (relative to the most expensive route shown)</span>
      </div>
      <div className="map-legend-row">
        <span className="legend-thickness">
          <i className="legend-line thin" />
          <i className="legend-line thick" />
        </span>
        <span>Line thickness: relative traffic volume (GB) on that route</span>
      </div>
      <div className="map-legend-row legend-hint">
        Drag boxes to rearrange · scroll/pinch to zoom · click a route for full breakdown · click a
        box to focus its routes
      </div>
    </div>
  )
}
