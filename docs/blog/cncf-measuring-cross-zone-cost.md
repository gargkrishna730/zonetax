# Measuring the cross-zone network cost your Kubernetes cluster cannot see

Most Kubernetes cost conversations are about CPU and memory. Those are the numbers surfaced by `kubectl top`, by every dashboard, and by the requests and limits engineers argue over in review.

Network cost is different. On a multi-zone cluster, traffic crossing an availability zone boundary is charged per gigabyte, and on AWS it is charged in *both* directions, so a gigabyte that crosses a zone boundary costs roughly twice the advertised per-direction rate. Nothing inside the cluster reports it. It arrives days later on the bill as a single undifferentiated line item with no workload attribution.

This post is about how to measure that traffic and attribute it to the workloads responsible, and about a measurement problem that turns out to matter more than the collection method.

## Why the cost is invisible

Three properties combine to make cross-zone traffic uniquely hard to see.

**It has no owner in the Kubernetes object model.** A pod has CPU requests, memory limits, and a node assignment. It does not have a "zones I talk to" field. The traffic is a property of a *pair* of workloads and their placement, not of any single object.

**Placement that causes it is usually accidental.** A stateful workload gets pinned to a zone by its persistent volume. A stateless client that talks to it heavily lands wherever the scheduler puts it. Neither decision was wrong in isolation. The cost emerges from the combination, and no admission controller flags it.

**The billing line item aggregates everything.** Cloud providers report regional data transfer as a total. By the time it appears, the pod that generated it may not exist any more.

The result is a cost that is real, continuous, and essentially unattributable with default tooling.

## What you can measure, and where

There are three places to observe this traffic, and they differ in cost and fidelity.

**Flow logs** from the cloud provider give authoritative byte counts, since they come from the same infrastructure that generates the bill. The drawback is that they are themselves billed per gigabyte of logs ingested, and they identify endpoints by IP address. Mapping an IP back to a pod that existed three hours ago requires correlating against historical cluster state, which most setups do not retain.

**eBPF** offers precise, low-overhead flow capture with rich context. It is the most capable option. It also brings kernel version constraints and elevated privileges, which can be a meaningful barrier in locked-down clusters.

**Conntrack** — the kernel's connection tracking table, readable at `/proc/net/nf_conntrack` — is the least glamorous option and often sufficient. The kernel already tracks every connection for NAT purposes, including byte counters in both directions when accounting is enabled:

```bash
sysctl -w net.netfilter.nf_conntrack_acct=1
```

Sampling that table periodically and diffing the counters gives per-connection byte deltas with no packet capture and no kernel programs.

The attribution step is the same regardless of source: resolve each endpoint IP to a pod, each pod to a node, and each node to a zone. The last step needs no cloud API calls, because Kubernetes already standardises it:

```
topology.kubernetes.io/zone
topology.kubernetes.io/region
```

Every managed Kubernetes provider populates these labels. Reading them from the node object avoids per-node instance metadata calls, extra IAM permissions, and provider-specific code paths.

## Four counting mistakes that are easy to make

Having built this, these are the errors worth knowing about in advance. Each produced a plausible-looking number.

**Counting each connection twice.** A connection between pods on different nodes appears in the conntrack table of *both* nodes. If an agent runs per node — which it must, to read a local kernel file — then both agents see the same flow. Without a rule about which side counts, every cross-node byte is counted twice. The fix is arbitrary but must be consistent: attribute the flow to the source pod's node only.

**Ignoring the reply direction.** Conntrack stores an original tuple and a reply tuple, each with its own counters. Reading only the original direction means downloads are nearly invisible: a workload pulling gigabytes from another zone shows almost no traffic, because the bytes flow in the direction you are not reading.

**Treating cumulative counters as deltas at startup.** Conntrack byte counters are cumulative for the lifetime of a connection. An agent that starts and reads a long-lived connection sees its entire history and, with no prior sample to diff against, attributes all of it to the moment it started. The result is an enormous phantom spike on every restart. The fix is to treat the first sample after startup as a baseline that contributes nothing.

