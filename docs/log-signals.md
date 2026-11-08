# Claude Code log signals: opportunities and gaps

| | |
|---|---|
| Status | Analysis, 2026-11-07 — its §4 proposals were adopted in [ADR 0028](adr/0028-harness-recorded-signals.md); §7, a second pass measured in money, led to [ADR 0031](adr/0031-m0-order-from-measured-spend.md) |
| Sample | One developer's machine: 598 logs (422 main sessions, 176 sub-agent logs), ~16,000 turns, $2,021 at list prices; Claude Code versions 2.1.288–2.1.318 |
| Method | Structure-only survey (field names, counts, sizes, timings); no prompt or file content was read |
| Builds on | [ADR 0027](adr/0027-measured-m0-findings.md) (measured P19 and P09), the M0 prototype's report, [patterns.md](patterns.md), [analyzer.md §10](analyzer.md#10-extensibility-harnesses-and-providers) |

The question: what do Claude Code's transcripts contain that Trimloop could
turn into optimisation guidance, beyond what the design and the prototype
already use — and where does the design assume something the logs do not
provide?

## 1. What the logs contain

Each `.jsonl` line has a `type`. Counts are from this sample.

| Record type | Lines | What it carries that matters |
|---|---|---|
| `assistant` | 37,917 | `message.usage` (all token classes, `cache_creation` by TTL, `output_tokens_details.thinking_tokens`, `server_tool_use`, `iterations`), `model`, `stop_reason`, content blocks (`tool_use`, `thinking`, `text`); line fields `effort`, `perTurnEffort`, `requestedModel`, `thinkingDurationMs`, `agentId`, `isSidechain` |
| `user` | 22,263 | `tool_result` blocks with `is_error`; `toolUseResult` metadata per tool (file line counts, output sizes, persisted-output paths, structured patches, `userModified`, `staleRecovered`); `turnOrigin`; `toolDenialKind` |
| `api-request` / `api-request-blob` / `api-request-shape` | 10,318 / 11,021 / 505 | The request as sent: `shapeHash` → shape (system prompt and tool list, sizes), `cacheControl` breakpoint paths, `keep` / `base` / `tail` (history editing), `params` (`max_tokens`, `thinking`, `context_management.edits`, `output_config.effort`, `betas`), `promptCacheRequested` (5m / 1h) |
| `attachment` | 26,495 | Context the harness injects: kind, `renderedRole`, rendered bytes — `skill_listing`, `instructions`, `mcp_instructions_delta`, `environment`, `agent_listing_delta`, `deferred_tools_delta`, `sandbox_instructions`, `edited_text_file`, `total_tokens_reminder`, `queued_command`, `budget_usd`; `prompt_snapshot` records the full system prompt and tool list |
| `system` | 2,886 | `subtype`: `turn_duration` (`durationMs`, `messageCount`), `api_error` (`retryAttempt`, `retryInMs`, `source`, `apiError`), `compact_boundary` with `compactMetadata` (`preTokens`, `postTokens`, `cumulativeDroppedTokens`, `durationMs`, `trigger`), `stop_hook_summary` with `hookInfos[].durationMs`, `away_summary` |
| `cost-state` | 467 | **Claude Code's own accounting per session**: `totalCostUSD`, `modelUsage` per model (every token class, `costUSD`, `webSearchRequests`), `totalAPIDuration`, `totalAPIDurationWithoutRetries`, `totalToolDuration`, `totalDuration`, `totalLinesAdded`, `totalLinesRemoved` |
| `queue-operation` | 3,110 | User queued a prompt while the agent was busy (`enqueue` / `dequeue` / `remove`) |
| `tools-used`, `agent-setting`, `agent-name`, `fork-context-ref`, `relocated`, `ai-title`, `custom-title` | small | Session lineage (fork parent, relocation), sub-agent type and name, auto-generated titles |
| `entrypoint` (on most lines) | — | `cli`, `claude-desktop`, `claude-vscode`, `sdk-cli`, `sdk-py` |

