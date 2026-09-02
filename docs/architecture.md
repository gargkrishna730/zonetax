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
- Serves the static UI SPA directly (no separate frontend deployment needed for MVP).

### UI (`ui/`)
- Static single-page app, no build step (plain HTML/CSS/JS + a small charting lib, e.g. D3 or a
  lightweight Sankey library) — served directly by the collector binary via `embed.FS`.
- Primary view: live Sankey/chord diagram of $ cost flowing between AZs.
- Secondary view: top-offenders table (namespace/workload ranked by cross-AZ $ spend).

### CLI (`cmd/zonetax-cli`)
- Thin HTTP client against the collector's REST API.
- `zonetax top` — current top cross-AZ cost offenders.
- `zonetax report --since 1h` — cost summary over a time window.

## Data flow

```
conntrack (per node)
  → agent: sample + aggregate + label with pod/AZ metadata
  → Prometheus metrics (raw bytes, no $ )
  → collector: scrape + apply pricing table
  → REST API (JSON, $ cost)
  → UI (Sankey) / CLI (tables)
```

## Why conntrack over eBPF for MVP

eBPF (e.g. via Cilium/Hubble or custom programs) gives more accurate, lower-overhead flow capture,
but requires kernel version constraints, more complex build tooling (clang/libbpf), and is
significantly more code to get right. Conntrack sampling via `/proc/net/nf_conntrack` is:
- Available on any standard Linux node without extra kernel modules in most cases
- Simple to implement and reason about
- "Good enough" for cost visibility (we care about aggregate GB, not per-packet precision)

eBPF is a natural v2 once the core cost-visibility concept is validated with real users.

## Cloud/AZ detection

Rather than querying cloud instance metadata (IMDS) from every agent pod (extra permissions,
extra attack surface, cloud-specific code paths), ZoneTax reads standard Kubernetes node labels:

- `topology.kubernetes.io/zone` — the AZ (e.g. `us-east-1a`)
- `topology.kubernetes.io/region` — the region
- `node.kubernetes.io/instance-type` — informational, for future cost nuance

These are populated by the cloud provider's cloud-controller-manager and are already present on
any properly configured EKS cluster — no extra IAM permissions needed.
