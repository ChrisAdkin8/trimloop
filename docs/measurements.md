# How each figure is worked out

| | |
|---|---|
| Status | Current: describes the M0 prototype as built |
| Last updated | 2026-11-09 |
| Related | [log-signals.md §8](log-signals.md#8-measured-figures-at-a-glance) (the figures, with their history), [patterns.md](patterns.md) (what each pattern is), [data-model.md](data-model.md) (fields) |

This page says how the prototype works out each figure it reports **today**,
which code does it, and which decision set each part. A second
implementation in Python (`scripts/recount.py`), written from this page,
checks the main figures end to end (`testdata/golden/README.md`). When a method
changes, this page changes with it. Older documents and ADRs quote the
figures of their day; for the current figure see
[log-signals.md §8](log-signals.md#8-measured-figures-at-a-glance), and for the
current method, this page.

Savings overlap (the same tokens can be removed by several fixes), so they
are never added. "Up to" is an upper bound; "likely" replays the logs with
the fix applied.

## 0. Terms

- **Harness**: the program that runs the model for you, here Claude Code.
  It writes the session logs Trimloop reads.
- **Session**: one conversation with its own log. A **main session** is
  the one you talk to; a **sub-agent** is a helper Claude starts for a side
  task, with its own log. A **fork** is a sub-agent that continues the main
  conversation. A main session with its sub-agents is a **conversation**.
- **Context**: everything a request sends: the system prompt, the tool
  definitions, the history so far, and the new message. Every turn sends
  all of it again.
- **Cache**: Anthropic keeps the start of a recent request so the next one
  can reuse it. Reusing it is a **cache read** (cheap: a tenth of the input
  price or less); storing it is a **cache write** (dearer than plain input).
  A stored start lasts 5 minutes or 1 hour (the harness chooses; the hour
  costs more to write). After it expires, the next request writes it all
  again.
- **Compaction**: the harness replaces the history with a short summary,
  when you run `/compact` or on its own near the context limit.
- **Tokens**: the units of text the model is priced by. Where the logs give
  only text (tool output, the system prompt, injected text), Trimloop
  estimates 4 bytes per token.
- **Quartiles**: with the values sorted, the ones a quarter and three
  quarters of the way along; the middle half lies between them.

## 1. Reading and pricing

| What | How it is worked out | Code | Decided in |
|---|---|---|---|
| Which usage counts | Each reply once, by message id, across all logs; within a log, its fullest usage line; a reply whose top-level counts are all zero is priced from its steps (`iterations`) | `cmd/trimloop` `buildSession`, `claudecode` `replyUsage` | ADR 0026, 0035, 0039 |
| Bad lines | A line that is not a JSON object (cut short, not JSON at all, or JSON of another kind, such as a list) is a bad line: counted in the report's reading figures and skipped, and the rest of the log is read. Blank lines are skipped, not counted. A last line with no line ending is unfinished (Claude Code may still be writing it): it is skipped, not counted, and read on a later run once it is complete | `jsonl.Lines`, `cmd/trimloop` `readLog` | — |
| Prices | Anthropic's list prices, checked against the pricing page; cache reads at 0.05× input on Opus 5.5 and Sonnet 5.5, 0.025× on Fable 5.1, 0.1× elsewhere. Fast mode doubles every rate on Opus 5.5, Opus 5 and Opus 4.8 (on other models a fast turn is left unpriced); US-only inference adds 10 % (not on Opus 4.5, Sonnet 4.5 or Haiku 4.5); Haiku 5.5 prompts over 100k input tokens use its long-prompt rates. A model id not in the catalogue as written is looked up again without a cloud provider's additions (a `us.`, `eu.`, `apac.` or `global.` region and `anthropic.` prefix, a Bedrock `-v1:0` version, a Vertex `@20251101` or a `-20251101` date); any other unknown id is left unpriced | `pkg/price` | ADR 0045 (decision 2), 0036 |
| Where a compaction falls | A reply logged after a compaction but timed before it belongs before it; text logged around the marker goes with the first reply after the compaction; compactions no reply followed are kept and counted | `buildSession` | ADR 0038, 0039, 0040, 0041 |
| Spend by project, skill, plugin, MCP server | Each turn's cost credited to the project it ran in, and to the skill, plugin, MCP server and agent type the harness names as its cause. The project is its conversation's (the folder the main session started in; worktrees under `.claude/worktrees` under their project) when the turn ran there, in a subfolder or in a folder above, whether the main session or a sub-agent ran it; another folder's own project otherwise. Other git worktrees count as their own folder: the logs say a folder is a worktree, not which repository it belongs to. Projects are named by their folder's name, with as many parent folders as tell two same-named folders apart (`work/api`, `home/api`; a folder with no parent left keeps its own name) | `FindAttribution` | ADR 0031 |
| Weekly spend in the summary | Total spend divided by the weeks the logs cover (the whole 24-hour spans from the first counted reply to the last, plus one, ÷ 7; prompts and other lines without a reply do not extend it); shown only for a week or more | `period` | ADR 0032 |
| Who ended a wait | The user, when a line says so (`origin` "human"; for older lines, a user line the harness did not mark as its own (`isMeta`) that is not a compaction summary or a task notice), when they typed while Claude worked, answered Claude's question, or allowed or refused a tool call. Only this yes/no is kept, never the text | `claudecode` `typedPrompt`, `buildSession` | ADR 0037, 0038, 0039 |

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
| **Compact earlier**: likely, range, up to | Replays each main session: before the turn that would pass 200k, compact the session (at its size after the previous turn) to the measured size of the first request after a real compaction (median), paying for the summary request and writing the new context. A real compaction resets the replay, and so does a fall of more than half in size with no compaction marker (cleared some other way; none in the founder's logs). The summary request writes the measured median summary (the harness's own "after" figure), or 25k tokens when the logs have no compaction (the founder's median was 22–29k); with no compaction, the size after is 80k. Range: the same replay at that size's lower and upper quartiles, capped at "up to". Up to: every affected turn cut to that size, compactions free. A replay that saves nothing is shown as "likely nothing", not left out. The card's counts (sessions past 200k; sessions past 500k that never compacted) are of main sessions with a priced turn past that size, the ones the replay changes, so they can be lower than "Never compacted" | `LikelyCompactionSaving`, `Analyse`, `compactionCounts` | ADR 0032, 0035, 0037, 0038, 0039 |
| Size after a compaction | The first request after each real compaction in a main session, when smaller than the size before; the harness's own "after" figure is only the summary | `FindGrowth` (`median_resume`) | ADR 0038, 0039 |
| **P09** cost | Cache writes of the first turn after more than an hour idle | `FindLongPauses` | ADR 0027 |
| **Start fresh**: up to, likely | Only breaks the user took: in a main session, ended by the user (§1). Up to: those writes; likely: as if that turn had started a new session of typical size (the median first request of main sessions, `TypicalStart`) | `userBreak`, `LikelyPauseSaving`, `ForPauses` | ADR 0032, 0037, 0038, 0039 |
| Concentration, cost per turn by size | Share of spend in the 1, 5 and 10 costliest conversations; mean cost of a turn up to 50k, 50k–200k, 200k–500k and over 500k tokens (a turn of exactly 200k is not over 200k, as in P19). The web page's headline share of large-turn spend is P19's cost (the input cost of turns over 200k); its share of turns is the bars' (all turns, priced or not) | `FindGrowth` | ADR 0031 |
| Never compacted, longest compaction | Sessions whose context passed 500k with no compaction, and their cost; the longest a compaction took (a median misleads: some are prepared in advance) | `FindGrowth` | ADR 0031, 0035 |
| **P04** carried and fixes | Each tool result carried from the turn it arrives (write price) until the session ends or is compacted (history price); fixes: the saving from keeping each result under 2k tokens, by kind of output | `FindToolOutput`, `ForToolOutput` | ADR 0027, 0030, 0039 |
| P04 details | Re-reads: views of the same part of a file already viewed in the session and not changed since. A part is the Read tool's offset and limit, `sed -n`'s script, or `head`'s and `tail`'s count; the whole file is a part of its own, and a whole view after a partial one (or the reverse) is not counted, since the parts may not overlap. An edit, a compaction, or a shell command that writes the file (`>`, `>>`, `tee`, `sed -i`) resets it; one that may change files it does not name (`git checkout`, `mv`, `cp`, `rm`, a formatter run with its write flag, `perl -i`, adding a dependency) resets every file. A command that views a file and writes in the same call is not a re-read. Filtered: shell commands whose output was already piped through a filter or sent to a file. Saved to disk: results too large for the context, of which the model saw a preview | `FindToolOutput`, `cmdshape` | ADR 0030 |
| **P07** | The fixed start on every turn (§2), and injected text carried like tool output, sorted by who can change it | `FindStandingContext` | ADR 0033, 0036, 0037, 0038 |
| P07 cards | One per source the user can change (skills and plugins, MCP servers, memory files); "up to" is that source's whole cost, as if it were removed | `ForStandingContext` | ADR 0033 |
| **P02** cache expiring during short waits | Turns on the 5-minute cache after a wait of 5 minutes to an hour. Saving: what the expiry cost, against the cache still being there (earlier context read, new part written at the 5-minute price). Also: the net cost of moving every 5-minute session to the hour's cache (all of them, since the lifetime is chosen before anyone knows which will wait), and how many main sessions and sub-agents used each lifetime, which sets the card's advice | `shortWaits`, `FindCacheLife`, `ForCacheLife` | ADR 0042, 0044 |
| The hour against 5 minutes | Turns of sessions on the 1-hour cache, from their first write on the hour, priced again on the 5-minute one: writes at its price, and what was read written again after a wait of 5 minutes to an hour. A range: the low end counts only waits over 10 minutes as losing the cache, the high end every wait over 5. Neither end is exact or a floor: a logged wait includes the next reply's own time, and unlogged requests can keep a cache alive | `FindCacheLife`, `onShort` | ADR 0042, 0044 |
| **P13** main sessions | Main-session turns priced again on the newer model in the same line (no dearer per token, same tokenizer: Opus 5 and 4.8/4.7 to Opus 5.5, Fable 5 to 5.1, Sonnet 5 to 5.5), and, as a trade-off, one tier down (Fable 5.1 to Opus 5.5). Turns are grouped by the catalogue model their id resolves to, so a provider or dated id (`claude-opus-5@20260101`, `us.anthropic.claude-fable-5-1-v1:0`) is the same row and name as the plain id; a model the catalogue holds under both a dated and a plain key (Haiku 4.5: `claude-haiku-4-5-20251001` and `claude-haiku-4-5`, at the same prices) is grouped under the plain key, whichever of the two an id resolves to; the same grouping applies to P14's models, the sub-agent model and the reconciliation's by-model rows; an unknown id stays as written | `FindMainModels`, `ForMainModels` | ADR 0042 |
| **P14** effort | Spend by the effort level each request asked for. Saving: for xhigh and max on each model, mean thinking tokens per turn above the same model's high, times the level's turns, at its output price. It compares means, not single turns (turn by turn would "save" even among high's own turns); means are over turns that record thinking (else output), and a model needs at least 30 high-effort turns from 3 or more sessions. The card advises the level most turns ran at (high, if that is xhigh or max) and names the models, since effort is set per model | `FindEffort`, `ForEffort` | ADR 0028, 0042, 0044 |
| **P13** | Sub-agent turns priced again on the suggested cheaper model; forks left out. Also: the share of sub-agent spend that is cache writes, and how many times each written token is read again, against main sessions; runs whose context passed 200k | `FindSubAgents` | ADR 0031, 0036, 0037 |
| What to do first | Every card saving at least $1, largest "up to" figure (the upper bound) first: every card has one, while only some have a likely figure. A card whose likely figure was worked out and is nothing goes last, saying why in its own words | `Analyse` (`internal/report/results.go`), `Recommendation.Expected` (`pkg/fixes`) | ADR 0035 |
| Reconciliation | Claude Code's own cost record for each main session it covers, against the logged cost of that session and its sub-agents. The difference is reported as spend the logs do not record; ADR 0028's test (within 2 %) is not passed in M0. Web searches are priced at $0.01 each here; they are not in total spend, which comes from the logs | `FindReconciliation` | ADR 0033, 0035, 0036 |
| Weekly figures | By calendar week, local time; kept in a history file; the oldest weeks keep a fuller stored copy when turns were counted the same way. Each week also keeps the P02 short-wait cost, the output cost of xhigh and max turns, and main-session spend with a cheaper choice, so the cards can be checked week by week; and, from analysis version 7, how often a usage limit was reached. Version 8 counts re-reads by the part of a file viewed | `FindWeekly`, `history.Merge` | ADR 0033, 0035, 0037, 0038, 0039, 0043, 0044, 0048, 0053 |
| Compactions by week | A compaction counts in the week of the reply it goes with: the first reply after it, not one logged after it but timed before it (that one triggered it). A compaction no reply followed, including the earlier of two with no reply between them, counts in the week of its own time, or of its session's last reply when it has no time. So a compaction just before midnight on Sunday whose next reply came just after counts in the new week | `buildSession`, `FindWeekly` | ADR 0039, 0041, 0045 |

## 4. Usage limits (subscribers)

For users on a flat-fee plan (Pro or Max), whose usage limits matter more
than dollars ([ADR 0048](adr/0048-usage-limits-for-subscribers.md)).

| Figure | How it is worked out | Code | Decided in |
|---|---|---|---|
| Plan | `organizationType`, `billingType` and the rate-limit tier from Claude Code's account file (`.claude.json` in `CLAUDE_CONFIG_DIR`, else the home folder); nothing else is read from it, and it is not stored. A subscriber is one billed by subscription, or one with limit hits or readings | `claudecode.Reader.Plan`, `Results.Subscriber` | ADR 0048 |
| Limit hits | Assistant lines Claude Code wrote itself with `error: "rate_limit"`, `isApiErrorMessage`, and text naming a usage limit ("You've hit your … limit", "… limit reached", "usage limit") and not a rate limit, in any of those wordings; "session limit" or "5-hour" is the 5-hour limit, "weekly" the weekly one, any other ("Opus limit") "other". Another rate limit (an API key's per-minute limit) is a failed request instead. Counted once by line id; activity for idle times; a session with only a hit is kept, and `--since` places it by its first hit | `parseLine`, `FindLimits` | ADRs 0048, 0052 |
| Limit readings | The status line's `rate_limits.five_hour` and `.seven_day` (used percentage, reset time), appended to `~/.trimloop/limits.jsonl` when either changes | `recordLimits`, `history.AppendLimits` | ADR 0048 |
| Weekly limit per dollar | In each weekly window (readings with the same reset time, to the minute), the rise from the first reading to the last, against the list-price spend logged between them; summed over windows. Shown once the windows cover $20 and 5 points. A stored limit whose reset time is more than 8 days after its reading, or more than a day before it (lines 0.1.0 stored unchecked), is left out. Overstated when Claude chat or another machine also used the limit. With `--since`, sessions started earlier are left out, so their spend is too | `FindLimits` | ADR 0048 |
| Share of the weekly limit, per action | The likely saving when the card has one (even when it is nothing), else the upper bound, per week of the period, times the weekly limit per dollar, capped at 100 % | `addLimits` | ADR 0048 |

## 5. From the second gap analysis

Added by [ADR 0050](adr/0050-second-gap-analysis-findings.md).

| Figure | How it is worked out | Code | Decided in |
|---|---|---|---|
| Model switches (P01) | A turn on a different model from the session's previous turn, not after a compaction, while the previous writes' cache was alive (the wait no longer than its lifetime), at a previous context over 20,000 tokens. Saving: the turn's cost less the same turn reading the earlier context and writing only its new part, at its own model's prices | `FindModelSwitches`, `ForModelSwitches` | ADR 0050 |
| Replies cut off (P12) | Claude Code's own error replies (`isApiErrorMessage`, model `<synthetic>`), sorted by text first, then by error field: "went to sleep" is the computer sleeping; a request refused before any work (an expired login, a bad request, a billing error, a rate limit, an overload, a server not reached: `ENOTFOUND`, `ECONNREFUSED`) is "not sent", counted apart, and costs nothing; a connection lost mid-response, a timeout or a server error is network. Counted once by line id; still activity for idle times. Saving: each sleep's context (the latest turn's) re-read at its model's cache-read price, a lower bound: after a long sleep the cache has often expired too, which P09 counts; the card shows at three sleeps or more | `FindFailures`, `ForFailures` | ADRs 0050, 0052 |
| Desktop-app tools never used (P06) | Tool-definition parts of MCP servers the desktop app adds (`ccd_`, `Claude_`, and the listed app servers) none of whose tools any turn called; "up to" is their whole carried cost | `UnusedAppServers`, `ForAppServers` | ADRs 0050, 0052 |
| Tool inputs | Each tool call's input size (bytes ÷ 4 as tokens), priced as carried from the next turn on, like tool output (arrival, then history price), until a compaction | `FindToolInputs` | ADR 0050 |
| Thinking share (P14) | Each turn's output cost times its thinking tokens over its output tokens, over all turns, against all output cost | `ThinkingCost` | ADR 0050 |
| Cost to start a session | Median cost of a main session's first turn (the median's rule); quoted on the long-break card | `StartCost` | ADR 0050 |
| When you work | Priced turns' cost by local hour and day of the week; weekend share; the most main sessions whose first and last turns overlap | `FindWhen` | ADR 0050 |
| Log retention | `cleanupPeriodDays` from Claude Code's settings file (that key only), else 30; a tip under 90 days | `Reader.Retention` | ADR 0050 |

## 6. Live nudges

| Figure | How it is worked out | Code | Decided in |
|---|---|---|---|
| Session size | The latest reply's context, from the last megabyte of the log; after a compaction (`/compact` or automatic), "compacted" until the next reply | `ReadTail`, `Status` | ADR 0034, 0037, 0038, 0039, 0040 |
| Cache countdown | From the latest reply's time, for the lifetime of the latest reply that wrote to the cache | `CacheLife` | ADR 0034, 0036 |
| Next message after the cache expired | The whole context written to the cache again, at that lifetime's write price | `ResendCost` | ADR 0034 |
| "About N× a fresh session" | The context divided by a fixed 30k tokens. The nudge reads one log and cannot measure a typical first request; the report measures it (`TypicalStart`, about 15k on the founder's logs), so the nudge's multiple errs low | `AfterReply` | ADR 0034 |