Sub-agent logs (`<session>/subagents/agent-*.jsonl`) carry the parent
`sessionId` and the same record types; `agent-*.meta.json` gives `agentType`
and `spawnDepth`. `tool-results/*.txt` holds tool outputs too large for the
log (93 in this sample, median 40 KB); only their sizes are needed.

## 2. What is already used

The design and prototype use: `usage` by token class (de-duplicated per
message id, [ADR 0026](adr/0026-usage-dedup-across-logs.md)), timestamps and
idle gaps, `cache_creation` by TTL, `model`, sub-agent linkage, and — in the
design — tool calls and results, command shapes, compaction boundaries,
harness cost records, injected context and attribution. The prototype now
reports cost by token class, P19, P09, P04, P07, P10 (from compaction
records), P13 for sub-agents, reconciliation and weekly figures; §8 has the
current measured figures. (Updated 2026-11-08; this section first listed
only P19 and P09.)

## 3. Opportunities, ranked by expected value

### 3.1 Reconcile against Claude Code's own cost records

`cost-state.modelUsage` is a per-session ground truth written by the harness
itself. It should become M0's validation oracle and a reported figure.

Measured: summing transcript `usage` per message id matches `cost-state` for
cache reads and cache writes in every session, but **uncached input disagrees
in 369 of 413 sessions** (transcript 0.1 % of the harness figure). The gap is
explained in 91 % of sessions by a **second model in `modelUsage`** — Claude
Haiku, used for title generation and similar auxiliary calls — whose
requests never appear as `assistant` lines (4,886 `ai-title` lines record
only the result). 13.9 M uncached tokens, 0.3 % of cost, invisible to a
transcript reader.

Implemented ([ADR 0033](adr/0033-reconciliation-history-standing-context.md)):
every main session with a record is compared with its logs and sub-agents;
on 412 sessions the harness recorded $52 (3 %) more (corrected by [ADR 0035](adr/0035-measurement-corrections.md) to about 13 % on 409 sessions: records written partway through a session had been compared as complete). The gap is
concentrated in sessions with many away-from-desk summaries and
compactions; the median session differs by 0.8 %.

What to do:

- Read `cost-state` and report **unobserved spend** per session: what the
  harness billed that the transcript does not show.
- Count `ai-title` (and similar) records as `auxiliary` turns with the
  harness's figures, so session kinds (ADR 0012) are complete.
- Make "within 2 % of `cost-state` per token class, auxiliary models
  included" the M0 acceptance test for the reader and price catalogue.
  (2026-11-08: not passed in M0; auxiliary turns are not built, and the
  report shows the gap, about 13 %. See
  [ADR 0036](adr/0036-prices-reconciliation-status.md).) The
  prototype's $2,021 and `cost-state`'s $1,867 cover different session sets;
  per-session reconciliation is the comparison that matters.
- Expect the gap to be a `harness.cost_record` capability that only some
  harnesses provide.

### 3.2 Context composition: what fills a large context (P04, explains P19)

ADR 0027 names this the next M0 step. The logs make it cheap, with no
fingerprints: every `tool_result` has a size and, through `toolUseResult`,
per-tool metadata.

| Tool | Calls | Result bytes (once) | p90 | Notes |
|---|---|---|---|---|
| Bash | 12,964 | 28.5 MB | 5.8 KB | 422 errors; 93 outputs over 30 KB persisted to disk (`persistedOutputPath`, `persistedOutputSize`) |
| Read | 1,642 | 17.2 MB | 29 KB | `numLines` / `totalLines` / `startLine` / `truncatedByTokenCap`: 801 whole-file reads vs 786 ranged; only 9 whole reads of files ≥ 1,000 lines |
| WebFetch | 669 | 4.2 MB | 25 KB | `bytes` fetched up to 7.7 MB, result capped ≈ 48 KB |
| WebSearch | 581 | 1.8 MB | 4.6 KB | |
| Grep | 497 | 1.2 MB | 5.5 KB | `numMatches`, `appliedLimit` show truncation |
| MCP tools | 514 | 1.1 MB | 5.9 KB | p99 34 KB, max 110 KB |

