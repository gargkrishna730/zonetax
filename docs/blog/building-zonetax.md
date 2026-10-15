# I built a tool to find hidden Kubernetes network costs. Then I found out it was lying to me.

Cross-AZ network traffic is one of the quietest line items on a Kubernetes bill. AWS charges
about $0.01/GB in each direction between availability zones, so a GB crossing a zone boundary
really costs $0.02. Nothing in your cluster tells you it is happening. It shows up days later as
"EC2-Other / DataTransfer-Regional-Bytes", with no indication of which pod caused it.

I built [ZoneTax](https://github.com/gargkrishna730/zonetax) to make that visible in real time:
a DaemonSet samples conntrack, resolves each flow to pod → node → availability zone, and a
collector prices it and serves a live map of which workloads are paying the zone tax.

It worked. It found a real and expensive problem on the test cluster within minutes. And for
several weeks, the numbers it reported were wrong in a way I could not see.

This is the part of the build worth writing about.

## The first win

On a 3-AZ EKS test cluster, ZoneTax immediately showed one route carrying **97% of all cross-AZ
spend**: a telemetry collector in one zone writing ~4.4 GB/hour into its database in another.
The database was pinned to its zone by an EBS volume. The collector was stateless and had simply
landed wherever the scheduler put it.

One `nodeSelector` later:

| | Cross-AZ GB/hour | Cost/day |
|---|---|---|
| Before | 4.83 | $2.32 |
| After | 0.06 | $0.03 |

About $70/month, from a one-line change, on a cluster nobody thought had a network cost problem.
That route vanished from the map within one scrape interval.

This is exactly the outcome I wanted. It is also what made me complacent.

## "Within 4% of the AWS bill"

Early on I compared ZoneTax's daily total against AWS Cost Explorer and got agreement within 4%.
I wrote that in the README. It felt like validation: the hard part was done, the numbers were
trustworthy, time to build features.

That number was meaningless.

## What the QA pass actually found

Before calling the project usable, I ran an end-to-end QA pass with **known traffic**: send
exactly 1 GiB between two pods in different zones, then check what ZoneTax reports. Not a code
review. Not a bigger unit test suite. Just a known input and a measured output.

It reported **2.16 GB**.

Every cross-node connection was being counted twice, because the agent on each node saw the same
connection and both counted it. A 2x overcount, on by default, in the core measurement path.

Then I ran the same test in the other direction — a download instead of an upload. It reported
**0.003 GB** for 1 GiB of real traffic. The agent was reading only the original direction of each
conntrack entry and ignoring the reply direction entirely, so downloads were effectively
invisible.

And here is why "within 4%" fooled me. The two bugs pointed in opposite directions. Direct
pod-IP traffic was double counted; traffic through a Service was counted once. Downloads were
missed almost entirely. On that particular cluster, with that particular traffic mix, the
overcount and the undercount roughly cancelled. The total looked plausible. **Every individual
number was wrong.**

A plausible aggregate is not evidence. It can be two large errors in opposite directions, and
you cannot tell the difference without a known input.

## The bug that only appears when something restarts

A third one was subtler. Restart an agent and it would report ~4 GB of cross-AZ traffic in two
minutes — a spike that never happened.

Conntrack counters are cumulative per connection. A freshly started agent reads a connection
that has already transferred 4 GB over the last hour and, having no previous sample to diff
against, treats the whole lifetime total as traffic that occurred since it started.

The fix is one flag: the first sample after startup establishes a baseline and contributes
nothing. 129.8 GB/hour became 0.06 GB/hour.

What makes this one instructive is that it is invisible in steady state. Any test that starts an
agent, sends traffic, and measures will pass. It only fires when a pod restarts during a window
you are measuring — which, in a real cluster, is constantly.

In total that QA pass found **ten** bugs, three of them serious counting errors. All of them were
found by measuring real traffic. **Zero** of them were found by reading code, and I had read that
code many times.

## Then I deleted the accuracy claim

With the counting bugs fixed, controlled tests came in within 1%:

| Test | Reported for 1 GiB (1.0737 GB) | Error |
|---|---|---|
| Upload across zones | 1.0814 GB | +0.7% |
| Download (reply direction) | 1.0780 GB | +0.4% |
| Through a ClusterIP Service | 1.0816 GB | +0.7% |
| Same-zone control | no cross-AZ entry | correct |

That residual +0.7% is not error. Conntrack counts bytes on the wire, headers included, which is
what AWS bills.

But a full-day comparison against the real bill now showed ZoneTax reporting **$0.038 against
AWS's $0.069** — about 45% low. Having just fixed the counting, I knew it was not a counting
problem. ZoneTax attributes **pod-to-pod** traffic. Managed databases, S3, load balancers, the
API server, internet egress: all real cross-AZ bytes, none of them attributable to a pod pair.

So I replaced "within 4% of the AWS bill" with a correction notice explaining that the original
claim was invalid, the controlled results, the 45% gap, and why the gap exists. The tool also now
reports unattributed traffic explicitly in the UI — "N GB outside the cluster, not costed" —
rather than quietly omitting it.

A tool that tells you it can only see part of the picture is more useful than one that implies it
sees everything.

## The bug that made the number confidently wrong

Not long after, someone installed ZoneTax on a `us-west-2` cluster. The dashboard said
`us-east-1`.

The collector had a hard-coded default region. Worse, the environment variable that was supposed
to override it **was never set anywhere in the Helm chart**. I confirmed it by reading the env of
a live pod: the variable simply was not there. Which meant no Helm install had *ever* set the
region. Every deployment, everywhere, had been reporting the same region since the beginning.

The dollar amounts happened to be right, because every AWS region in the pricing table is priced
identically. It would have become real mispricing the instant a differently-priced region was
added.

The fix reads the region from the standard `topology.kubernetes.io/region` node label. The part I
care about more is the fallback: when the region genuinely cannot be determined, ZoneTax now
reports it as **unknown** instead of guessing.

Verifying this taught me something about verification itself. My test cluster is in `us-east-1` —
the same value as the old hard-coded default. "The API says us-east-1" was perfectly consistent
with *detection works* and with *it silently fell back again*. I had to deliberately construct
evidence that could distinguish the two: confirm the constant no longer exists in the source,
confirm the fallback path yields empty rather than a region, confirm detection reports the zone
*list* (which no constant could produce), and push a non-default region end to end.

An observation that cannot fail is not a test.

## The silent half of a cost tool

While fixing that, I found something I consider worse than a wrong number.

One agent pod had been stuck `Pending` for eight days on a node with no spare memory. That node's
traffic was never measured. And the API said nothing about it. A cluster with a dead agent looked
*identical* to a cluster with no cross-AZ traffic.

For a cost tool, that is the most dangerous possible failure: it under-reports, and it looks like
good news. Your spend appears to drop.

So the API now reports measurement coverage — total nodes, measured nodes, and the names of
unmeasured ones — and the Prometheus metrics include a `zonetax_coverage_complete` gauge so
alerts can be gated on complete data:

```yaml
# Only alert on spend while measurement is actually complete.
- alert: CrossAZSpendHigh
  expr: zonetax_cross_az_cost_usd > 1 and zonetax_coverage_complete == 1
  for: 15m

# And separately, tell me when ZoneTax has stopped seeing the whole cluster.
- alert: ZoneTaxCoverageIncomplete
  expr: zonetax_coverage_complete == 0
  for: 15m
```

The second alert fired immediately on my own cluster, correctly, for the node that had been
invisible for eight days. Setting `agent.priorityClassName=system-node-critical` let the
scheduler preempt a low-priority pod, coverage went from 4/5 nodes to 5/5, and the alert cleared.
That was the first time I had end-to-end proof that the signal tracked a real problem *and*
resolved when fixed.

## What I deliberately did not build

The roadmap once had multi-cloud support, eBPF, and Slack alerting on it. All three are now
documented non-goals, and cutting them improved the project.

**Azure.** Microsoft removed cross-AZ data transfer charges in May 2024. There is no cost to
report. Support would mean shipping a feature that returns $0.00 forever.

**GCP.** Technically viable, but it bills differently: sender-side only, and per GiB rather than
per GB. ZoneTax hard-codes a 2x multiplier because *AWS* bills both directions. Shipping GCP
without making that per-cloud would overstate cost by roughly 2x — and I have no GKE cluster to
validate against. Given that every serious bug in this project was caught by measuring real
traffic, shipping an unvalidated cloud would mean guessing at precisely the thing the tool
exists to get right.

**eBPF.** It was on the roadmap as the "proper" v2 implementation. But eBPF's selling point is
accuracy, and conntrack already measures within 1%. The residual is header overhead that AWS
bills anyway, so eBPF would observe the same wire bytes and produce the same number — while
adding kernel version constraints and `CAP_BPF` to an agent whose existing `hostNetwork`
requirement is already the main adoption barrier. It would also mean rewriting the one component
whose correctness was hardest to earn.

**Slack alerting.** This one was embarrassing. `values.yaml` exposed `alerting.slackWebhookUrl`
and a threshold, and the README advertised alerts as a feature. **Nothing in the code read
either value.** An operator could configure a threshold, see no error, and reasonably believe
they were covered.

Before deleting it I checked whether "use Prometheus instead" was actually possible. It wasn't:
the collector's `/metrics` returned 404. Only the agents exposed metrics, and those are raw byte
counters with no cost math — so there was no way to monitor *spend* outside the dashboard.
Deleting the fake feature without building the real one would have left a gap. Now the collector
exports costed metrics, and alerting is delegated to the Prometheus and Alertmanager a
cost-conscious cluster already runs.

## What I'd take from this

**A plausible total proves nothing.** Mine was the sum of a 2x overcount and a near-total
undercount. Use known inputs, not aggregates that look about right.

**Test what a restart does.** Several classes of bug are invisible in steady state and only
appear when a process restarts mid-measurement.

**Make sure your evidence could fail.** If my cluster had been in any region other than
`us-east-1`, I would have caught the region bug in seconds. Instead the coincidence let a broken
default look correct.

**For a measurement tool, "I could not see this" is a feature.** Unmeasured nodes, unattributed
traffic, downtime gaps — all are now reported explicitly. Missing data is never shown as $0 and
never extrapolated. A number you cannot calibrate is worse than an admitted gap.

**Narrow and verified beats broad and assumed.** "Cross-AZ pod-to-pod cost on AWS, verified
within 1% on a real cluster" is a stronger claim than three half-supported clouds and an eBPF
branch.

---

ZoneTax is Apache 2.0 at
[github.com/gargkrishna730/zonetax](https://github.com/gargkrishna730/zonetax). Install with
Helm, port-forward, and you will know within a scrape interval whether you have a zone tax
problem:

```bash
helm repo add zonetax https://gargkrishna730.github.io/zonetax
helm install zonetax zonetax/zonetax -n zonetax --create-namespace
kubectl -n zonetax port-forward svc/zonetax-collector 8080:8080
```

Cloud and region are auto-detected. If it cannot work out your region, it will tell you so
rather than pick one.
