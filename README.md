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

## Quickstart

```bash
# Coming soon:
helm repo add zonetax https://gargkrishna730.github.io/zonetax
helm install zonetax zonetax/zonetax -n zonetax --create-namespace
```

## Development

Requires Go 1.22+.

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