These "once" sizes are what each result costs on arrival; the carried cost is
that size on every later turn of the session (at cache-read price when
cached). P04 should attribute by tool and, for Bash, by the command's shape,
so the L1 fix can name the flag or filter (`--quiet`, `| tail`, test
reporter) rather than say "large output".

### 3.3 Compaction telemetry (P10, and the P19 fix)

`compactMetadata` gives exact numbers: in 18 compactions, `preTokens`
median 538 k (max 975 k), `postTokens` ≈ 20 k, `cumulativeDroppedTokens` up
to 4.26 M, `durationMs` median 28 s (max 127 s), `trigger` auto 17 / manual 1.
Auto-compaction fires only near the context limit; sessions grow from 150 k
to 970 k before it does. This quantifies P19's "compact earlier" advice
(how much a compaction keeps, what it costs in time and tokens) and makes P10
an M0 detector from `system` lines alone.

### 3.4 Effort and thinking (P14) — a direct lever

Every request records `output_config.effort` and every assistant line
`effort` / `perTurnEffort` (`medium`, `high`, `xhigh`, `max`; a per-turn
control beta is enabled). In this sample `max` accounts for 19 % of output
tokens (10.3 M of 55 M) and `xhigh` for 10 %; `thinkingDurationMs` totals
25.6 h (median 2.2 s, p90 15 s, max 455 s). P14 can therefore run in M0 from
usage plus `effort`, and its fix is a setting the harness already exposes.
Thinking *text* is mostly absent from the logs (median thinking block 0
bytes), so `content.local` does not cover reasoning content.

### 3.5 Sub-agent economics

Sub-agents account for 18 % of input tokens and 29 % of output tokens;
`querySource` shows 40 % of requests come from built-in agents. Each
sub-agent starts a cold context and (ADR 0027) uses the 5-minute cache.
`agentType` and `spawnDepth` (meta.json) and `pendingBackgroundAgentCount`
allow a new aggregate: **cost by agent type**, with recommendations on
cache lifetime per agent type and on sub-agent fan-out.

### 3.6 Retries, errors and denials (P12, P05, P11)

`system` / `api_error` lines carry `retryAttempt` (up to 8), `retryInMs`,
`source` (`request_retry` 75, `connection_retry` 39) and `apiError`
(`computer_slept` 28); `cost-state` shows 2.4 h of API time spent in
retries; `streamCutCause: stream_suspended` 37 times. P12 is therefore
measurable in M0 without fingerprints. Tool-level: 422 Bash errors; the
same Bash command repeated immediately 67 times, 18 of them right after an
error (P05 signal); 75 tool denials (`toolDenialKind`: automode, permission
rule, user rejection) — each a turn spent on a call that could not run, a
signal P11 does not yet name.

### 3.7 Harness-injected context (P07 by kind)

Attachments with `renderedRole: system` are text the harness adds to the
model's context. Per kind, total rendered bytes in the sample: `skill_listing`
8.0 MB (26 KB each, 117 sessions), `instructions` 1.1 MB, `mcp_instructions_delta`
1.0 MB, `environment` 0.85 MB, `agent_listing_delta` 0.84 MB,
`deferred_tools_delta` 0.82 MB, `sandbox_instructions` 0.49 MB,
`total_tokens_reminder` 1.0 MB (105 bytes × 9,560 turns). Most are per
session, not per turn, so they matter mainly when a cache is re-written. P07
should attribute standing context to **memory files, system prompt, tool
definitions and injected attachment kinds**, so the fix can say "fewer
skills loaded" or "one MCP server fewer" instead of "trim CLAUDE.md".

