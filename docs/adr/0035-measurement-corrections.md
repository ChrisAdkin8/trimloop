# ADR 0035: Measurement corrections and how actions are ranked

- Status: Superseded by [ADR 0045](0045-m0-measurement-method.md) (kept as history). Was: Accepted — built in the M0 prototype; amended by [ADR 0036](0036-prices-reconciliation-status.md), [ADR 0037](0037-partial-weeks-prompts-compaction-nudges.md), [ADR 0038](0038-real-size-after-compaction.md), [ADR 0039](0039-rebuilt-cache-and-likely-ranges.md) and [ADR 0040](0040-position-pricing-and-one-method-page.md)
- Date: 2026-11-08
- Amends: [ADR 0026](0026-usage-dedup-across-logs.md) (which line of a message counts), [ADR 0027](0027-measured-m0-findings.md) (how P19's saving is priced), [ADR 0031](0031-m0-order-from-measured-spend.md) (P10 in M0), [ADR 0032](0032-readable-reports.md) (ranking), [ADR 0033](0033-reconciliation-history-standing-context.md) (reconciliation rule, history)

## Context

Two independent reviews of the M0 prototype found that several decisions
had been changed in code without a record, and that three measurements
were wrong:

- **Ranking.** ADR 0032 ranked actions by the likely saving where one
  exists. Only some actions have one, so the ranking compared likely
  estimates with upper bounds. The code was changed to rank by the upper
  bound without a record.
- **P19 and the compaction replay were priced too high.** ADR 0027 priced
  the tokens above the threshold at the turn's average input price, and the
  compaction replay (ADR 0032) did the same. That average includes writing
  the turn's new tokens to the cache, which a smaller context still pays;
  the removed tokens are old history, normally read from the cache.
- **Reconciliation compared incomplete records.** Claude Code's cost
  records carry no timestamp, so ADR 0033's rule for leaving out
  out-of-date records never applied. Records written partway through a
  session were compared as if complete, and their shortfalls cancelled real
  gaps: the reported gap was 2–4 % when it is about 13 %.
- **Output was undercounted.** Claude Code writes one line per content
  block; for some messages an early line carries a partial output count,
  and the first line was kept (0.9 % of output tokens).
- **History kept stale figures.** A week stored by an older version, which
  counted more turns, was never replaced.

## Decision

1. **Actions are ranked by their upper bound**, the one figure every
   recommendation has. Cards still show a likely estimate where there is
   one.
2. **Removed history is priced at what history cost on that turn**: the
   cache-read price, or the cache-write price when the cache was rebuilt.
   This applies to P19's saving and to the compaction replay.
3. **A harness cost record is dated by the last turn logged before it**,
   and a session is compared only when its records cover it: no work more
   than 10 minutes after the last record, and no work before the earliest
   recorded run. Other sessions are reported as left out.
4. **Each message counts its fullest usage line** within a log (repeats
   across logs still count once, ADR 0026). A reply whose top-level usage
   is zero but which lists its steps (`iterations`) is priced from them.
5. **Weekly figures record the analysis version** that produced them; a
   week still in the logs replaces one from an older version.
6. **P10 in M0 is the reduced-fidelity report from compaction records**,
   which is built; ADR 0031's "P10 in full" in M0 is withdrawn. Full
   fidelity stays in M3.

## Consequences

On the founder's logs (2026-11-08, $2,134 over 44 days):

- Compacting earlier: likely about $463, up to $688 (was $841 and $1,146).
- P19's saving: about 19 % (was 31 %). Decision 2 was not built as written
  until ADR 0039. These are the figures of 2026-11-08 before ADRs 0036–0040;
  current figures: [log-signals.md §8](../log-signals.md#8-measured-figures-at-a-glance); current method: [measurements.md](../measurements.md).
- Claude Code's records show about 13 % ($202 on 409 sessions) more than
  the logs, not 2–4 %; 4 sessions are left out as not covered.
- [log-signals.md §8](../log-signals.md#8-measured-figures-at-a-glance)
  holds the corrected figures; earlier documents carry dated notes.
- Decisions changed in code need a record in the same change, as
  CLAUDE.md asks.
