# Accuracy: ZoneTax vs the AWS bill

> **Correction (2026-10-05, v0.4.0).** End-to-end QA with known traffic found two agent bugs in
> v0.3.x and earlier: every cross-node connection was counted twice (both nodes' agents counted
> it), and the download direction of connections was not counted at all. Which connections were
> affected depended on how they were addressed (direct pod IP traffic was double counted, while
> traffic through a Service was counted once), so the errors partly offset each other on this
> cluster. The within-4% comparison below is therefore **not** reliable evidence of accuracy. Fixed in v0.4.0, where controlled tests
> measure within 1% (see "Controlled test" below). A fresh comparison against the AWS bill on
> v0.4.0 data replaces the table below once a full day has been collected.

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