`prompt_snapshot` attachments record the full system prompt (median 10.7 KB,
max 85 KB) and tool list (median 13 KB, max 195 KB): with `api-request-shape`
this is `request.full_body` for transcripts.

### 3.8 Exact P01 causes without fingerprints

`api-request.shapeHash` changes only when the system prompt or tool list
changes. Measured: of 10,322 consecutive request pairs, **9,822 kept the same
shape and 66 changed, all by tool-list changes** — the system prompt was
stable within sessions. `cacheControl` gives breakpoint positions (3–4 per
request), `promptCacheRequested` the TTL asked for, and `keep` / `tail` the
history edit. So for Claude Code, P01's churn is rare (ADR 0027: 1.5 %), its
causes are identifiable from the shape record, and the 4-breakpoint
precondition of ADR 0013 can be checked directly. Keyed fingerprints remain
necessary for anything that leaves the machine and for other harnesses;
they are not needed to explain P01 locally.

### 3.9 Time, productivity and attribution (value.md forms 2 and 5)

- **Time:** `cost-state` splits API time (93 h), tool time (64 h), retries
  (2.4 h) and wall time (3,530 h, including idle); `turn_duration` gives
  per-turn wall time (median 43 s, p90 15 min); stop hooks took up to
  637 s per run. Speed findings need no new capture.
- **Productivity:** `totalLinesAdded` / `Removed` (114 k added; $0.03 per
  line at list price; 395 of 467 sessions changed no lines — many are
  sub-agent, research or chat sessions). An outcome signal the design lacks.
- **Interruptions:** users queued a prompt while the agent was busy in 401
  sessions (1,556 queued prompts); `staleReadFileStateHint` 203 and
  `staleRecovered` 11 mark edits that collided with user changes — both
  usable as correction signals (design §6.3).
- **Attribution:** `entrypoint` (CLI $1,579, desktop $165, SDK $121, VS
  Code $3), `turnOrigin` (human vs `sdk`, `task_notification`, `peer`,
  `scheduled`), and `attributionSkill` / `attributionPlugin` /
  `attributionMcpServer` / `attributionAgent` on turns — the data behind
  Claude Code's own `/usage` breakdown. Cost by skill, plugin and MCP server
  is what P06, P07 and P16 recommendations need to name their target.

### 3.10 Smaller signals

- `usage.server_tool_use` (`web_search_requests`, `web_fetch_requests`) and
  `cost-state.webSearchRequests` (471): per-request charges absent from the
  price catalogue; small here (≈ $5) but a catalogue gap.
- `budget_usd` attachments (1,216): SDK sessions run under budgets — a
  natural place for Trimloop's forecasts.
- `fork-context-ref`, `relocated` and resumed logs: session lineage for P09
  and ADR 0026, available directly.
- `stop_reason` (`stop_sequence` 67), `max_tokens` (128 k / 64 k),
  `usage.iterations` (all ≤ 1 so far): worth recording for future
  multi-iteration server loops.
- `requestedModel` vs `model`: no fallbacks observed; keep as a check.

## 4. Gaps in the design that the logs expose

