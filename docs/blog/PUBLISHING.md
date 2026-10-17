# Publishing and visibility

Two different problems, often confused:

- **Launch spike** — a burst of attention over 2-3 days. Posts do this. Mostly gone within a week.
- **Sustained discovery** — people finding the project months later because they searched for the
  problem it solves. Directory listings and search do this.

Do both. The listings matter more over time, and almost nobody bothers with them.

---

# Part 1: Sustained discovery (do this first — it keeps working)

## Artifact Hub (highest value, currently missing)

This is where people search for Kubernetes charts. ZoneTax is **not listed**, which is the
single biggest discovery gap. It is free, takes one form, and keeps delivering installs
indefinitely.

1. Sign in at https://artifacthub.io with GitHub
2. Control Panel → Repositories → Add
3. Kind: **Helm charts**, Name: `zonetax`, URL: `https://gargkrishna730.github.io/zonetax`

Artifact Hub re-scans periodically, so new releases appear automatically. The chart already
carries the `artifacthub.io/*` annotations it reads for the listing (license, category, links).

Optional but worth it: claim ownership so the listing shows as verified. Add a
`artifacthub-repo.yml` file at the root of the `gh-pages` branch containing the repository ID
Artifact Hub gives you after adding it.

## GitHub topics

Already set: `kubernetes`, `finops`, `aws`, `eks`, `cost-optimization`, `prometheus`,
`observability`, `golang`, `cloud-cost`, `cross-az`. GitHub's topic pages are browsable, so this
is passive discovery.

## Awesome lists (PRs, not posts)

Each is a one-time pull request that keeps paying off. Read each list's contribution guide and
match its existing entry format exactly:

| List | Why it fits |
|---|---|
| `awesome-kubernetes` | Large, broadly browsed |
| `awesome-finops` | Very on-topic, smaller and easier to get merged |
| `awesome-opencost` / FinOps Foundation resources | Audience is exactly the buyer |
| `awesome-go` | Needs tests + docs, which this has |

Suggested entry text:

```markdown
- [ZoneTax](https://github.com/gargkrishna730/zonetax) - Real-time cross-AZ network cost
  visibility for Kubernetes on AWS. Attributes data-transfer spend to the specific workloads
  generating it.
```

## CNCF Landscape

Submit via PR to `cncf/landscape`. Being on the landscape is a credibility signal that outlasts
any post. Check the current submission requirements first — they ask for a repo that is actively
maintained with a clear license, both of which apply.

---

# Part 2: Launch spike

Accounts you need to create yourself (email verification, ToS acceptance):

| Platform | Notes |
|---|---|
| dev.to | Sign in with GitHub, instant |
| Hacker News | `Show HN` is allowed for your own project |
| r/kubernetes, r/devops, r/aws | Check each subreddit's self-promo rules |
| LinkedIn | Already have it |

## Canonical URL: set this or you lose the SEO

Post the full article in **one** canonical place, then set `canonical_url` on every copy pointing
back to it. Without this, Google treats the copies as duplicate content and may rank none of
them. The repo is the natural canonical home:

```
https://github.com/gargkrishna730/zonetax/blob/main/docs/blog/building-zonetax.md
```

## Order matters

1. **dev.to first.** Low stakes, supports `canonical_url` natively. Watch which sections get
   quoted — that tells you the real hook.
2. **r/kubernetes next**, 1-2 days later. Lead with the cost finding, not the tool.
3. **Hacker News** last, as `Show HN`. Effectively one attempt, so go after the earlier
   audiences have stress-tested the framing. Tue-Thu, ~8-10am ET.
4. **LinkedIn** any time — different audience (recruiters, FinOps and platform leads).

Later, as separate posts rather than reruns: **r/aws** (frame it as the data-transfer bill,
not Kubernetes) and **r/devops** (frame it as the measurement lesson).

## dev.to front matter

```yaml
---
title: "I built a tool to find hidden Kubernetes network costs. Then I found out it was lying to me."
published: true
tags: kubernetes, aws, devops, go
canonical_url: https://github.com/gargkrishna730/zonetax/blob/main/docs/blog/building-zonetax.md
---
```

Then paste the body of `building-zonetax.md` below it, minus the `#` title line.

---

## Hacker News (Show HN)

Title (80 char limit, HN dislikes hype — state the thing plainly):

```
Show HN: ZoneTax – see which Kubernetes pods drive cross-AZ transfer cost
```

First comment, posted immediately after submitting:

