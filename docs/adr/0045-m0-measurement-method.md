# ADR 0045: The M0 measurement method

- Status: Accepted — built in the M0 prototype; rule 10: analysis version 7: [ADR 0048](0048-usage-limits-for-subscribers.md)
- Date: 2026-11-08
- Supersedes: [ADR 0035](0035-measurement-corrections.md), [ADR 0036](0036-prices-reconciliation-status.md), [ADR 0037](0037-partial-weeks-prompts-compaction-nudges.md), [ADR 0038](0038-real-size-after-compaction.md), [ADR 0039](0039-rebuilt-cache-and-likely-ranges.md), [ADR 0040](0040-position-pricing-and-one-method-page.md), [ADR 0041](0041-compaction-edges-and-quartile-rule.md), [ADR 0044](0044-effort-thinking-and-cache-figures.md) (kept as history)
- Amends: [ADR 0026](0026-usage-dedup-across-logs.md), [ADR 0027](0027-measured-m0-findings.md), [ADR 0028](0028-harness-recorded-signals.md), [ADR 0030](0030-how-fixes-are-chosen.md), [ADR 0031](0031-m0-order-from-measured-spend.md), [ADR 0032](0032-readable-reports.md), [ADR 0033](0033-reconciliation-history-standing-context.md), [ADR 0034](0034-live-nudges.md), [ADR 0042](0042-p02-p14-and-main-session-models.md), [ADR 0043](0043-weekly-trend-for-new-findings.md)

## Context

The M0 prototype's measurement method was settled over eight ADRs (0035
to 0041 and 0044), one per review round, each amending the ones before.
Finding today's rule for one figure took several hops, and some of those
ADRs recorded bug fixes rather than decisions. This ADR states the method
in force, in one place, so the eight can be read as history.

## Decision

The method in force is below, by topic. [measurements.md](../measurements.md)
gives each rule in full with the code behind it; the measured figures are in
[log-signals.md §8](../log-signals.md#8-measured-figures-at-a-glance).

1. **Counting usage.** Each reply once, by message id, across all logs; within
   a log, its fullest usage line; a reply whose top-level counts are all
   zero is priced from its steps (ADR 0026).
2. **Prices.** Anthropic's pricing page is the source; the catalogue is
   versioned. Where Claude Code's own records price differently, the
   difference shows in reconciliation.
3. **Ranking and savings.** Actions are ranked by their upper bound ("up
   to"). A likely figure is shown where a fix can be replayed on the logs;
   where it rests on a measured size that varies, its range is shown too.
   Quartiles use the median's rule.
4. **What history costs.** Removed history is priced by where it sat in the
   request: read from the cache, or written where the cache was rebuilt.
   Carried tool output and injected text are priced as a write on a turn
   that rebuilt its cache, else as a read; the system prompt and tool
   definitions as a read when anything was read.
5. **Compaction.** The replay covers main sessions, compacting before the
   turn that would pass 200k tokens, at the session's size after the
   previous turn, to the measured size of the first request after a real
   compaction (not the summary's size). A reply logged after a compaction
   marker but timed before it belongs before it; text logged around the
   marker goes with the first reply after; every compaction counts, with
   or without a reply after it.
6. **Who ended a wait.** The user, when a line says so (or, in older logs,
   by elimination), when they typed while Claude worked, answered Claude's
   question, or allowed or refused a tool call; only the yes/no is kept.
   "Start fresh after a long break" counts breaks the user took in main
   sessions.
7. **Cache lifetimes (P02).** The saving is what an expiry cost against the
   cache still being there. The hour's option is priced over every session
   wholly on the 5-minute cache, since the lifetime is chosen before anyone
   knows who will wait. Which sessions used which lifetime is counted from
   the logs. What the hour saved the sessions that used it is a range, its
   ends labelled by what they assume.
8. **Effort (P14).** Each model's xhigh and max turns against that model's
   own high-effort turns, by mean thinking tokens per turn where logged,
   with a baseline of at least 30 turns from 3 sessions. Advice returns to
   the user's usual level and mentions per-model settings.
9. **Reconciliation.** Claude Code's cost record for each main session it
   covers, against the logged cost of that session and its sub-agents. The
   gap is reported; ADR 0028's 2 % test waits for auxiliary turns.
10. **Weekly history.** A week the logs still hold replaces the stored copy,
    except a week Claude Code may have partly deleted, where the stored
    copy stays if it has more turns counted the same way. One history file
    per log folder. Any change to how weekly figures are worked out raises
    the analysis version (7 today, [ADR 0048](0048-usage-limits-for-subscribers.md)).
11. **Live nudges.** After a compaction the status line says "compacted"
    until the next reply; after-reply nudges re-arm for any threshold the
    session falls back under; the cache countdown uses the lifetime of the
    latest reply that wrote to the cache.

How the method changes from now on: a change of method that is a decision
gets a new ADR amending this one; a correction that keeps the decision (a
bug fix, a test, a wording change) goes in the commit message and in
measurements.md, not in a new ADR.

## Consequences

- The eight superseded ADRs keep their context and figures as history;
  their status lines point here.
- [measurements.md](../measurements.md) is the reference for detail;
  this ADR is the reference for what was decided.
