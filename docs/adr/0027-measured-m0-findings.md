# ADR 0027: Add P19 large context and bring P09 into M0, from measured sessions

- Status: Proposed
- Date: 2026-11-07
- Amends: [patterns.md](../patterns.md) (P09 milestone and signal, new P19),
  M0 detector list ([ROADMAP.md](../../ROADMAP.md),
  [analyzer.md §15](../analyzer.md#15-build-order)),
  lever ranking in [value.md §2](../value.md#2-the-levers-ranked-by-expected-value)

## Context

[value.md](../value.md) ranks the levers by expected value as a hypothesis,
with fixing broken caching (P01, P02) first. Step 1 of its validation plan
(§8) is to run M0 on the founder's own Claude Code sessions. The M0
prototype now reads transcripts and prices each turn at Anthropic list
prices (catalogue `anthropic-list-2026-11-07`), so that step has been run.

Sample: one developer, 594 sessions (subagent runs counted separately),
about 16,000 turns, $2,003 of spend at list prices, mostly Claude Opus 5.5.

| Candidate | Measured | Likely saving |
|---|---|---|
| Turns sending more than 200k tokens of context | 57% of spend is the input cost of these turns | up to 31% (cost of the tokens above 200k) |
| Turns after more than 1 hour idle, re-writing the cache | 17% of spend | up to 17% |
| P02: turns after 5–60 minutes idle that lost their cached prefix (nearly all in subagents, which use the 5-minute cache) | 6% | below zero: moving subagents to the 1-hour cache costs more than it saves |
| P02: using the 5-minute cache everywhere instead of 1 hour | — | below zero: about −42% |
| P01: cached prefix lost with no pause, excluding compactions | 1.5% | ≤ 1.5% |
| P04: tool results over 8k tokens, excess written once and carried | 2.2% | ≤ 2.2% |
| P14: thinking tokens at `max` / `xhigh` effort | 3% | a fraction of that |

Cost by token class: cache reads 40%, cache writes 41% (1-hour 26%, 5-minute
15%), output 19%, uncached input under 0.1%. The median turn carries about
150k tokens of context; one in ten carries over 600k.

Two findings dominate, and neither is in the M0 detector list. The closest
patterns do not fit: P07 covers only standing context, P08 needs
fingerprints, and P09 (whose fix matches the long-pause finding) is planned
for M3 and triggers only on resumes.

## Decision

- **New pattern P19 — Large working context**, milestone M0, Go, max fix
  level L1. A turn sends more than a threshold of input tokens (default
  200k). Saving: the input cost of the tokens above the threshold, at the
  turn's average input price per token; an upper bound, since some context
  is needed. Fix: L1, start a new session per task or compact earlier.
  Requires only `usage.*`. P04, P07 and P08 explain *why* a context is large,
  and are reported as its causes where they are detected.
- **P09 moves from M3 to M0** in reduced fidelity. Its signal widens from
  "first turn after a resume" to "any turn after idle longer than the
  longest cache lifetime the provider offers, that re-writes the cache". It
  needs `usage.cache_write` and `timing.per_turn`. The fingerprint check
  (prefix differs despite identical content) stays for full fidelity.
- **M0 order of work:** P19 and P09 first (built). Next, the composition of
  context (what fills it: tool output, files read, conversation), which
  delivers P04 and explains P19. Full-fidelity P01 (keyed fingerprints,
  request rebuild) stays in M0 scope but is sequenced after that, until more
  developers have been measured.
- **P02 reports the fit of the cache lifetime**, including "no change
  needed" when the current lifetime is already the cheaper choice.
- **The ranking in value.md stays a hypothesis.** This measurement is
  recorded there, but the levers are re-ranked only after §8 step 2
  (5–10 developers): one heavy user of a 1M-context model is not
  representative.

## Consequences

- Pattern IDs run P01–P19. Every document that lists them is updated.
- M0 can show its two largest findings without fingerprints, keys or the
  store, so the value case can be tested on other developers early.
- P01's fingerprint path is delayed; if other developers show more churn
  (for example, from volatile system prompts), the order changes back.
- P19 and P09 overlap: a turn after a long pause usually also carries a
  large context. Reports must not add their savings together.
- Some P09 pauses are not breaks: a main session also goes idle while a
  subagent runs (9 of 140 such gaps in the sample). The report's advice must
  allow for that.
