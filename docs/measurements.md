# How each figure is worked out

| | |
|---|---|
| Status | Current: describes the M0 prototype as built |
| Last updated | 2026-11-08 |
| Related | [log-signals.md §8](log-signals.md#8-measured-figures-at-a-glance) (the figures, with their history), [patterns.md](patterns.md) (what each pattern is), [data-model.md](data-model.md) (fields) |

This page says how the prototype works out each figure it reports **today**,
which code does it, and which decision set each part. When a method
changes, this page changes with it. Older documents and ADRs quote the
figures of their day; for the current figure see
[log-signals.md §8](log-signals.md#8-measured-figures-at-a-glance), and for the
current method, this page.

Savings overlap (the same tokens can be removed by several fixes), so they
are never added. "Up to" is an upper bound; "likely" replays the logs with
the fix applied.

## 1. Reading and pricing

| What | How it is worked out | Code | Decided in |
|---|---|---|---|
| Which usage counts | Each reply once, by message id, across all logs; within a log, its fullest usage line; a reply whose top-level counts are all zero is priced from its steps (`iterations`) | `cmd/trimloop` `buildSession`, `claudecode` `replyUsage` | ADR 0026, 0035, 0039 |
| Prices | Anthropic's list prices, checked against the pricing page; cache reads at 0.05× input on Opus 5.5 and Sonnet 5.5, 0.025× on Fable 5.1, 0.1× elsewhere | `pkg/price` | ADR 0036 |
| Where a compaction falls | A reply logged after a compaction but timed before it belongs before it; text logged after the marker goes with the next reply | `buildSession` | ADR 0039, 0040 |
| Who ended a wait | The user, when a line says so (`origin` "human"; for older lines, a user line that is not `isMeta`, a compaction summary or a task notice), when they typed while Claude worked, answered Claude's question, or allowed or refused a tool call. Only this yes/no is kept, never the text | `claudecode` `typedPrompt`, `buildSession` | ADR 0037, 0038, 0039 |

## 2. What history costs on a turn

Each request reads a prefix from the cache, writes the next part to it, and
sends the rest uncached. The **fixed start** (system prompt and tool
definitions) comes first, then the history, oldest first, then the turn's
new tokens.

| Rate | Used for | How | Code |
|---|---|---|---|
| Removed history | P19, the compaction replay | Each removed token priced by where it sits: just after the fixed start, read if inside the cached prefix, written if inside the written part (the cache was rebuilt, as after a pause) | `removedCost` |
| Carried history | P04 tool output, P07 injected text | A cache write when the turn wrote more than it read (the cache was rebuilt), else a cache read | `historyRate` |
| Fixed start | P07 system prompt and tool definitions | A cache read when the turn read anything; written again when new or changed | `prefixRate`, `arrivalRate` |

Decided in ADR 0035 (decision 2), 0039 (decision 1) and 0040.

## 3. Findings and fixes

| Figure | How it is worked out | Code | Decided in |
|---|---|---|---|
| **P19** cost and saving | Cost: input cost of turns over 200k tokens. Saving ("up to"): the tokens above 200k on each such turn, as removed history | `FindContextSize` | ADR 0027, 0035, 0040 |
| **Compact earlier**: likely, range, up to | Replays each main session: before the turn that would pass 200k, compact the session (at its size after the previous turn) to the measured size of the first request after a real compaction (median), paying for the summary request and writing the new context. Range: the same replay at that size's lower and upper quartiles. Up to: every affected turn cut to that size, compactions free | `LikelyCompactionSaving`, `Analyse` | ADR 0032, 0035, 0037, 0038, 0039 |
| Size after a compaction | The first request after each real compaction in a main session, when smaller than the size before; the harness's own "after" figure is only the summary | `FindGrowth` (`median_resume`) | ADR 0038, 0039 |
| **P09** cost | Cache writes of the first turn after more than an hour idle | `FindLongPauses` | ADR 0027 |
| **Start fresh**: up to, likely | Only breaks the user took: in a main session, ended by the user (§1). Up to: those writes; likely: as if that turn had started a new session of typical size | `userBreak`, `LikelyPauseSaving`, `ForPauses` | ADR 0032, 0037, 0038, 0039 |
| **P04** carried and fixes | Each tool result carried from the turn it arrives (write price) until the session ends or is compacted (history price); fixes: keeping each result under 2k tokens | `FindToolOutput` | ADR 0027, 0030, 0039 |
| **P07** | The fixed start on every turn (§2), and injected text carried like tool output, sorted by who can change it | `FindStandingContext` | ADR 0033, 0036, 0037, 0038 |
| **P13** | Sub-agent turns priced again on the suggested cheaper model; forks left out | `FindSubAgents` | ADR 0031, 0036, 0037 |
| Reconciliation | Main sessions whose cost records cover them, against their logged cost with sub-agents; the gap is reported, not passed | `FindReconciliation` | ADR 0033, 0035, 0036 |
| Weekly figures | By calendar week, local time; kept in a history file; the oldest weeks keep a fuller stored copy when turns were counted the same way | `FindWeekly`, `history.Merge` | ADR 0033, 0035, 0037, 0038, 0039 |

## 4. Live nudges

| Figure | How it is worked out | Code | Decided in |
|---|---|---|---|
| Session size | The latest reply's context, from the last megabyte of the log; after a `/compact`, "compacted" until the next reply | `ReadTail`, `Status` | ADR 0034, 0037, 0038, 0040 |
| Cache countdown | From the latest reply's time, for the lifetime of the latest reply that wrote to the cache | `CacheLife` | ADR 0034, 0036 |
