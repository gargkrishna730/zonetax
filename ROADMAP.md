# Roadmap

Scope: AWS/EKS first. Conntrack-based sampling for MVP (eBPF is a possible v2).

- [x] **M0** — Repo scaffold, LICENSE, README, architecture doc, Helm chart shell, CI skeleton
- [x] **M1** — Agent captures conntrack flows, maps IP→pod→node→AZ via k8s informer + node labels,
      exposes raw byte-count metrics (Prometheus format). Unit tested (conntrack parser, pod/node
      index incl. fake-clientset informer test, aggregator, metrics registration); accuracy
      validation against a real multi-AZ cluster is still pending — see "Known limitations" below.
- [ ] **M2** — Cost engine: versioned AWS pricing table (YAML), collector aggregates AZ-pair bytes
      into $ cost, REST API to query current/historical spend.
- [ ] **M3** — UI: live Sankey/chord diagram of $ flow between AZs + top-offenders table
      (namespace/workload breakdown). Static SPA, no build step, served by the collector.
- [ ] **M4** — Alerting: Slack webhook on $/hour threshold breach.
- [ ] **M5** — CLI (`zonetax top`, `zonetax report --since 1h`) hitting the collector API.
- [ ] **M6** — Polish: multi-arch CI images, demo GIF against a real multi-AZ EKS cluster,
      CONTRIBUTING.md, Helm repo publishing, blog writeup.

## Known limitations (post-M1)

- **Not yet validated against a live multi-AZ cluster.** All M1 logic is unit tested against fake
  data (conntrack fixtures, fake k8s clientset) but hasn't been run against a real EKS cluster's
  `/proc/net/nf_conntrack` yet. Byte counts depend on `net.netfilter.nf_conntrack_acct=1` being
  set on nodes — most default AMIs do NOT enable this, so real-world validation may surface a need
  to set that sysctl via a DaemonSet init container or node bootstrap config.
- **hostNetwork required.** The agent runs with `hostNetwork: true` to see the node's real
  conntrack table, which has security review implications for locked-down clusters.
- **ReplicaSet→Deployment name resolution is a heuristic** (strips the pod-template-hash suffix),
  not an API lookup — unusual naming conventions could produce a wrong workload label.
- **Same-node pod-to-pod traffic isn't distinguished from same-AZ cross-node traffic** yet; both
  land in the same `same_az_bytes_total` bucket.

## Non-goals (for now)

- Multi-cloud (GCP/Azure) — architecture should stay portable, but AWS is the only implementation.
- Full eBPF — conntrack sampling first; revisit if accuracy/perf demands it.
- Automated remediation (topology spread, pod affinity) — ZoneTax is observability-first.
- Historical cost storage beyond Prometheus retention — no bundled long-term TSDB in MVP.
