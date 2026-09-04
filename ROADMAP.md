# Roadmap

Scope: AWS/EKS first. Conntrack-based sampling for MVP (eBPF is a possible v2).

- [x] **M0** — Repo scaffold, LICENSE, README, architecture doc, Helm chart shell, CI skeleton
- [x] **M1** — Agent captures conntrack flows, maps IP→pod→node→AZ via k8s informer + node labels,
      exposes raw byte-count metrics (Prometheus format). Unit tested (conntrack parser, pod/node
      index incl. fake-clientset informer test, aggregator, metrics registration); accuracy
      validation against a real multi-AZ cluster is still pending — see "Known limitations" below.
- [x] **M2** — Cost engine: versioned AWS pricing table (YAML), collector aggregates AZ-pair bytes
      into $ cost, REST API to query current/historical spend. Deployed and validated live against
      solrn-dev — see "Known limitations" below.
- [x] **M3** — UI: interactive zone-to-zone traffic map + top-offenders table (namespace/workload
      breakdown). Served by the collector via Go embed.FS at "/", polling /api/v1/costs +
      /api/v1/top every 10s. Iterated five times on real user feedback against the live
      solrn-dev dashboard:
      1. Two real bugs: cost math only counted AWS's per-direction rate (2x undercounted vs the
         real bill, since cross-AZ GB is billed at both ends); the zone-only Sankey graph crashed
         ("circular link") on bidirectional zone traffic, a common case, silently blanking the
         whole page. Fixed with a workload->src-zone->dst-zone 3-column DAG + per-section
         try/catch isolation.
      2. The 3-column DAG became unreadable once one workload's cost ($218) dwarfed the rest —
         replaced with a static circular zone map + workload filter dropdown. Still visually
         collapsed and wasn't interactive/draggable like a reference APM service map.
      3. Rebuilt as a force-directed graph (d3-force/drag/zoom): zone anchors + draggable
         workload nodes, edges workload->home-zone. Fixed interactivity, but never actually
         showed the thing being asked for — which zone the traffic went TO, only which zone a
         workload lives in.
      4. User pointed at a real APM service-map screenshot (boxes + directed labeled arrows,
         source->destination) and asked for that shape directly. Rebuilt as a static zone-to-zone
         flow diagram (still plain D3/SVG): nodes are zones only, no physics, directed curved
         edges are the real source-zone->destination-zone cost aggregate, each with a $-cost pill
         and hover breakdown by contributing workload.
      5. Correctly right in *what* it showed, but user called out (with a real APM tool
         screenshot for comparison) that hand-rolled SVG could never deliver proper drag/zoom
         interactivity, and pushed to invest in a real frontend stack instead of another SVG
         patch. Rewrote the whole dashboard as a React + TypeScript SPA (Vite) using
         **@xyflow/react (React Flow)** for the graph: zones are custom draggable nodes, routes
         are custom "floating" edges (dynamically re-anchoring to whichever box side faces the
         other node, so dragging never visually detaches an edge) with cost-pill labels and
         hover tooltips, native pinch/scroll zoom and pan. This retired three iterations' worth
         of hand-rolled drag/zoom/curve-geometry bugs by delegating that problem to a
         purpose-built graph library instead of re-solving it a fourth time. Frontend source
         lives in ui/app/; the collector's Dockerfile now has a Node build stage that runs
         `npm run build` before the Go build embeds ui/dist via go:embed (dist/node_modules are
         gitignored, never committed, to prevent drift from source); CI's build-test job builds
         the UI before `go build ./...` for the same reason. Verified with a real headless
         Chromium (Playwright, driven manually since the browser tool can't reach localhost)
         against the live solrn-dev collector via a Vite dev-server API proxy: confirmed actual
         node drag (caught and fixed two real bugs this way — hidden connection Handles were
         still catching pointer events and stealing drag gestures; nodes were plain memoized
         props with no onNodesChange wiring, so ReactFlow silently ignored drag output entirely),
         confirmed scroll-to-zoom changes the viewport transform, confirmed the workload filter
         correctly recomputes to just that workload's real routes, and confirmed the edge hover
         tooltip renders the correct per-workload cost breakdown.
      6. Added a persistent click-to-drill-down panel on zone-to-zone edges (not just a hover
         tooltip, which caps at "top 5" and can't be clicked into): shows the complete,
         sortable list of real workload pairs behind a route, each row pivoting into the
         Workload↔Workload view scoped to exactly that pair via an exact-pair filter (not the
         coarser single-workload dropdown filter).
      7. **Observability redesign** (SRE + design pass on ambiguous time semantics, no
         daily/date filtering, cramped map, unlabeled "All workloads"): see the "Time-series &
         observability redesign" entry below for the full breakdown — new backend history API,
         redesigned KPI/session panel, daily/hourly cost chart, fullscreen map with search and
         node-focus, and a real legend.
- [x] **Cross-AZ cost accuracy fix (~80x overcount).** User asked to compare ZoneTax's tracked
      cost against real AWS Cost Explorer billing (`DataTransfer-Regional-Bytes`, ~$2.13/day
      account-wide) — ZoneTax was reporting an extrapolated ~$173/day, an ~80x overcount. Root
      cause: `/proc/net/nf_conntrack`'s `bytes=` field is cumulative for a connection's entire
      lifetime, not delta-since-last-sample; the agent was calling `counter.Add(cumulativeBytes)`
      every 15s tick, so any long-lived connection (e.g. a persistent OTel gRPC stream) had its
      full lifetime byte count re-added on every tick, compounding into massive overcounting —
      this also explains why that workload topped the offenders list (an artifact, not real
      cost). Fixed with a new `internal/deltatrack` package (6 unit tests) that tracks
      last-seen-cumulative-bytes per connection and emits true per-sample deltas, handling
      counter resets (connection restarts) by treating the post-reset value as a fresh delta
      rather than a negative number. Deployed and re-verified against live `solrn-dev` data:
      post-fix extrapolated rate came down to ~$8.35/day over a real 10-minute sample — still not
      an exact match to the ~$2.13/day AWS figure (that comparison itself has real noise: AWS's
      number is account-wide across all resources, not just this EKS cluster, and a 10-minute
      sample extrapolated to a day vs. a 7-day steady average isn't apples-to-apples), but the
      order-of-magnitude bug is confirmed fixed (~80x → ~4x, and the residual gap has a
      plausible, non-buggy explanation rather than being unexplained).
- [x] **Time-series & observability redesign.** User feedback: KPI values didn't state their
      measurement period, "since agents last restarted" was ambiguous and gave no daily
         breakdown, exact collection time/timezone/freshness weren't visible, there was no
         daily/hourly cost view, the map was too small to inspect and had no fullscreen mode,
         "All workloads" was hard to read, and there was no legend or node-search/focus. Backend:
         added `internal/collector/history.go` (in-memory hourly-bucketed history built by
         snapshotting the collector's cumulative cost/traffic totals every cycle and diffing
         consecutive snapshots — since Summary's totals are cumulative Prometheus-counter values,
         not per-cycle deltas, matching the same class of problem the deltatrack fix solved at the
         byte-counting layer; counter resets are handled the same way). Every bucket honestly
         reports `complete`/`has_data` so a partial/in-progress or before-history-began bucket is
         never presented as equivalent to a fully observed one. New `GET /api/v1/history?range=
         1h|6h|24h|7d` endpoint (8 collector-level + 5 API-level tests). `/api/v1/costs` and
         `/api/v1/top` now also return `server_time_utc`, `collector_started_at_utc`, and
         `scrape_interval_seconds` so the UI never has to guess or invent a timestamp. Frontend
         (Vitest + Testing Library added as the project's first frontend test framework, 39 tests):
         redesigned `KpiPanel` labels every KPI as an explicit "this session" metric (not a
         calendar day) with a session panel showing exact last-collected/server-time/session-start
         timestamps + timezone and collection interval; new `CostHistoryChart` (daily bars for the
         7d range, hourly for shorter ranges, real loading/error/empty states, hover tooltip with
         exact window + completeness); map got a search box (workload/namespace/zone substring
         match, dims non-matching nodes/edges rather than removing them), click-to-focus on any
         node (highlights just its inbound/outbound routes, generalizing the "click zone A→B, see
         workload breakdown" request to any node in either view), a real `MapLegend` (arrow
         direction, color scale, line-thickness meaning), and a fullscreen mode (Escape key +
         close button, backdrop, filters/search/focus/positions all carry over since it's the same
         ReactFlow instance reparented, not a second diagram).
- [ ] **M4** — Alerting: Slack webhook on $/hour threshold breach.
- [ ] **M5** — CLI (`zonetax top`, `zonetax report --since 1h`) hitting the collector API.
- [ ] **M6** — Polish: multi-arch CI images, demo GIF against a real multi-AZ EKS cluster,
      CONTRIBUTING.md, Helm repo publishing, blog writeup.

## Known limitations (post-M1)

- **Validated against a live multi-AZ cluster (solrn-dev, 2026-09-02).** Deployed via Helm to a
  real 3-AZ EKS cluster; confirmed real cross-AZ byte counts attributed correctly to actual
  workloads (e.g. `solrn-aura-backend-api-dev` us-east-1c -> us-east-1a). Two real issues were
  found and fixed during validation, both now baked into the Helm chart:
  1. `net.netfilter.nf_conntrack_acct` was disabled by default on nodes (EKS AL2023), so byte
     counts came back as 0 — fixed with a privileged init container that sets the sysctl
     (`agent.enableConntrackAcct`, default true).
  2. The distroless nonroot container couldn't read `/proc/net/nf_conntrack` even with
     NET_ADMIN — fixed by running the agent container as `runAsUser: 0`.
- **hostNetwork + privileged init container required.** The agent runs with `hostNetwork: true`
  and (by default) a privileged init container to flip the conntrack-accounting sysctl — real
  security review implications for locked-down/prod clusters. Fine for dev/test; revisit before
  recommending this for production without review.
- **ReplicaSet→Deployment name resolution is a heuristic** (strips the pod-template-hash suffix),
  not an API lookup — unusual naming conventions could produce a wrong workload label.
- **Same-node pod-to-pod traffic isn't distinguished from same-AZ cross-node traffic** yet; both
  land in the same `same_az_bytes_total` bucket.
- **DaemonSet doesn't tolerate node resource pressure well** — on a memory-constrained node in
  testing, the agent pod stayed Pending indefinitely (expected; not a bug, but worth noting for
  resource-tight clusters).

## Non-goals (for now)

- Multi-cloud (GCP/Azure) — architecture should stay portable, but AWS is the only implementation.
- Full eBPF — conntrack sampling first; revisit if accuracy/perf demands it.
- Automated remediation (topology spread, pod affinity) — ZoneTax is observability-first.
- Historical cost storage beyond Prometheus retention — no bundled long-term TSDB in MVP.
