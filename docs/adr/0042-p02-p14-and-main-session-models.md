# ADR 0042: P02, P14 and main-session models in M0

- Status: Accepted — built in the M0 prototype; amended by [ADR 0043](0043-weekly-trend-for-new-findings.md) and [ADR 0044](0044-effort-thinking-and-cache-figures.md) (decisions 1 and 3); method summarised and amended by [ADR 0045](0045-m0-measurement-method.md)
- Date: 2026-11-08
- Amends: [ADR 0031](0031-m0-order-from-measured-spend.md) (P13 in M0 covers main sessions too; items 9 and 10 done), [ADR 0029](0029-m0-build-order-by-user-value.md) items 3 (P14) and 7 (P02), and [ROADMAP](../../ROADMAP.md) (all three done), [ADR 0028](0028-harness-recorded-signals.md) (P14 from effort levels built)

## Context

A measurement of what the prototype did not yet analyse in the founder's
logs ($2,212 over 44 days) found, ranked by money missed:

- **Sub-agents losing their cache during waits** ($108): Claude Code's
  sub-agents keep their cache for 5 minutes; 86 waits of 5 minutes to an
  hour made them save their whole history again. P09 looks only at waits
  over an hour.
- **Main-session models** ($106): Fable 5.1 turns would cost $79 less on
  Opus 5.5 (a trade-off), and Opus 5 and Sonnet 5 turns $29 less on their
  newer, cheaper versions with the same tokenizer (no trade-off). P13
  looked only at sub-agents.
- **Effort level** ($77): output above a typical high-effort turn on xhigh
  and max turns. P14 was planned for M0 in reduced fidelity and not built.
- **The 1-hour cache pays off**: the main sessions' 1-hour cache cost
  about $760 less than the 5-minute one would have. Worth saying, so a user
  does not "fix" it.

The rest were small (model switches $11, cache churn $3), unexplained (the
13 % reconciliation gap), or not measurable from the logs.

## Decision

1. **P02 (cache TTL expiry) is built in reduced fidelity**: a turn in a
   session on the 5-minute cache, after a wait of 5 minutes to an hour,
   whose cache had expired. Saving: its cost less what it would have cost
   on the hour's cache (the history read, the new part written). The card
   recommends keeping slow commands out of sub-agents. The same finding
   compares the turns that used the hour with the 5-minute cache.
2. **P13 covers main sessions**: a newer model in the same line that costs
   no more per token and counts tokens the same way (`price.Successor`),
   and, as a quality trade-off, one tier down (`price.LowerTier`: Fable 5.1
   to Opus 5.5). Sub-agents stay with P13's sub-agent card, so the two do
   not overlap.
3. **P14 is built in reduced fidelity**: spend by the effort level each
   request asked for (`effort` on Claude Code's lines, now on each turn),
   and the output above high's mean on xhigh and max turns as an upper
   bound. The card advises keeping the highest levels for hard problems.

## Consequences

On the founder's logs (2026-11-08): P02 up to $101 (84 of 86 waits in
sub-agents); main-session models $29 (newer) and $79 (tier down); P14 up to
$77; the hour's cache $761 cheaper than 5 minutes. Figures in
[log-signals.md §8](../log-signals.md#8-measured-figures-at-a-glance); method
in [measurements.md](../measurements.md).
