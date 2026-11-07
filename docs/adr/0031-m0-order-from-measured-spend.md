# ADR 0031: Re-rank M0 from measured spend; sub-agent model fit in M0

- Status: Proposed
- Date: 2026-11-07
- Amends: [ADR 0029](0029-m0-build-order-by-user-value.md) (M0 build order), [patterns.md](../patterns.md) (P13, P14 milestones)

## Context

ADR 0029 ranked the unbuilt M0 work using token shares and a first survey
of the logs. Its item 1, context composition (P04), has shipped. A second
structure-only pass on the same machine then priced what the code still
misses ([log-signals.md §7](../log-signals.md#7-second-pass-measured-in-money-2026-11-07)).
Four results change the order:

- **Sub-agents run on the main model.** No Agent call set a model; sub-agent
  spend was 26 % of the total, and the same turns would have cost about
  13 % of all spend less on Sonnet 5.5. No planned M0 work reports this:
  P13 (model over-provisioning) waits for a turn classifier in M4.
- **Spend is concentrated and compaction comes late.** The costliest five
  conversations made 59 % of spend; six sessions passed 500 k tokens without
  compacting. P19 says "compact earlier" without saying when.
- **Attribution is free.** Each turn names its project folder and the skill,
  plugin, MCP server or agent type behind it; grouping needs no inference.
- **Effort is worth less than its token share.** P14 was third because
  `max` and `xhigh` produced 29 % of output tokens; priced, the most a lower
  effort could have saved is about $67 (3 %).

Reconciliation with the harness's own cost record matters for trust (the
harness bills 4.2 % more than transcripts show, and 14 % of spend has no
record to check), but saves nothing directly. It is needed before reports
reach other developers (value.md §8 step 2), not before then.

## Decision

**1. P13 for sub-agents in M0, reduced fidelity.** Report sub-agent spend
by agent type and model, and what the same turns would cost on the cheaper
model the price catalogue names (`price.Cheaper`: Sonnet 5.5 for Opus and
Fable models). Agent types that continue the main conversation and read
its cache (Claude Code's `fork`) get no suggestion: on another model they
would lose that cache. The fix (ADR 0030) is `structure` — a custom agent
definition with a `model` field — with `advise` — the Agent tool's `model`
argument — as the alternative. A harness-wide default (Claude Code's
`CLAUDE_CODE_SUBAGENT_MODEL`) is not offered while its precedence over
those two is reported inconsistent. The saving is stated as untested for
quality, with a check: compare the agent type's turns per run and tool
errors after a week. Per-turn routing (op `route_model`, L3) stays in M4.

**2. Concentration and compaction point with P19; P10 in reduced
fidelity.** Report the share of spend in the costliest 1, 5 and 10
conversations (a main session with its sub-agents), the mean cost of a turn
by context size, compactions from the harness's records (manual or
automatic, typical size before and after, longest wait), and sessions past
500 k tokens that never compacted. The fix names the point to compact (the
P19 threshold, 200 k) and the command; its saving is part of P19's and is
not counted again. Claude Code's auto-compaction threshold setting is not
offered while it is reported to be ignored in some versions.

**3. Attribution by project, skill, plugin and MCP server.** Each turn
carries its project (the working folder's name, or for a worktree under
`.claude/worktrees/`, the project it was copied from) and the harness's
attribution fields. A sub-agent with no project takes its main session's.
The M0 report shows names in plain text on the developer's own machine;
anything stored or sent keys them (`repo`, `skill`, `plugin`, `mcp`,
`agent`; ADRs 0011 and 0024).

**4. The M0 build order becomes** (each step ships before the next):

| # | Work | Measured basis | Status |
|---|---|---|---|
| 1 | Context composition (P04) | $251 carried (12 %) | Done |
| 2 | **Sub-agent economics and P13 reduced fidelity** (Decision 1) | ≈ $271 (13 %) | Done |
| 3 | **P19 concentration and compaction point; P10 reduced** (Decision 2) | Top 5 conversations 59 %; $292 in never-compacted sessions past 500 k | Done |
| 4 | **Attribution** by project, skill, plugin and MCP server (Decision 3) | Top project 32 % of spend | Done |
| 5 | **Reconciliation** with the harness cost record: auxiliary and summary requests, web-search fees in the price catalogue | 4.2 % unobserved; 14 % unverifiable | Next; before value.md §8 step 2 |
| 6 | **P07 with injected context by kind**, priced by carried cost | $88 (4.3 %) | |
| 7 | **Results kept between runs** (ADR 0029 item 4) | Needed to measure any fix | |
| 8 | **Command-line options**: `--since`, `--repo`, `--format` | Filters for item 4's groups | |
| 9 | **P14 in reduced fidelity** | ≤ $67 (3 %) | |
| 10 | **P02 cache-lifetime fit** | Guards a false saving (ADR 0027) | |
| 11 | **P03, P06, P01; then P05, P10 in full, P12** | P05, P11, P12 small in this sample (errors 3.3 %, retries 1.4 h of 52 h) | |
| 12 | **Distribution** (ADR 0029 item 10) | — | |
| 13 | **Keyed fingerprints and related work** (ADR 0029 item 11) | — | |

## Consequences

- ROADMAP.md's build order, the P10, P13, P14 and P19 entries in
  patterns.md, the capability list in analyzer.md and the Turn record in
  data-model.md follow this ADR.
- The order rests on one developer's logs. ADR 0029's rule stands: re-rank
  when value.md §8 step 2 brings measurements from other developers.
- A sub-agent model recommendation can lower quality; it is offered with
  its outcome check, never as a pure saving.
- Totals stay unreconciled with the harness until item 5; the report's
  figures may be about 4 % low.
- The price catalogue gains per-request charges for server tools (web
  search) with item 5, as log-signals §4 proposed.
