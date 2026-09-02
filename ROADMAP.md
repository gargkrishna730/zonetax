# Roadmap

Scope: AWS/EKS first. Conntrack-based sampling for MVP (eBPF is a possible v2).

- [x] **M0** — Repo scaffold, LICENSE, README, architecture doc, Helm chart shell, CI skeleton
- [ ] **M1** — Agent captures conntrack flows, maps IP→pod→node→AZ via k8s informer + node labels,
      exposes raw byte-count metrics (Prometheus format). Validate accuracy before adding cost math.
- [ ] **M2** — Cost engine: versioned AWS pricing table (YAML), collector aggregates AZ-pair bytes
      into $ cost, REST API to query current/historical spend.
- [ ] **M3** — UI: live Sankey/chord diagram of $ flow between AZs + top-offenders table
      (namespace/workload breakdown). Static SPA, no build step, served by the collector.
- [ ] **M4** — Alerting: Slack webhook on $/hour threshold breach.
- [ ] **M5** — CLI (`zonetax top`, `zonetax report --since 1h`) hitting the collector API.
- [ ] **M6** — Polish: multi-arch CI images, demo GIF against a real multi-AZ EKS cluster,
      CONTRIBUTING.md, Helm repo publishing, blog writeup.

## Non-goals (for now)

- Multi-cloud (GCP/Azure) — architecture should stay portable, but AWS is the only implementation.
- Full eBPF — conntrack sampling first; revisit if accuracy/perf demands it.
- Automated remediation (topology spread, pod affinity) — ZoneTax is observability-first.
- Historical cost storage beyond Prometheus retention — no bundled long-term TSDB in MVP.
