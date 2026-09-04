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
