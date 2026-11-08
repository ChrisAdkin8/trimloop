# ADR 0029: M0 build order by value to the user

- Status: Accepted — order re-ranked by [ADR 0031](0031-m0-order-from-measured-spend.md); item 4 amended by [ADR 0033](0033-reconciliation-history-standing-context.md)
- Date: 2026-11-07
- Amends: [ADR 0027](0027-measured-m0-findings.md) (M0 order of work)

## Context

The M0 prototype (about 1,900 lines of Go, 22 tests) reads Claude Code
transcripts, de-duplicates usage across logs (ADR 0026), normalises Anthropic
usage, prices it, and reports two findings: P19 (large working context) and
P09 (long pauses, reduced fidelity). Of the twelve M0 detectors, two exist;
there are no command-line options, no stored results, no structured output
and no fixtures.

A comparison of the code with ADRs 0001–0028 ranked the unbuilt work by its
value to a Trimloop user, weighing three things: how much spend a finding is
likely to reveal (measured on the founder's transcripts where possible),
how directly the user can act on it, and how much it affects whether the
numbers are trusted.

## Decision

M0 is built in this order. Each step should ship before the next starts.

| # | Work | Why here |
|---|---|---|
| 1 | **Context composition** — size every tool result and attribute context by tool and command shape; delivers P04 and explains P19 | Turns the largest finding (57 % of spend in turns over 200k tokens) into specific actions; needs tool results, which the reader does not yet parse |
| 2 | **Reconciliation with the harness cost record**, including auxiliary turns (ADR 0028) | Users will compare totals with Claude Code's own figures; a mismatch costs trust before any finding is read. Also the cheapest check on the reader and prices |
| 3 | **P14 in reduced fidelity** — spend by effort level | The most direct lever (a setting the user already has); `max` and `xhigh` produced 29 % of output tokens; data already in parsed lines |
| 4 | **Results kept between runs** — a history of findings with the price-catalogue version, structured (JSON) output, offsets saved so re-runs read only new lines | The core promise is recommend, apply, measure; measuring needs history. A simple history file comes first; the full store (ADRs 0003, 0017) follows when needed |
| 5 | **Command-line options** — `analyse`, `--since`, `--repo`, `--format` | Lets the user ask what one project cost over one period; without filters, reports mix everything |
| 6 | **P07 with injected context by kind** (ADR 0028) | Names the skill, plugin or MCP server to remove; moderate saving, mostly on cache re-writes |
| 7 | **P02 cache-lifetime fit** (ADR 0027) | Guards against a false saving: the measured switch to the 5-minute cache would cost about 42 % more |
| 8 | **P10 and P12 from harness records** (ADR 0028) | Cheap; supports P19's advice and reports retry time; small direct saving |
| 9 | **P01, P03, P05, P06** | Small in the measured sample; full P01 needs fingerprints. Re-ranked once more developers are measured |
| 10 | **Distribution** — an installable release, then the minimal conformance runner and harness profiles | Needed for the 5–10 developers of value.md §8 step 2; the full signed pipeline (ADRs 0008, 0014) can follow |
| 11 | **Keyed fingerprints, key commands, segmentation, redaction, exact hashes** (ADRs 0004, 0011, 0019, 0024, 0025) | Protect stored or uploaded data; the prototype stores and sends nothing, so they add no value until item 9's full-fidelity detectors or M1 need them |

Two engineering changes ride along where they fit, without their own step:
findings record the price-catalogue version (with item 4), and detectors
move to the incremental interface of ADR 0010 before M1, so the same code
runs on live gateway traffic.

## Consequences

- The M0 scope in ROADMAP.md is unchanged; only the order is set.
- Keyed fingerprints move from early to late in M0. M0 still stores and
  sends no content until item 11 is done, so ADR 0004's promise holds.
- The order is re-ranked when value.md §8 step 2 produces measurements from
  other developers.
