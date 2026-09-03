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
- [x] **M3** — UI: live Sankey/chord diagram of $ flow between AZs + top-offenders table
      (namespace/workload breakdown). Static SPA (D3.js from CDN, no build step), served by the
      collector via Go embed.FS at "/", polling /api/v1/costs + /api/v1/top every 10s. Iterated
      twice on real user feedback against the live solrn-dev dashboard:
      1. Two real bugs: cost math only counted AWS's per-direction rate (2x undercounted vs the
         real bill, since cross-AZ GB is billed at both ends); the zone-only Sankey graph crashed
         ("circular link") on bidirectional zone traffic, a common case, silently blanking the
         whole page. Fixed with a workload->src-zone->dst-zone 3-column DAG + per-section
         try/catch isolation.
      2. The 3-column DAG became unreadable once one workload's cost ($218) dwarfed the rest
         ($0.0001 range) — dozens of overlapping unreadable labels. Replaced with a compact
         fixed-layout node-link zone map (nodes = AZs, curved directed edges = cost flows,
         red->green color scale by relative cost, sqrt-scaled edge width so the dominant flow
         doesn't crush the rest) plus a workload filter dropdown (the "service map" need is now
         served by filtering, not by cramming every workload into the diagram) and a
         red/green cost color on the offenders table too.
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
