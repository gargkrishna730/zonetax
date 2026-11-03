# Accuracy: ZoneTax vs the AWS bill

> **Correction (2026-10-05, v0.4.0).** End-to-end QA with known traffic found two agent bugs in
> v0.3.x and earlier: every cross-node connection was counted twice (both nodes' agents counted
> it), and the download direction of connections was not counted at all. Which connections were
> affected depended on how they were addressed (direct pod IP traffic was double counted, while
> traffic through a Service was counted once), so the errors partly offset each other on this
> cluster. The within-4% comparison below is therefore **not** reliable evidence of accuracy. Fixed in v0.4.0, where controlled tests
> measure within 1% (see "Controlled test" below). A fresh comparison against the AWS bill on
> v0.4.0 data is in "Full-day comparison" below.

## Controlled test (v0.4.0)

1 GiB (1.0737 GB) sent between two test pods on a real 3-AZ EKS cluster, measured by ZoneTax:

| Test | Reported | Error |
|---|---|---|
| Upload, pod in AZ a to pod in AZ c | 1.0814 GB | +0.7% |
| Download, AZ c streams to AZ a (reply direction) | 1.0780 GB | +0.4% |
| Upload through a ClusterIP Service | 1.0816 GB | +0.7% |
| Same-AZ upload (control) | no cross-AZ entry | correct |

The small positive error is TCP/IP header overhead: conntrack counts bytes on the wire, payload
plus headers, which is also what AWS bills.

The same test on v0.3.1 reported 2.16 GB for the upload (double counted) and 0.003 GB for the
download (reply direction ignored).

## Re-verified with known traffic (v0.7.0, 2026-10-17)

Internal consistency checks (totals matching, no gaps) are necessary but **not sufficient** —
they all passed in v0.3.x while the counting was 2x wrong. So accuracy was re-confirmed the only
way that can fail honestly: by sending a known quantity and comparing.

Two pods were scheduled on nodes in different AZs and exactly 1 GiB (1.0737 GB) was transferred:

| Test | Reported | Error |
|---|---|---|
| 1 GiB, pod in us-east-1c to pod in us-east-1a | 1.0834 GB | **+0.90%** |
| Attribution | `us-east-1c -> us-east-1a`, correct namespace/pod both ends | correct |
| Counted once (not per-node) | single entry of 1.0801 GB, plus 0.0015 GB of return ACKs | correct |
| 1 GiB same-AZ (control, both pods on one node) | +0.0018 GB cross-AZ, +1.1772 GB same-AZ | correct |
| Cross-AZ entries naming the same-AZ pod | 0 | correct |

The +0.90% is TCP/IP header overhead, which is what AWS bills too. The same-AZ control is the
important half: traffic that is not billable must not be costed, and no cross-AZ entry was
created for it.

This rules out the failure modes that the earlier bugs represented — double counting across
nodes, ignoring the reply direction, and billing non-billable traffic — and it also rules out
pod-index resolution having silently degraded, since both endpoints resolved to the correct
namespace and pod name.

## Re-verified after 19 days continuous uptime (v0.7.2, 2026-11-03)

Repeated independently on a 5-node, 3-AZ cluster after the collector had been running
**19.4 days with zero restarts** across all six pods, to check whether accuracy holds after
long uptime rather than only on a freshly started deployment.

| Test | Reported | Error |
|---|---|---|
| 1 GiB, pod in us-east-1a to pod in us-east-1c | 1.0814 GB | **+0.71%** |
| Attribution | `us-east-1a -> us-east-1c`, correct namespace/pod both ends | correct |
| Counted once (not per-node) | single entry of 1.0800 GB, plus 0.0014 GB of return ACKs | correct |
| 1 GiB same-AZ control (two *different* nodes in us-east-1c) | +0.0072 GB cross-AZ, +1.3294 GB same-AZ | correct |
| Cross-AZ entries naming the same-AZ pod | 0 | correct |

The control is stronger than the v0.7.0 one: both pods were on **different nodes in the same
zone**, so the traffic crossed a host boundary without crossing a zone boundary. That is the
case a naive implementation gets wrong, and it still produced zero cross-AZ entries
(the $0.00014 residual is unrelated background traffic in the same window).

Supporting checks over the same run, all against live data:

- Coverage 5/5 nodes measured, `complete: true`, 0 agents failed.
- `sum(entries.cost_usd)` equals `totals.cross_az_cost_usd` to float precision (delta 2.2e-16),
  and every entry satisfies `gb * 0.02 == cost_usd` exactly (74/74).
- History returned **169/169 contiguous hourly buckets** over the 7-day window, all gaps exactly
  1.00 h, timestamps monotonic, **no negative values** (counter-reset bug) and no spikes
  (max/mean 1.5x on organic traffic; the validation hour itself reads 8.6x, which is the 2 GiB of
  injected test traffic showing up exactly where it should).
- `/metrics` route series count equals API entry count (74 = 74), and the per-route series sum
  equals the exported total (delta 1.1e-16).