| Gap | Where | Suggested change |
|---|---|---|
| Harness accounting is not a capability | analyzer.md §10.1 | Add `harness.cost_record` (per-session cost and usage from the harness), `harness.injected_context` (attachment kinds with sizes), `request.shape_hash`, `timing.turn_duration`, `session.origin`, `usage.server_tools`, `outcome.lines_changed` |
| Auxiliary calls are invisible in transcripts | data-model.md usage normalisation; ADR 0012 session kinds | Count `ai-title`-style records as `auxiliary` turns using the harness's figures; report unobserved spend; make reconciliation with `cost-state` the reader's acceptance test |
| Turn record lacks fields the logs provide | data-model.md Turn | `effort`, `origin`, `entrypoint`, `requested_model`, `stop_reason`, `shape_hash`, `cache_ttl_requested`, `history_keep`; Session: `agent_type`, `spawn_depth`, `entrypoint` |
| Outcome signals are thin | design.md §6.3 | Add lines changed, tool error rate, denials, queued interruptions, stale-file conflicts, retry time |
| P07 scope | patterns.md P07 | Standing context includes harness-injected attachment kinds; attribute by kind |
| P10, P12 and P14 wait for M3/M4 but need no fingerprints | patterns.md table; ROADMAP M0 | Candidates for M0 in reduced fidelity, like P09 (ADR 0027) — a decision for an ADR |
| Price catalogue has no per-request charges | data-model.md price catalogue | Add server-tool request prices |
| New name-bearing fields | ADR 0011 / 0024 label registry | `agentName`, `customTitle`, `aiTitle`, `attributionSkill/Plugin/McpServer`, skill names in `skill_listing`: key them (`alias`, `mcp`, `tool`; add `skill`, `plugin`) or drop them |
| Thinking content unavailable | analyzer.md `content.local` | State that reasoning text is not in transcripts; P14 uses counts and `effort` only |
| Version dependence | analyzer.md harness profile | All of the above holds for 2.1.288–2.1.318; the conformance kit must check each field's presence per version |

## 5. Suggested order for the next M0 work

Superseded: [ADR 0029](adr/0029-m0-build-order-by-user-value.md) set the
order, and [ADR 0031](adr/0031-m0-order-from-measured-spend.md) re-ranks it
from §7's measurements. Kept as first written:

1. **Reconciliation** with `cost-state`, including auxiliary calls (§3.1) —
   turns the prototype's estimates into verified numbers.
2. **Context composition** by tool from `tool_result` sizes and
   `toolUseResult` metadata (§3.2) — delivers P04 and explains P19.
3. **Compaction and growth** from `compactMetadata` (§3.3) — P10, and
   evidence for P19's fix.
4. **Effort and retries** (§3.4, §3.6) — P14 and P12 from fields already
   parsed; cheap.
5. **Injected context by kind** (§3.7) — P07 with named fixes.
6. **Attribution** by entrypoint, origin, agent type, skill and plugin
   (§3.9) — the team-level view M2 needs, available locally now.

## 6. Caveats

- One developer, one machine, mostly one model (Opus 5.5) with a 1 M
  context; shares will differ elsewhere.
- Counts were taken without reading content; sizes are bytes, with tokens
  estimated at 4 bytes each where stated.
- Field names and record types are as logged by Claude Code 2.1.288–2.1.318
  and may change.

## 7. Second pass: measured in money (2026-11-07)

After the P04 detector shipped, a second structure-only pass priced what the
code still misses, on the same machine (598 logs, 16,314 turns, $2,057 at
list prices). It confirms some §3 opportunities, corrects the size of
others, and adds two. [ADR 0031](adr/0031-m0-order-from-measured-spend.md)
re-ranks the M0 build order from it.

