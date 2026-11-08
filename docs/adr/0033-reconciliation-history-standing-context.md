# ADR 0033: Reconciliation, weekly history, and pricing standing context

- Status: Accepted — built in the M0 prototype; amended by [ADR 0035](0035-measurement-corrections.md)
- Date: 2026-11-07
- Amends: [ADR 0028](0028-harness-recorded-signals.md) (how reconciliation is done), [ADR 0029](0029-m0-build-order-by-user-value.md) item 4 (results kept between runs), [patterns.md P07](../patterns.md#p07--bloated-standing-and-injected-context)

## Context

ADR 0031 put three items next: reconciliation with the harness's cost
record, results kept between runs, and P07 priced by carried cost. Building
them raised three questions the earlier ADRs left open.

1. **Which sessions to compare.** Claude Code writes a running cost record
   (`cost-state`) per run of a session. Comparing only sessions whose logs
   show less than the record inflated the gap: a forked session's turns are
   credited to its parent's log (ADR 0026), so the parent looks "behind" and
   the fork looks like pure gap. Records can also be out of date when a
   session is still running.
2. **What to keep between runs.** Claude Code deletes old logs, so a fix's
   "before" disappears within weeks. Keeping whole findings would keep
   names and titles, which ADR 0004 and ADR 0032 keep out of stored data.
3. **How to price the fixed start of a request.** Claude Code records the
   system prompt and tool list (`prompt_snapshot`), but half the records
   omit the tool list, and it records them again without changes.

## Decision

1. **Reconciliation compares every main session with a record**, against
   the cost of that session and its sub-agents in the logs, and reports the
   net difference as spend the logs do not show. A record counts as out of
   date, and the session is left out, only when the session went on more
   than 10 minutes after the record was last written. Each run's latest
   record counts; runs are added. Sessions with no record are reported as
   unverifiable. Web-search requests from the record are priced at the
   catalogue's per-request price (`price.WebSearch`, $10 per 1,000).
2. **Weekly figures are kept between runs** in `~/.trimloop/history.json`
   (`--history FILE` to move it, `--no-history` to neither read nor write
   it): per calendar week (Monday, local time), spend, turns, large-context
   cost, sub-agent spend, re-writes after pauses, carried tool output, file
   views and re-reads, compactions, and the price-catalogue version. Numbers
   only: no names, titles, paths or session ids. A week in the current logs
   replaces the kept copy unless the kept copy has more turns (its logs were
   partly deleted since). The file is written atomically, mode 0600. Saved
   offsets for incremental reading remain to do.
3. **P07 prices what every request carries.** The system prompt and tool
   definitions are charged on every turn at that turn's price for cached
   history, and at its write price when they are new to the session or
   changed. A record without a tool list keeps the last list seen. Text the
   harness adds (attachments with rendered content; memory files by file)
   is carried like tool output (P04) until the session ends or is
   compacted. Each source is labelled with who can change it: skills and
   plugins, MCP servers (tool definitions and instructions), memory files,
   or the harness itself. Fixes are offered only for the first three.

## Consequences

- On the founder's logs: Claude Code recorded $52 (3 %) more than the logs
  (corrected by [ADR 0035](0035-measurement-corrections.md): about 13 %, once
  records written partway through a session are left out)
  show for 412 sessions; 14 % of spend has no record. Standing and injected
  context cost $221 (11 %), of which built-in tool definitions are $102,
  the skill listing $36 and MCP servers $18.
- The report can show a week-by-week table, so the effect of a fix is
  visible; weeks whose logs were deleted come from the history file and are
  marked "kept".
- The history file is new stored data on the developer's machine; it holds
  figures only, consistent with ADR 0004.
- P07's figures overlap P19 and P04 (they are part of what fills the
  conversation) and are never added to them.
