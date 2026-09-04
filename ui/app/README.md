# ZoneTax dashboard (`ui/app`)

React + TypeScript SPA (Vite), the ZoneTax collector's embedded UI. Renders the zone-to-zone
cross-AZ traffic map using [@xyflow/react](https://reactflow.dev) (draggable zone nodes, floating
edges with cost-pill labels + hover tooltips, native pinch/scroll zoom) plus a top-offenders
table, polling the collector's own `/api/v1/costs` REST endpoint every 10s.

## Develop

```sh
npm install
npm run dev
```

By default `vite.config.ts` proxies `/api/*` to `http://localhost:18099` — port-forward a real
collector there to develop against live data:

```sh
kubectl -n zonetax port-forward svc/zonetax-collector 18099:8080
```

## Build

```sh
npm run build
```

Outputs static assets to `../dist`, which `ui/ui.go` embeds into the collector binary via
`go:embed`. `dist/` and `node_modules/` are gitignored — never commit them; the collector's
Dockerfile and CI both run this build step from source before compiling Go.

## Source layout

- `src/App.tsx` — top-level layout, data polling, ReactFlow wiring
- `src/zoneFlow.ts` — aggregates raw cost entries into zone-to-zone routes (pure, unit-testable)
- `src/components/ZoneNode.tsx`, `src/components/ZoneFlowEdge.tsx` — custom ReactFlow node/edge
- `src/floatingEdgeUtils.ts` — edge-to-border-intersection geometry (the "floating edge" pattern,
  so edges never visually detach from a dragged node)
- `src/format.ts` — number/color formatting shared across the dashboard