| # | Finding | Measured | What it changes |
|---|---|---|---|
| 1 | **Sub-agents run on the main model.** All 166 Agent calls left `model` unset, so every sub-agent ran on Opus 5.5 (or Fable) | Sub-agents: $529 (26 %). Their Opus 5.5 turns, $471, would cost $235 on Sonnet 5.5 — 11 % of all spend, quality untested | New: a reduced-fidelity P13 for sub-agents in M0 |
| 2 | **Sub-agents use the cache poorly.** Each starts cold and writes with the 5-minute lifetime | Writes are 50 % of sub-agent spend (39 % in main sessions); each written token is read 14 times (46 in main); 28 runs passed 200 k tokens and cost $286 | Part of finding 1; supports §3.5 |
| 3 | **Spend is concentrated.** | Top 1 session 21 %, top 5 47 %, top 10 60 %; median session $0.25 | P19 should lead with concentration |
| 4 | **Compaction comes late or never.** | 18 of 19 compactions automatic, at a median 538 k tokens, down to 20 k in 28 s; 6 sessions passed 500 k and never compacted ($292); a turn over 500 k costs $0.28 against $0.06 under 50 k | P19's fix can name a point to compact; P10 in reduced fidelity |
| 5 | **The harness bills more than transcripts show** — and more than §3.1 found | On 412 sessions with `cost-state`: $1,845 against $1,770 (+4.2 %). Prices match (the harness's tokens at catalogue prices reproduce its cost; 2026-11-08: except Sonnet 5.5 cache reads, which Claude Code prices at $0.20 against the pricing page's $0.10, [ADR 0036](adr/0036-prices-reconciliation-status.md)). The gap is requests with no `assistant` line: ≈ $22 from 277 `away_summary` recaps that re-read the context, $3 of web-search fees (316 requests, not in the catalogue), Haiku helpers, and an unattributed rest. 14 % of spend ($287) is in sessions with no `cost-state` | Reconciliation stays near the top: it is now worth ≈ 4 %, not 0.3 % |
| 6 | **Injected context has a measurable carried cost** | Carried like tool output (P04 method): $88 (4.3 %); `skill_listing` alone $36 (26 KB each) | P07 by kind can report dollars, not only bytes |
| 7 | **Effort is a smaller lever than its token share suggested** | `max` and `xhigh` turns are 22 % of spend but their output costs $109; at `high`'s mean output per turn they would save ≈ $67 (3 %), an upper bound since harder tasks get higher effort | P14 moves down the order |
| 8 | **Spend is attributed on every turn** — `attributionSkill`, `attributionPlugin`, `attributionMcpServer`, `attributionAgent`, and `cwd` | e.g. plugin `checked-plans` $56; MCP `ccd_turn` $31; by project: 22 %, 14 %, 12 %, 11 % for the top four | Cost by skill, plugin, MCP server and project needs no inference |
| 9 | **Some planned signals are small here** | Bash errors 3.3 %; runs of 3+ all-failing turns: 2 ($0.17); identical shell commands repeated: 2 %; retries 1.4 h of 52 h API time; 75 denials | P05, P11 and P12 can wait for more developers' data |

**Implemented** (ADR 0031 items 2–4): findings 1–4 and 8 are now in the
report. The code's figures differ slightly from the table above: the
sub-agent saving is $271 (13 %; 2026-11-07, §8 has the current figure), as it also prices sub-agent turns on Fable
and leaves out forks, which read the main session's cache; concentration is
by conversation (a main session with its sub-agents) — top 1 24 %, top 5
59 %, top 10 75 %; and projects are attributed per turn, with worktrees
under `.claude/worktrees/` counted as the project they were copied from
(32 of 422 main sessions changed folder partway). Compactions are counted
on the turn after them, so a compaction with no later turn is not reported.

Method notes: sessions de-duplicated per message id as the prototype does
(ADR 0026); away-summary cost estimated at the preceding turn's context and
cache-read price; injected-context tokens at 4 bytes each; the Agent tool's
own result is usually a short "started in the background" note, so the size
of what a sub-agent returns could not be measured from it.

## 8. Measured figures at a glance

The same developer's logs were measured several times as the code grew.
Totals differ because the logs grew and the oldest were deleted, and some
methods changed. This table is the reference for figures other documents quote; where they differ, this table is the later measurement.

