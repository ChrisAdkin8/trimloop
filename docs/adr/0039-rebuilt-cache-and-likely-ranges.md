# ADR 0039: Removed history on a rebuilt cache, and likely savings as a range

- Status: Superseded by [ADR 0045](0045-m0-measurement-method.md) (kept as history). Was: Accepted — built in the M0 prototype; decisions 1, 2 and 5 amended by [ADR 0040](0040-position-pricing-and-one-method-page.md); decision 3 by [ADR 0041](0041-compaction-edges-and-quartile-rule.md)
- Date: 2026-11-08
- Amends: [ADR 0035](0035-measurement-corrections.md) decision 2 (as built), [ADR 0038](0038-real-size-after-compaction.md) (decisions 1 and 3–6), [ADR 0034](0034-live-nudges.md) (status line), [ADR 0033](0033-reconciliation-history-standing-context.md) (P07 rates), [patterns.md P09](../patterns.md#p09--resume-bloat) and [P19](../patterns.md#p19--large-working-context)

## Context

A sixth review found two errors in the compaction and P19 figures that
pulled in opposite directions, and a likely saving too sensitive to report
as one number:

1. **ADR 0035 decision 2 was not built as written.** It prices removed
   history "at the cache-read price, or the cache-write price when the
   cache was rebuilt". The code chose the read price whenever a turn read
   anything; after a pause the system prompt and tools (about 27k tokens)
   are usually still read, so the write branch almost never ran. 169 turns
   wrote more than they read.
2. **A reply was read as the first request after a compaction when it was
   the one that triggered it.** Claude Code can log that reply after the
   compaction marker with an earlier time, and with zero top-level counts
   and only its steps (`iterations`) filled in: it was read as 862 tokens
   and priced without its 965k cache reads. It moved the measured size
   after a compaction from 92k to 80k.
3. **The likely compaction saving rests on a size measured on 15
   compactions.** It moved from $463 to $314 in two rounds; small changes
   in that sample swing it.
4. **Smaller:** refusals of a tool call carry no `permissionDecision`, so
   they were not counted as the user's; pauses in sub-agents counted as
   breaks the user could restart from; weekly compactions left out those
   after a session's last reply; weeks stored before analysis versions
   were not marked as older; a P07 row still said "messages you typed"
   after ADR 0038 moved those out.

## Decision

1. **History after the fixed start is priced as a write when the turn
   wrote more than it read** (the cache was rebuilt), else as a read. The
   fixed start (system prompt, tool definitions) keeps the read price when
   anything was read. This applies to P19, the compaction replay, P04's
   carried cost and P07's injected text.
2. **A reply whose top-level counts are all zero is priced from its steps**,
   even when a cache-write split is left at the top level. A reply logged
   after a compaction but timed before it belongs before it, in the
   session and in the status line. A size after a compaction is measured
   only on main sessions and only when it is smaller than the size before.
3. **The likely compaction saving is shown with a range**: the replay run at
   the lower and upper quartiles of the measured size after a compaction,
   with how many compactions it rests on.
4. **A break the user took** is one in a main session that a message from
   the user ended; refusing a tool call counts as such a message. P09's
   saving, its likely estimate and the weekly re-write figure use that one
   rule.
5. Weekly compactions count those after a session's last reply; the weekly
   tables show the re-writes after breaks and the compactions; weeks with
   no analysis version are marked as older; setting `CLAUDE_CONFIG_DIR` to
   `~/.claude` uses the default history file.

Not changed: approving a call counts as the user's even when the long
wait was the tool running afterwards (the logs do not say when the user
decided), and a connector the app adds under an id is sorted with the
user's MCP servers (its name does not say who added it). Both are small on
these logs.

## Consequences

On the founder's logs (2026-11-08, $2,191 over 44 days):

- P19, keeping turns under 200k: up to $670 (31 %), was $424 (19 %).
- Compacting earlier: likely about $531 ($511–$640), up to $820; it was
  $314 and $547. The first request after a compaction: median 92k (middle
  half 57k–98k, 15 compactions).
- Tool output carried (P04): $339; what every request carries (P07): $262.
- Start fresh after a long break: up to $306, likely about $294; 32 waits
  ($62) were not breaks the user could restart from.
- These savings overlap: they are never added.
