# ZoneTax

**See the cross-AZ "tax" your Kubernetes cluster is paying — before it hits your cloud bill.**

Cross-availability-zone (cross-AZ) network traffic is one of the most common silent cost leaks in
Kubernetes clusters. AWS charges ~$0.01/GB in each direction (so effectively $0.02/GB round trip)
for traffic that crosses AZ boundaries — and by the time it shows up in Cost Explorer, it's already
hours or days too late to catch the pod that caused it.

ZoneTax is a lightweight, Kubernetes-native tool that:

- 🔍 **Detects** cross-AZ traffic in real time via an in-cluster DaemonSet agent
- 💰 **Prices it** using a versioned, cloud-specific pricing table (AWS first)
- 📊 **Visualizes** it as a live flow map — which namespaces/workloads are paying the "zone tax"
- 🚨 **Alerts** when spend crosses a threshold (Slack webhook)
- 🖥️ **CLI** for quick `zonetax top` / `zonetax report` checks without opening a dashboard

## Status

🚧 Early development (pre-v0.1). AWS/EKS only for now. See [ROADMAP.md](./ROADMAP.md).

## Does ZoneTax cost anything to run?

Compute-wise: the agent (DaemonSet) and collector are lightweight Go binaries — default requests
are 50m CPU / 64Mi memory per agent pod and 100m CPU / 128Mi memory for the single collector pod
(see [values.yaml](./deploy/helm/zonetax/values.yaml)), well within what most clusters have spare.

Network-wise (the more interesting question, since ZoneTax measures cross-AZ *network* cost):
the agent only reads a local kernel file (`/proc/net/nf_conntrack`) — zero network traffic. The
collector periodically scrapes each agent's `/metrics` endpoint over HTTP, and if a scrape
happens to cross an AZ boundary, that traffic is technically billable like any other cross-AZ
traffic. Measured live against a real 3-AZ EKS cluster: this overhead comes out to
**~$0.0002/day (~$0.007/month)** — about **0.012%** of the real cross-AZ spend the same cluster's
workloads generate. See "How data collection actually works" below for why it stays this small.

## Why not just use Kubecost / OpenCost?

[OpenCost](https://github.com/opencost/opencost) is the standard for k8s cost allocation, but
zone-level network cost attribution is a known, currently-open gap
([opencost#2464](https://github.com/opencost/opencost/issues/2464)). ZoneTax is a focused,
single-purpose tool that does one thing well — cross-AZ cost visibility, live, out of the box —
rather than being a bolt-on feature of a broader FinOps platform. It's not a replacement for
OpenCost; it's a companion tool you can run alongside it.

## How it works

```
┌─────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                 │
│                                                             │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐               │
│  │  Node A   │   │  Node B   │   │  Node C   │  AZ: us-east-1a/b/c
│  │  (AZ-a)   │   │  (AZ-b)   │   │  (AZ-c)   │               │
│  │           │   │           │   │           │               │
│  │ [agent]   │   │ [agent]   │   │ [agent]   │  DaemonSet:   │
│  │ conntrack │   │ conntrack │   │ conntrack │  samples flows│
│  │ sampling  │   │ sampling  │   │ sampling  │  → pod → AZ   │
│  └────┬─────┘   └────┬─────┘   └────┬─────┘               │
│       │              │              │                      │
│       └──────────────┴──────────────┘                      │
│                      │  /metrics (Prometheus)               │
│              ┌───────▼────────┐                             │
│              │   Collector     │  Aggregates AZ-pair bytes   │
│              │  + pricing table│  Applies cloud/region cost  │
│              │  + REST API     │                             │
│              └───────┬────────┘                             │
│                      │                                       │
│         ┌────────────┴────────────┐                         │
│    ┌────▼─────┐              ┌────▼─────┐                   │
│    │   UI      │              │  CLI      │                  │
│    │ (Sankey    │              │ zonetax   │                  │
│    │  flow map) │              │ top/report│                  │
│    └───────────┘              └───────────┘                  │
└─────────────────────────────────────────────────────────┘
```

See [docs/architecture.md](./docs/architecture.md) for details.

### How data collection actually works (and why it's not itself a cost problem)

- The **agent** runs as a DaemonSet (one pod per node) and reads `/proc/net/nf_conntrack` —
  a file the Linux kernel already maintains locally on that node. This is a **local read, not a
  network call** — sampling every 15s costs zero network bytes and zero AWS spend.
- The **collector** (a single pod) periodically scrapes each agent's tiny Prometheus `/metrics`
  endpoint over HTTP (every 30s by default). This step *is* real network traffic, and if the
  collector pod happens to land in a different AZ than an agent pod, that scrape traffic is
  technically billable cross-AZ traffic like anything else.

**How small is that, really?** Measured live against a real 3-AZ EKS cluster over a 28-hour
session: ZoneTax's own collector→agent scrape traffic extrapolates to **~11 MB/day, ~$0.0002/day
(~$0.007/month)** — about **0.012%** of the real ~$1.94/day cross-AZ spend the same cluster's
actual workloads were generating over the same window. It's this small by construction, not luck:
`/metrics` responses are a few KB of plain text (byte counters + labels, nothing payload-heavy),
the scrape cadence is deliberately infrequent (30s, not per-request), and it's one collector
talking to a handful of agents (N pods), not N-to-N traffic.

One honest nuance: ZoneTax doesn't exclude its own traffic from what it reports — you can see
`zonetax-collector → zonetax-agent` show up as its own row in the dashboard if you look. That's
intentional (a cost tool shouldn't have a blind spot for its own footprint), but worth knowing so
you're not confused seeing ZoneTax's own name in its own numbers.

## Quickstart

```bash
# Coming soon:
helm repo add zonetax https://gargkrishna730.github.io/zonetax
helm install zonetax zonetax/zonetax -n zonetax --create-namespace
```

## Development

Requires Go 1.25+.

```bash
git clone https://github.com/gargkrishna730/zonetax.git
cd zonetax
go build ./...
go test ./...
```

## License

[Apache 2.0](./LICENSE)

## Contributing

Contributions welcome once the core M1 milestone (agent traffic capture) lands. See
[ROADMAP.md](./ROADMAP.md) for current status and [CONTRIBUTING.md](./CONTRIBUTING.md).
