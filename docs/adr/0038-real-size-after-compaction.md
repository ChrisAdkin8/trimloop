# ADR 0038: The real size after a compaction, and who ended a wait

- Status: Accepted — built in the M0 prototype
- Date: 2026-11-08
- Amends: [ADR 0037](0037-partial-weeks-prompts-compaction-nudges.md) (decisions 1–4), [ADR 0035](0035-measurement-corrections.md) and [ADR 0032](0032-readable-reports.md) (the compaction replay), [ADR 0034](0034-live-nudges.md) (status line and re-arming), [ADR 0033](0033-reconciliation-history-standing-context.md) (P07 sources, history file), [data-model.md](../data-model.md) (Turn, Week)

## Context

A fifth review found that ADR 0037 rested on two wrong readings of the
logs, and that its history rule could keep wrong figures:

1. **A compaction's recorded "after" size is only the summary.** The first
   request after each of the 16 compactions with a reply sent 2.3–4.8 times
   that (a median of 80k tokens against 29k): it also carries the system
   prompt, tool definitions and the files, skill and tool listings the
   harness re-attaches. The compaction replay compacted to the summary's
   size, so "compact earlier: likely $463" was overstated by about a third,
   and the status line showed the summary's size after a `/compact`.
2. **Answers to Claude's questions and permission decisions arrive as tool
   results**, so ADR 0037 counted waits on the user (an AskUserQuestion
   answered hours later, a command approved after a break) as waits on a
   tool. Newer logs also say who sent each user line (`origin`), including
   other agents and background tasks; compaction summaries were counted as
   typed; messages typed while Claude worked were missed.
3. **ADR 0037's history rule compared turn counts across analyses** that
   counted turns differently, so an older, inflated copy of the oldest week
   could never be replaced.
4. **The P09 section said "up to $368 could be saved"** next to a line
   saying $72 of it did not apply.

## Decision

1. **The replay compacts to the measured size of the first request after a
   real compaction** (median, `median_resume`; 80k when the logs have
   none). The summary request writes the recorded summary size as output.
   Only main sessions are replayed: the user cannot compact a sub-agent. A
   real compaction leaves nothing for the replay to compact before the next
   turn. Reports show both sizes.
2. **After a `/compact`, the status line says "compacted"** and shows a
   size only after the next reply. Each after-reply nudge re-arms when the
   session falls back under its threshold, not only under 200k.
3. **A wait counts as the user's** when a line says the user sent it
   (`origin` "human"; for older lines without `origin`, a user line that is
   not `isMeta`, not a compaction summary and not a background-task
   notice), when the user typed while Claude worked (`queued_command` from
   "human"), when the user answered an AskUserQuestion, or when the user
   allowed or refused a tool call (`permissionDecision` from "user_…").
   `Turn.Prompted` is this flag; it is the yes/no part of the planned
   `origin` field (data-model.md), which replaces it when built.
4. **P09's saving and its top sessions cover only breaks the user took**;
   its cost still covers every long wait. The weekly re-write figure counts
   only those breaks too (analysis version 3).
5. **History keeps a fuller stored copy of a partly deleted week only when
   both counted turns the same way** (analysis version 2 or later). A
   history file is kept per log folder: runs over `CLAUDE_CONFIG_DIR` use
   their own file.
6. **MCP instructions are sorted per server**, like tool definitions, and
   compactions after a session's last reply are counted.

## Consequences

On the founder's logs (2026-11-08, $2,180 over 44 days):

- Compacting earlier: likely about $314, up to $547 (was $463 and $701).
- Start fresh after a long break: up to $308, likely about $296; 31 waits
  ($60) were not on the user (was 36, $72).
- "Remove MCP servers you don't use" is $2.56: the rest of the instruction
  text came from the app's own servers and a plugin.
- Earlier figures stay in log-signals §8 with this record named.
