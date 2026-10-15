# Architecture

## Components

### Agent (`cmd/agent`)
- Go binary, deployed as a Kubernetes DaemonSet (one per node).
- Periodically samples `/proc/net/nf_conntrack` (requires `hostNetwork` + `NET_ADMIN` or reading
  via a mounted `/proc`) to observe active connections and byte counters.
- Maintains a local k8s informer cache (via client-go, scoped with RBAC to `pods`, `nodes`) to
  resolve source/destination IP → Pod → Node.
- Resolves Node → AZ via the `topology.kubernetes.io/zone` label (standard k8s label, populated by
  the AWS cloud-controller-manager on EKS).
- Aggregates bytes transferred per (src AZ, dst AZ, namespace, workload) tuple over a sampling
  window (default 15s).
- Exposes a Prometheus `/metrics` endpoint with raw byte-count metrics — **no cost math in the
  agent**. Keep it dumb and fast.

### Collector (`cmd/collector`)
- Single Deployment (not per-node).
- Scrapes all agent `/metrics` endpoints (or is scraped by Prometheus, ingests via remote_write —
  TBD in M2).
- Loads a versioned pricing table (`internal/pricing`, YAML) keyed by cloud provider + region,
  e.g. AWS us-east-1 cross-AZ = $0.01/GB per direction.
- Computes $ cost per AZ-pair/namespace/workload over rolling windows.
- Serves a REST API (`internal/api`) for the UI and CLI.
- Serves the static UI SPA directly (no separate frontend deployment needed).

### UI (`ui/`)
- Static single-page app, no build step (plain HTML/CSS/JS + a small charting lib, e.g. D3 or a
  lightweight Sankey library) — served directly by the collector binary via `embed.FS`.
- Primary view: live Sankey/chord diagram of $ cost flowing between AZs.
- Secondary view: top-offenders table (namespace/workload ranked by cross-AZ $ spend).

### CLI (`cmd/zonetax`)
- Thin HTTP client against the collector's REST API (`/api/v1/map`, `/api/v1/history`).
- `zonetax top [--range 24h] [-n 10]` — top cross-AZ routes by cost for a window.
- `zonetax report [--range 24h]` — totals, zone-pair breakdown, hourly buckets.

## Data flow

```
conntrack (per node)
  → agent: sample + aggregate + label with pod/AZ metadata
  → Prometheus metrics (raw bytes, no $ )
  → collector: scrape + apply pricing table
  → REST API (JSON, $ cost)
  → UI (Sankey) / CLI (tables)
```

## Why conntrack, not eBPF

ZoneTax samples `/proc/net/nf_conntrack` rather than attaching eBPF programs. This is a settled
decision, not a staging post:

- **The accuracy argument doesn't apply.** eBPF's main draw is more precise flow capture, but
  controlled tests put conntrack sampling within 1% of the bytes actually sent (+0.7% upload,
  +0.4% download, +0.7% through a Service — see [accuracy.md](./accuracy.md)). There is no
  accuracy gap left to close.
- **The residual error isn't error.** That ~0.7% is TCP/IP header overhead: conntrack counts
  bytes on the wire, which is also what AWS bills. eBPF would observe the same wire bytes and
  produce the same figure.
- **It would make deployment harder.** The agent already needs `hostNetwork` and a privileged
  init container, which is the main barrier to adoption in locked-down clusters. eBPF adds
  kernel-version constraints, `CAP_BPF`/`CAP_SYS_ADMIN`, and clang/libbpf build tooling on top.
- **It would discard hard-won correctness.** Every serious counting bug found so far
  (cross-node double counting, uncounted reply-direction traffic, agent-restart spikes) was
  found and fixed in the conntrack path. Rewriting it means re-earning that confidence from
  scratch.

What actually limits ZoneTax's agreement with the AWS bill is **attribution scope** — it costs
pod-to-pod traffic, so anything else is reported separately as `unattributed_gb` rather than
guessed at. eBPF would not change that, because it is a decision about what to attribute, not
about how to observe packets.

## Cloud/AZ detection

Rather than querying cloud instance metadata (IMDS) from every agent pod (extra permissions,
extra attack surface, cloud-specific code paths), ZoneTax reads standard Kubernetes node labels:

- `topology.kubernetes.io/zone` — the AZ (e.g. `us-east-1a`)
- `topology.kubernetes.io/region` — the region
- `node.kubernetes.io/instance-type` — informational, for future cost nuance

These are populated by the cloud provider's cloud-controller-manager and are already present on
any properly configured EKS cluster — no extra IAM permissions needed.