- Zones reported matched `topology.kubernetes.io/zone` on every node, with **0 entries** where
  `src_zone == dst_zone`.
- All node-level fallback endpoints referenced real node names with the correct zone (4/4).
- Zero error/panic lines in 7 days of logs across all five agents and the collector.

Attribution quality in this run: 73.5% of cross-AZ cost was fully pod-to-pod, 26.5% involved the
node-level fallback for traffic with no pod on one end, consistent with the documented scope.

### Same run, compared against the AWS bill (2026-11-02 UTC)

The known-traffic test above proves counting is right. This compares *coverage* against the bill
on the most recent day where both sides have a complete window.

| | Cross-AZ volume | Cost |
|---|---|---|
| AWS Cost Explorer, this cluster's 5 instances only | 15.352 GB billed = **7.676 GB unique** | **$0.1535** |
| ZoneTax, same UTC day | **4.060 GB** observed | **$0.0812** |
| Difference | ZoneTax sees 52.9% of unique GB | **-47.1%** |

Both sides were checked for comparability before computing the ratio:

- ZoneTax: all **24/24** hourly buckets present, none flagged `complete: false` or `has_data:
  false`, so no partial window is deflating its figure.
- AWS: all **5/5** cluster instances appear in the resource-level bill. A further 8 non-cluster
  resources in the same account also billed inter-AZ transfer (4.85 GB) and were excluded, which
  is why the account-wide figure (20.2 GB) must not be used here.
- The excluded resources were checked rather than assumed: the 6 EC2 instances among them are all
  tagged to a *different* EKS cluster, and `describe-instances` filtered on this cluster's tag
  returns exactly the same 5 IDs that `kubectl get nodes` reports. So no node that was part of the
  cluster on 2026-11-02 was left out of the AWS side (a terminated node would otherwise bill
  without appearing in `kubectl` today).
- All 5 nodes were created well before 2026-11-02, so the node set was stable across the whole
  comparison window.
- The two pricing models agree: AWS's own bill works out to exactly $0.01 per billed GB, which is
  the one-directional half of the $0.02 ZoneTax applies per observed GB. The difference is
  therefore entirely volume, not rate.

The error direction is **under-reporting**, which is the safe direction for a cost tool and
matches the documented scope: ZoneTax attributes pod-to-pod flows and surfaces everything else as
`unattributed_gb` rather than guessing. Coverage here is *better* than the -68% measured on
2026-10-16, so the trend is not a regression.

## Full-day comparison vs the AWS bill (v0.7.0, 2026-10-16 UTC)

The most recent check, on a day where both sides have a complete window and all 5 nodes were
measured (`coverage.complete = true`):

| | Cross-AZ volume | Cost |
|---|---|---|
| AWS Cost Explorer (`EC2: Data Transfer - Inter AZ`) | 10.338 GB billed = **5.169 GB unique** | **$0.1034** |
| ZoneTax, same UTC day | **1.660 GB** observed | **$0.0332** |
| Difference | ZoneTax sees 32% of unique GB | **-68%** |

AWS's GB figure counts each unique GB twice because it bills both the sending and receiving
side; the unique figure is what ZoneTax's one-directional observation is comparable to.

**This gap is scope, not a counting error**, and the tool's own metrics show why. Over the same
period one agent reported:

| | GB |
|---|---|
| Attributed cross-AZ | 0.770 |
| Attributed same-AZ | 3.459 |
| **Unattributed** | **2.733** (39% of all bytes it saw) |

Unattributed traffic has one endpoint ZoneTax cannot resolve to an in-cluster pod: managed
databases, S3 and other AWS services, load balancers, the Kubernetes API server, internet
egress. Some fraction of it crosses an AZ boundary and is therefore on the bill, but ZoneTax
reports it separately rather than guessing how much.

Note the gap *widened* from -45% (2026-10-05) to -68% here, and that is the expected direction:
the large pod-to-pod flow that dominated the earlier measurement was fixed by co-locating the
workload, so what remains is increasingly traffic ZoneTax is not designed to attribute. The
absolute difference is about 7 cents a day.

Internal consistency was checked at the same time: the per-route entries sum exactly to the
reported total (3.4234 GB), `cross_az_gb x effective_price_per_gb` equals the reported cost to
the cent, and the Prometheus route series count matches the API entry count (54 = 54).

## Earlier full-day comparison (v0.4.0, 2026-10-05 UTC)

| | Cost |
|---|---|
| AWS Cost Explorer, this cluster's EC2 instances | **$0.0693** (6.9 GB billed both sides = 3.5 GB unique) |
| ZoneTax, same UTC day, complete window | **$0.0381** (1.9 GB) |
| Difference | **-45%** |

