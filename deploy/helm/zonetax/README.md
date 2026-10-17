# ZoneTax

**See the cross-AZ "tax" your Kubernetes cluster is paying, before it hits your cloud bill.**

On AWS, traffic between availability zones costs about $0.01/GB in *each direction*, so a GB
crossing a zone boundary really costs $0.02. Nothing in a cluster tells you it is happening: it
surfaces days later as `EC2-Other / DataTransfer-Regional-Bytes`, with no indication of which pod
caused it.

ZoneTax makes that visible in real time and attributes every dollar to the workload responsible.

## Install

```bash
helm repo add zonetax https://gargkrishna730.github.io/zonetax
helm install zonetax zonetax/zonetax -n zonetax --create-namespace
```

Then open the dashboard:

```bash
kubectl -n zonetax port-forward svc/zonetax-collector 8080:8080
# http://localhost:8080
```

Cloud and region are auto-detected from standard node labels. There is nothing to configure.

## What you get

- An interactive map of which namespaces and workloads talk across zones, priced in dollars
- A REST API (`/api/v1/costs`, `/top`, `/history`, `/map`)
- Costed Prometheus metrics on `/metrics`, including measurement-coverage signals
- 7 days of history, saved across restarts, with downtime reported as gaps rather than as `$0`

## How it works

An agent runs as a DaemonSet on every node, samples `/proc/net/nf_conntrack`, and resolves each
flow to pod, node and availability zone using the standard `topology.kubernetes.io/*` labels. A
single collector scrapes the agents, applies a versioned AWS pricing table, and serves the API,
dashboard and metrics.

The agent reads a local kernel file and generates no network traffic of its own.

## Accuracy

Verified on a real 3-AZ EKS cluster by transferring a known 1 GiB between pods:

| Test | Reported | Error |
|---|---|---|
| Upload across zones | 1.0834 GB | +0.90% |
| Download (reply direction) | 1.0780 GB | +0.4% |
| Through a ClusterIP Service | 1.0816 GB | +0.7% |
| Same-zone control | no cross-AZ entry | correct |

The small positive error is TCP/IP header overhead: conntrack counts bytes on the wire, which is
what AWS bills.

ZoneTax attributes **pod-to-pod** traffic. Anything with an endpoint outside the cluster (managed
databases, S3, load balancers, internet egress) is reported separately as unattributed rather
than guessed at, so its total reads below the full AWS data-transfer bill by design.

## Common configuration

| Value | Default | Notes |
|---|---|---|
| `cloud.provider` / `cloud.region` | `""` | Auto-detected from node labels; set only to override |
| `collector.persistence.enabled` | `false` | `true` gives a 1Gi PVC so history survives pod replacement |
| `agent.priorityClassName` | `""` | Set to `system-node-critical` so the agent can schedule on full nodes |
| `collector.scrapeIntervalSeconds` | `30` | Data-freshness bound, reported in the API |

A node without a running agent is not measured. The API's `coverage` block names unmeasured
nodes and `zonetax_coverage_complete` goes to `0`, so partial measurement is visible rather than
looking like zero traffic.

## Alerting

```yaml
# Alert on spend, but only while measurement is complete.
- alert: CrossAZSpendHigh
  expr: zonetax_cross_az_cost_usd > 1 and zonetax_coverage_complete == 1
  for: 15m
```

## Requirements

- Kubernetes with `topology.kubernetes.io/zone` node labels (EKS sets these)
- The agent needs `hostNetwork` and, by default, a privileged init container to enable conntrack
  byte accounting (`agent.enableConntrackAcct`)
- AWS/EKS. Azure removed cross-AZ transfer charges in May 2024, so there is nothing to measure
  there; GCP is not yet supported because it bills cross-zone traffic differently

## Links

- [Source and full documentation](https://github.com/gargkrishna730/zonetax)
- [Accuracy methodology](https://github.com/gargkrishna730/zonetax/blob/main/docs/accuracy.md)
- [Roadmap and non-goals](https://github.com/gargkrishna730/zonetax/blob/main/ROADMAP.md)

Apache 2.0 licensed.