| Figure | Value | When | Method |
|---|---|---|---|
| Total spend at list prices | $2,003 → $2,021 → $2,057 → $2,134 → $2,149 → $2,166 → $2,180 | 2026-11-06 → 11-08 | Usage counted once per message id, fullest line kept (ADR 0026, 0035); the logs kept growing |
| Spend in turns over 200k tokens (P19) | 57–58 % | 11-06 → 11-08 | Input cost of those turns |
| P19 saving, keeping turns under 200k | 31 % → **19 %** ($415) | 11-06 → 11-08 | First priced at the turn's average input price; corrected to price the removed history at the cache-read price (or write price when the cache was rebuilt) |
| Compact earlier, likely saving | $820–841 → $463 → $462 → **$314** (up to $547) | 11-07 → 11-08 | Replay of compacting at 200k; corrected as above ([ADR 0035](adr/0035-measurement-corrections.md)); then compacting before the turn that passes the limit ([ADR 0037](adr/0037-partial-weeks-prompts-compaction-nudges.md)); then to the measured size of the first request after a compaction (median 80k, not the 22–29k summary) and main sessions only ([ADR 0038](adr/0038-real-size-after-compaction.md)) |
| Size after a compaction | summary median 22–29k; first request after it median 80k (2.3–4.8×) | 11-08 | The harness records only the summary; the next request also sends the system prompt, tools and re-attached files ([ADR 0038](adr/0038-real-size-after-compaction.md)) |
| Re-writes after a break of over an hour (P09) | 17 % ($353–368) | 11-06 → 11-08 | Cache-write cost of the first turn back |
| Of those, waits not on the user | 36 of 137, $72 → **31, $60** | 11-08 | A tool, sub-agent, timer or background task ended the wait; "start fresh" counts only the rest: up to $296 → **$308**, likely about $296 ([ADR 0037](adr/0037-partial-weeks-prompts-compaction-nudges.md); answers to Claude's questions and permission decisions count as the user's, [ADR 0038](adr/0038-real-size-after-compaction.md)) |
| Tool output (P04) | 2.2 % above 8k tokens per result; **12 %** carried in all | 11-06; 11-07 | 2.2 % is the saving from capping each result at 8k tokens (ADR 0027); 12 % is the cost of carrying all tool output; the report now uses a 2k target (about 3 %) |
| What every request carries (P07) | 4.3 % injected text → **11 %** with system prompt and tool definitions | 11-07 | Carried like tool output (ADR 0033) |
| Sub-agents on a cheaper model (P13) | $235 → $271–274 → $281 → $284 → **$287** (13 %) | 11-07 → 11-08 | First Opus 5.5 turns only; then Fable too, forks left out; prices checked against the pricing page ([ADR 0036](adr/0036-prices-reconciliation-status.md)); every Opus and Fable model ([ADR 0037](adr/0037-partial-weeks-prompts-compaction-nudges.md)); the logs kept growing |
| Read only the lines you need (P03/P04) | about $76 of carried cost → card up to **$48** | 11-07 → 11-08 | $76 was the carried cost of file-view output the fix aims at; $48 is the card's upper bound, keeping each file-view result under the 2k-token target |
| Claude Code's records above the logs | 4.2 % → 3 % → 2.1 % → **13 %** ($202, 409 sessions) | 11-07 → 11-08 | $0.18 of it is Claude Code pricing Sonnet 5.5 cache reads at $0.20, where the pricing page says $0.10 (ADR 0036). The earlier figures compared records written partway through a session as if complete (their shortfalls cancelled real gaps); 13 % compares only sessions the records cover ([ADR 0035](adr/0035-measurement-corrections.md)) |
| Re-writes after breaks: earlier quotes | about $314–340 | 11-07 | The first message back after more than an hour, sessions over 200k; the same money as the P09 row |
| Spend in the costliest sessions | top 1: 23–24 %, top 5: 59 %, top 10: 75 % | 11-07 → 11-08 | Main session with its sub-agents |
| Sessions past 500k that never compacted | 6, about $292 | 11-07 → 11-08 | |
| Costliest project | 22 % → 32 % | 11-07 | 22 % used each session's starting folder; 32 % counts each turn's own project, with worktrees under their project |
| Lower effort (P14), at most | about $67 (3 %) | 11-07 | Output of max and xhigh turns priced at high's mean |

