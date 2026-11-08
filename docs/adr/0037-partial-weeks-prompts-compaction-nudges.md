# ADR 0037: Partly deleted weeks, breaks the user took, and nudges after a compaction

- Status: Accepted — built in the M0 prototype; decisions 1–4 amended by [ADR 0038](0038-real-size-after-compaction.md)
- Date: 2026-11-08
- Amends: [ADR 0033](0033-reconciliation-history-standing-context.md) and [ADR 0035](0035-measurement-corrections.md) (which stored week wins), [ADR 0036](0036-prices-reconciliation-status.md) decision 3, [ADR 0034](0034-live-nudges.md) (what the nudges read), [ADR 0032](0032-readable-reports.md) (the compaction replay), [patterns.md P09](../patterns.md#p09--resume-bloat) (which breaks the fix counts)

## Context

A fourth review of the M0 prototype found:

1. **History lost data.** ADR 0036 replaced a stored week whenever its
   price list or analysis was older, even when Claude Code had since
   deleted part of that week's logs: each new price list cut down the
   oldest week, which is what the history file is for.
2. **Nudges ignored a compaction.** They read only replies, and Claude Code
   writes no reply for a `/compact`. Until the next reply they showed the
   old size, and held the next message as if the whole session would be
   re-sent: they penalised following their own advice.
3. **A quarter of "long breaks" were not breaks.** 36 of 137 pauses over an
   hour ($72 of $368) ended with no message from the user: the session was
   waiting on a tool, a sub-agent, a timer or a background task. "Start
   fresh after a long break" cannot apply to them.
4. **The compaction replay compacted in the wrong place**: at the turn that
   passed the limit, removing that turn's own new tokens, and priced each
   compaction as reading the limit rather than the session's size.
5. **The text report called compacting earlier "part of P19's saving"**,
   but it is larger: it shrinks a session to a summary, well under P19's
   limit.

## Decision

1. **Only weeks Claude Code may have partly deleted keep a fuller stored
   copy.** Claude Code deletes old logs first, so weeks up to the one
   holding the oldest log still on disk (by last change) can be partial.
   For those, the stored copy stays when it has more turns, whatever price
   list or analysis produced it. Every later week in the logs is complete
   and replaces the stored copy. Reports mark weeks from the history file
   ("kept") and weeks worked out with another price list or an older
   analysis ("older prices or method").
2. **The nudges read a compaction logged after the latest reply.** The
   session's size is then the compaction's "after" size; the status line
   shows no cache countdown and the message is not held, since the next
   message writes the summary to the cache whenever it is sent.
3. **The reader records whether the user sent a message before each turn**
   (a flag; the text is not kept, ADR 0004). Lines Claude Code marks as its
   own (`isMeta`) and background-task notices do not count. P09 still
   reports every long wait, and says how many no message from the user
   started; "Start fresh after a long break" counts only the others, in its
   saving and its likely estimate.
4. **The replay compacts before the turn that would pass the limit**, at the
   session's size after the previous turn; that turn keeps its own new
   tokens, and the compaction is priced as reading that size. The first
   turn of a log is never compacted before, since nothing precedes it.
5. **Compacting earlier is reported as overlapping P19, and possibly
   larger**, not as part of it.

Also in this change, without a decision of its own: a sub-agent's type is
read from the `agent-<id>.meta.json` Claude Code writes beside its log when
the log does not name it; MCP servers the desktop app adds (`Claude_…`,
`visualize`, `terminal` and others) are sorted with the harness; sub-agents
whose main session's log is gone count as unrecorded in reconciliation; and
every Opus and Fable model in the catalogue suggests Sonnet 5.5 for
sub-agents, as ADR 0031 says.

## Consequences

On the founder's logs (2026-11-08, $2,166 over 44 days):

- "Start fresh after a long break": up to $296, likely about $285 (was up
  to $368); P09 itself still reports $368, of which $72 was waits on tools.
- Compacting earlier: likely about $462, up to $700 (was $463 and $701 on
  the same day's logs). Corrected by ADR 0038: about $314, up to $547.
- "Remove MCP servers you don't use" falls from $19 to $5: the rest was
  servers the desktop app adds.
- Running sub-agents on Sonnet 5.5: up to $284 (was $281), now that every
  Opus and Fable model suggests it.
- The history file keeps the oldest week's fuller figures through price
  and analysis changes; the report says when a week's figures are older.