**Forgetting that headers are billed.** Conntrack counts bytes on the wire, which includes TCP/IP headers. That makes measured traffic slightly higher than application payload. This is not an error to correct: cloud providers bill wire bytes too.

The first three are worth emphasising because of *how* they were found. Not by code review, which had happened several times. They were found by transferring a known quantity — exactly 1 GiB between two pods in different zones — and comparing the reported figure against the known input.

That test is worth running against any traffic measurement you depend on. An aggregate that looks plausible is not evidence, because two errors in opposite directions can produce a believable total while every individual number is wrong.

## The failure mode specific to cost tools

Measurement systems usually fail loudly. A missing exporter produces a gap in a graph; an alert fires on absent data.

A cost measurement system fails differently. If an agent stops reporting, the measured cost goes *down*. A node whose agent cannot schedule contributes no traffic, and a workload with no traffic is indistinguishable from a workload that is not being watched. The graph shows a number that looks like an improvement.

This is worth stating plainly: **for a cost tool, under-reporting is the dangerous direction, and under-reporting looks like good news.**

The mitigation is to make measurement coverage a first-class signal, reported alongside the cost itself:

- How many nodes exist, and how many reported in the last cycle
- Which specific nodes were not measured
- A single boolean for whether coverage was complete

With that, alerts can be gated on complete measurement rather than firing on partial data:

```yaml
# Only alert on spend while measurement is actually complete.
- alert: CrossZoneSpendHigh
  expr: cross_zone_cost_usd > 1 and coverage_complete == 1
  for: 15m

# Separately, alert when the measurement itself has gaps.
- alert: CoverageIncomplete
  expr: coverage_complete == 0
  for: 15m
```

The second alert is the one that matters. Without it, a dead agent silences the first one and nothing tells you.

The same principle applies to attribution scope. Traffic between a pod and something outside the cluster — a managed database, object storage, a load balancer, the internet — cannot be attributed to a workload pair. It is tempting to distribute it proportionally or quietly omit it. Reporting it separately as unattributed is more useful, because it tells an operator exactly how much of the bill the tool is not explaining.

## Finding the cost in practice

On the cluster this work was validated against, one route accounted for 97% of all cross-zone traffic: a stateless telemetry collector in one zone writing continuously to its datastore in another. The datastore was pinned to its zone by a persistent volume. The collector had been scheduled wherever there was room.

Adding a `nodeSelector` to co-locate the collector with its datastore reduced that route's cost from roughly $2.32 per day to $0.03.

The fix is not interesting. The point is that nothing in the cluster had surfaced the problem, and it had been running for months.

Two patterns are worth checking in any multi-zone cluster:

1. **Centralised logging and metrics backends.** Collectors and agents write continuously and in volume. Check where they sit relative to their storage.
2. **Stateful workloads whose volume pins them to one zone** while their clients are spread across all of them. Every client in a different zone pays per gigabyte.

Where co-location is not acceptable — multi-zone redundancy is usually a deliberate availability decision — the useful output is still knowing the price. Cross-zone replication for a database may be entirely worth its cost. That is a judgement an operator can only make with the number in front of them.

## Summary

Cross-zone network cost is measurable with tools already present on every node. The collection method matters less than people expect; conntrack sampling is sufficient for byte-level accuracy, and the kernel does the hard part.

What matters more is measurement discipline:

- **Validate against a known input.** A plausible total can hide two large errors that cancel.
- **Decide explicitly which side of a flow counts**, or count everything twice.
- **Handle process restarts**, or report spikes that never happened.
- **Report coverage alongside cost.** A silent agent looks like a saving, not a blind spot.
- **Report what you cannot attribute** rather than distributing or hiding it.

That last pair is the part most likely to be skipped, and the part that determines whether anyone should trust the number.

---

*This post represents the viewpoint of its author and does not necessarily reflect an official position of the CNCF or any Technical Advisory Group.*

*Krishna Garg is a DevOps engineer working on Kubernetes platform and cost efficiency. The measurements described here come from building [ZoneTax](https://github.com/gargkrishna730/zonetax), an Apache 2.0 licensed tool for cross-zone cost attribution; the techniques and failure modes apply regardless of the implementation used.*
