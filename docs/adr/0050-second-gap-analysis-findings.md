# ADR 0050: Findings from the second gap analysis

- Status: Accepted — built in the M0 prototype; amended by [ADR 0052](0052-third-review-corrections.md) (refused requests are "not sent"; unused app servers are P06)
- Date: 2026-11-09
- Amends: [ADR 0045](0045-m0-measurement-method.md) (new figures), [ADR 0042](0042-p02-p14-and-main-session-models.md) (P14 shows the whole thinking share), [ADR 0033](0033-reconciliation-history-standing-context.md) (P07 names unused app servers)

## Context

A pass over the founder's logs (2026-11-09: 571 logs, 45 days, $2,275 at
list prices) looked for what the report misses. The top actions already
cover most avoidable spend. Smaller things were missing:

- **Model switches mid-session** ($10 as decided below; a first count of
  $24 included switches made after the cache had expired anyway): the
  first turn on the new model writes the whole context to the cache again. A cause of cache rebuilds
  (P01), the only one measured from usage alone.
- **Replies cut off by the computer sleeping** (57): Claude Code logs a
  made-up reply, "API Error: Your computer went to sleep mid-response", and
  the request is sent again (P12). The context re-read is at least $4; the
  cut-off output is not logged.
- **App-provided MCP servers never called** ($10 for `claude-artifacts`;
  $15 with the three others the detector finds): P07
  leaves servers the desktop app adds out of its "remove" card, rightly,
  since `claude mcp remove` cannot remove them, so an unused one goes
  unmentioned.
- **What Claude writes stays in the session too**: commands, scripts and
  file contents in tool calls cost about $128 to carry by a first scan
  ($230 as priced below; see log-signals §8) and are not in the
  report's view of what fills a session, which counts tool output only.
- **Thinking is 41 % of output spend** ($174 of $423); P14 shows only the
  part above the usual level.
- **Starting a new session is cheap** (median first turn $0.06), which the
  long-break card can say.
- **When the spend happens**: 28 % at weekends, up to five main sessions at
  once — context for usage limits.
- **Logs are deleted after 30 days** unless `cleanupPeriodDays` is set,
  which limits what trimloop can see.

Unexpected cache rebuilds otherwise ($3) and tool errors (3 % of Bash calls)
were also measured and are too small to report.

## Decision

1. **P01, model switches** (reduced fidelity): a turn on a different model
   from the previous turn of the same session, not after a compaction, with
   the cache still alive (the wait shorter than the previous writes'
   lifetime) and a previous context over 20,000 tokens. Its saving is the
   turn's cost less the same turn reading the earlier context instead of
   writing it. The card: choose the model when the session starts; compact
   before switching.
2. **P12, replies cut off** (reduced fidelity): Claude Code's made-up error
   replies, sorted by their text into "computer slept", "network" and
   "other"; each counted once by line id. The saving is the context of the
   turn before, re-read at its model's cache-read price (a lower bound:
   after a long sleep the cache has often expired too, which P09 counts). The card (when the
   computer slept at least three times): keep it awake while Claude works.
3. **P07, unused app servers**: an MCP server the desktop app adds, whose
   tool definitions were sent but none of whose tools was called in the
   period, gets its own card: in sessions that do not need it, the
   terminal app (`claude`) does not load it.
4. **Tool inputs**: each tool call keeps the size of its input. The text of
   a turn's tool calls is priced as carried from the next turn on, like
   tool output, until a compaction; the report's "what fills a session"
   shows it as its own part, and JSON carries it by tool.
5. **P14 shows the whole thinking share**: thinking cost (output cost times
   the turn's thinking share of its output) against all output cost.
6. **The long-break card** gives the median cost of a new main session's
   first turn.
7. **When you work**: spend by hour and day of the week (local time), the
   weekend share, and the most main sessions running at once. Reported, not
   a card.
8. **Log retention**: the report and `setup --check` read
   `cleanupPeriodDays` from Claude Code's settings file (that key only) and
   suggest 365 when it is unset or under 90.

## Consequences

- Two new cards (P01, P12) and one new P07 card; the P09 card's text and
  the "what fills a session" bar change. Every other figure is unchanged.
- The weekly history is unchanged (no new weekly figures), so the analysis
  version stays at 7.
- Reading `cleanupPeriodDays` adds one key read from Claude Code's settings,
  which `setup` already edits.