```
I built this after noticing cross-AZ traffic is invisible until the bill arrives. AWS charges
~$0.01/GB each direction between AZs, so a GB crossing a zone boundary costs $0.02, and nothing
in the cluster tells you which pod caused it.

A DaemonSet samples conntrack, resolves each flow to pod -> node -> availability zone, and a
collector prices it. On the test cluster I built it against, one route was 97% of cross-AZ
spend: a stateless telemetry collector in one zone writing to its database in another, because
an EBS volume pinned the database and the scheduler put the collector wherever. A nodeSelector
took it from $2.32/day to $0.03/day.

The part I'd actually like feedback on is the measurement, because I got it wrong for weeks. I
originally claimed agreement "within 4% of the AWS bill". That was meaningless: a 2x
double-count (both nodes' agents counted each connection) and a near-total undercount (the
reply direction of each conntrack entry was ignored, so downloads were invisible) happened to
cancel out on that cluster's traffic mix. A plausible total hid two large errors in opposite
directions. Only sending a known 1 GiB between two pods found it: it reported 2.16 GB one
direction and 0.003 GB the other.

Controlled tests are now within 1%, and the remaining +0.7% is TCP/IP header overhead, which is
what AWS bills anyway. Against a full day's real bill it reads substantially low (about a third
of unique cross-AZ GB on a recent day), and that gap is scope rather than error: it only
attributes pod-to-pod traffic, so managed services, load balancers and internet egress are
reported separately as unattributed rather than guessed at — 39% of observed bytes on that day.

Deliberately narrow: AWS only, conntrack not eBPF, no built-in alerting (it exports Prometheus
metrics instead). Reasons for each are in ROADMAP.md. Azure dropped cross-AZ charges in May
2024 so there's nothing to measure there, and GCP bills sender-side only per GiB, which needs a
per-cloud multiplier I can't validate without a GKE cluster.

Apache 2.0: https://github.com/gargkrishna730/zonetax
```

**Expect to be challenged on:** why not eBPF, why not VPC Flow Logs, the hostNetwork/privileged
requirement, and overlap with OpenCost/Kubecost. All four have honest answers in the post and
ROADMAP — use them rather than defending.

---

## r/kubernetes

Title:

```
Found one pod driving 97% of our cross-AZ data transfer cost. Built a tool to make it visible.
```

Body:

```
Cross-AZ traffic is charged ~$0.01/GB in each direction on AWS, and nothing in Kubernetes tells
you which workload is generating it. It shows up days later as "EC2-Other /
DataTransfer-Regional-Bytes" with no pod attribution.

I wrote a small tool that samples conntrack from a DaemonSet, resolves each flow to
pod -> node -> AZ via the standard topology.kubernetes.io labels, and prices it.

The first thing it found on my test cluster: a stateless telemetry collector in one AZ writing
~4.4 GB/hour into its datastore in another. The datastore was pinned to its zone by an EBS
volume; the collector had just landed wherever. One nodeSelector later, $2.32/day -> $0.03/day.

Two things that might be useful to others regardless of the tool:

1. If you run a multi-AZ cluster with a central logging/metrics backend, check where the
   collectors sit relative to the storage. Stateless things that write a lot are the usual
   culprit.
2. Watch for stateful workloads whose volume pins them to a zone while their clients spread
   across all of them.

Repo (Apache 2.0): https://github.com/gargkrishna730/zonetax
Longer writeup, including three counting bugs that made my first accuracy claim worthless:
https://github.com/gargkrishna730/zonetax/blob/main/docs/blog/building-zonetax.md
```

Check the subreddit rules before posting — some require a self-promotion flair or limit how
often you can link your own project.

---

## LinkedIn

Different audience: platform leads, FinOps, hiring managers. Lead with the lesson, not the tech.

```
I spent weeks trusting a number that was wrong.

I built ZoneTax, an open-source tool that shows which Kubernetes workloads are generating
cross-AZ network cost — the line item that only appears days later on the AWS bill with no
indication of which pod caused it.

It worked. On the test cluster I built it against, one route accounted for 97% of cross-AZ
spend: a stateless telemetry collector in one availability zone writing to its database in
another. One nodeSelector took that from $2.32/day to $0.03/day.

Then I validated it properly, and found the numbers had been wrong the whole time.

I had compared the daily total against AWS Cost Explorer, got agreement within 4%, and written
that in the README. But two bugs were cancelling out: every cross-node connection was counted
twice, and the download direction wasn't counted at all. A plausible total was hiding two large
errors in opposite directions.

Sending a known 1 GiB between two pods exposed it immediately: 2.16 GB reported one direction,
0.003 GB the other. Ten bugs in that QA pass. Every one found by measuring real traffic. None
found by reading code, and I had read that code many times.

Three things I took from it:

→ A plausible aggregate is not evidence. It can be two opposite errors.
→ For a measurement tool, "I could not see this" is a feature. Unmeasured nodes and
  unattributable traffic are now reported explicitly, never shown as $0.
→ Narrow and verified beats broad and assumed. I cut multi-cloud, eBPF and built-in alerting,
  and the project got better.

Apache 2.0, free to use: github.com/gargkrishna730/zonetax

#kubernetes #aws #devops #finops #opensource
```

---

## After posting

- Add the published URL to the repo README and your portfolio.
- Reply to every comment in the first few hours; that is what drives ranking on HN and Reddit.
- If someone reports a bug, the measurement lesson applies: ask for a known input, not a
  description.
