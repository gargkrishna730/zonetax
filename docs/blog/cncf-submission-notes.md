# CNCF blog submission — checklist and cover note

Draft: `cncf-measuring-cross-zone-cost.md` (same directory)

## Where this goes

Submit via the CNCF Marketing Submission Form (linked from the blog guidelines in
`cncf/foundation/policies-guidance/blog-guidelines.md`). The form wants a Google Doc or Word
document, not a markdown file or a link.

Topic ownership sits with **TAG Operational Resilience**, whose scope covers "Resource
Optimization, Cost Efficiency". CNCF restructured eight TAGs into five during 2025; TAG
Observability and TAG Environmental Sustainability were both folded into it, which is why their
old repositories look dormant. TAG Operational Resilience has no blog repository of its own yet,
so the main CNCF blog form is the route.

## Eligibility: read this before submitting

The guidelines list accepted authors as CNCF members, Graduated or Incubating project
maintainers, ambassadors, mentors and mentees, scholarship recipients, and TAGs. An independent
maintainer of a non-CNCF project is not on that list.

The blog does publish "Community post" items — five appear on the current front page — so
community submissions clearly reach publication. But acceptance here is genuinely uncertain, and
that is worth knowing up front rather than being surprised by a decline.

## Guideline compliance

Checked against the published rules:

| Requirement | Status |
|---|---|
| Vendor-neutral, no promotional content | No product mention in the body |
| Affiliation disclosed | Author bio names the tool and its licence |
| Disclaimer included | Present above the bio |
| No criticism of any project | eBPF described as "the most capable option" |
| No links to CNCF sites or forms | None |
| One external link, at the end | One, in the bio |
| Technical and educational value | How-to framing with a worked failure analysis |
| Original content, within two months | Yes, written for this submission |
| Creative Commons Attribution licence | Acceptable |

## If asked about prior publication

The dev.to article is a different piece: it is a project write-up, where this is a technique
write-up. They share underlying measurements but no text. If the reviewer considers that prior
publication, the honest answer is that the findings were previously described in a personal post
and permission to republish is granted.

## Suggested cover note for the form

> This is a technique-focused piece on measuring cross-zone network cost in Kubernetes and
> attributing it to workloads, aimed at the TAG Operational Resilience scope of resource
> optimisation and cost efficiency.
>
> It covers why this cost has no owner in the Kubernetes object model, the trade-offs between
> flow logs, eBPF and conntrack sampling as data sources, four counting mistakes that each
> produce plausible but wrong numbers, and why a cost measurement tool has to report its own
> coverage — because an agent that stops reporting makes spend appear to fall.
>
> The post is vendor-neutral. The measurements come from an Apache 2.0 project I maintain, which
> is disclosed in the author bio and not mentioned in the body.

## Converting to a document

The form needs a Google Doc or Word file:

1. Open a new Google Doc
2. Paste the markdown from `cncf-measuring-cross-zone-cost.md`
3. Apply heading styles to the `##` lines and remove the `#` characters
4. Keep the three code blocks in a monospace font
5. Keep the disclaimer and bio as the final two paragraphs

**Watch the YAML block.** Two lines inside it are YAML comments that begin with `#`:

```
# Only alert on spend while measurement is actually complete.
# Separately, alert when the measurement itself has gaps.
```

Google Docs autoformatting can convert a line starting with `#` into a heading. Those two lines
must stay as plain monospace text inside the code block, otherwise the alert examples become
unreadable and the document looks careless. Check them after pasting.

The document has: 1 title, 6 section headings, 3 code blocks, no tables, and runs about 1,600
words (roughly a 6 minute read).

## Expected timeline

- Response: 7–10 business days, longer around KubeCon
- Publication: 2–3 weeks after acceptance
- CNCF edits the post and proposes a publication date