ZoneTax reports about half of what AWS bills on this day. The gap is **not** a counting error
(controlled tests above are within 1%): ZoneTax only attributes **pod-to-pod** traffic. Anything
with an endpoint outside the cluster is skipped: managed databases, S3 and other AWS services,
load balancers, the Kubernetes API server, internet egress, and traffic to a pod the index has
not seen. On a quiet day, when the large pod-to-pod flows have been fixed, that unattributed
traffic dominates what remains, so the percentage gap looks large even though the absolute
difference is a few cents.

Since v0.4.1 the agent exposes `zonetax_agent_unattributed_bytes_total`, and the dashboard shows
it as "N GB outside the cluster, not costed" next to cross-AZ traffic, so this gap is visible
rather than silently missing.

Reading the two numbers:
- **ZoneTax** answers "which of my workloads talk across AZs, and what does that cost" (the part
  you can act on by moving a workload).
- **The AWS bill** is every cross-AZ byte the account produced, including traffic ZoneTax cannot
  attribute to a pod pair.

## Earlier comparison (v0.2.0, superseded)

How close is ZoneTax's cross-AZ cost to what AWS actually charges? Measured on a real 3-AZ EKS
test cluster (us-east-1), October 2026, ZoneTax v0.2.0.

### Result (superseded, see correction above)

| | Cross-AZ GB / day | Cost / day |
|---|---|---|
| AWS Cost Explorer, cluster's EC2 instances only (2026-10-03) | 104.7 GB (209.4 GB billed, both sides) | **$2.094** |
| ZoneTax, 22.9 h continuous window (2026-10-03 18:18 to 10-04 17:10 UTC), scaled to 24 h | 100.8 GB | **$2.017** |
| Difference | | **-3.7%** |

On v0.2.0 ZoneTax landed within 4% of the AWS bill, but with the counting bugs described in the correction above, so this match is not reliable.

## Method

- **AWS side:** Cost Explorer `get-cost-and-usage-with-resources`, usage type
  `DataTransfer-Regional-Bytes` (EC2 cross-AZ transfer), grouped by resource ID, keeping only
  instances tagged with this cluster's `eks:cluster-name`. This excludes NAT gateways, other
  clusters in the same account, and load balancers. AWS bills each cross-AZ GB on both the
  sending and receiving instance, so billed GB / 2 = unique GB crossing a zone boundary.
- **ZoneTax side:** `/api/v1/map?range=24h` total, with history surviving restarts (v0.2.0).
  22.9 of the 24 h were observed, so the total is scaled to 24 h. Every hourly bucket in the
  window was complete, and traffic was steady (4.1 to 4.8 GB/hour).
- AWS hourly granularity is not enabled on this account, so the two windows overlap but are not
  identical (AWS: UTC calendar day; ZoneTax: the trailing 23 h). Day-to-day AWS variation over
  the prior week was $2.06 to $2.29/day (about ±5%), comparable to the gap measured.

## Why ZoneTax reads slightly low

1. One node's agent could not be scheduled (node at memory capacity), so that node's traffic was
   not observed. AWS billed that instance $0.0014 for the day, so this explains very little.
2. Window mismatch (UTC day vs trailing 23 h) at roughly ±5% daily variation.
3. conntrack only sees connections the node tracks. Traffic that bypasses conntrack (for example
   some host-network or NOTRACK flows) is not counted.

## History of this number

| Version | Measured vs AWS | Cause |
|---|---|---|
| pre-fix (Sep 2026) | ~80x over | conntrack byte counters are cumulative, were re-added every sample |
| after deltatrack fix | ~4x over (10-minute sample) | short sample extrapolated, compared to account-wide total |
| v0.2.0 (this page) | **3.7% under** (23 h, cluster-scoped) | proper window, cluster-only AWS figure |

## Reproduce

```bash
# AWS: per-resource cross-AZ cost for one UTC day
aws ce get-cost-and-usage-with-resources --time-period Start=2026-10-03,End=2026-10-04 \
  --granularity DAILY --metrics UnblendedCost UsageQuantity \
  --filter '{"Dimensions":{"Key":"USAGE_TYPE","Values":["DataTransfer-Regional-Bytes"]}}' \
  --group-by Type=DIMENSION,Key=RESOURCE_ID
# keep instance IDs belonging to your cluster (tag eks:cluster-name), sum them

# ZoneTax
zonetax report --range 24h
```
Note: `get-cost-and-usage-with-resources` only covers the last 14 days.

## Case study: fixing the top cost driver (2026-10-04)

ZoneTax showed one route carrying 97% of the test cluster's cross-AZ spend: a telemetry collector
in one AZ writing ~4.4 GB/hour into its database in another AZ (the database's EBS volume pins
it to its zone). The collector is stateless, so it was pinned to the database's zone via its Helm
values (`nodeSelector: topology.kubernetes.io/zone`). No data migration was needed.

| | Cross-AZ GB/hour | Cost/day |
|---|---|---|
| Before (1 h window) | 4.83 | $2.32 |
| After (first 8 min) | 0.06 | $0.03 |

That route disappeared from the map within one scrape interval. The remaining cross-AZ traffic
is the apps in other zones sending telemetry to the collector (a few MB/hour).
