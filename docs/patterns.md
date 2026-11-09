# Waste pattern catalogue

Each pattern has an ID, the signals that reveal it, how it is detected, how the
saving is estimated, and the fix (with the actuator safety level from
[design.md §7](design.md#7-actuators-fixing-things)). How Trimloop chooses
among a pattern's fixes, keeps cuts failure-preserving and measures the
result is set by [ADR 0030](adr/0030-how-fixes-are-chosen.md).

"Prefix" means the leading part of a request that a provider can serve from its
prompt cache. Anything that changes inside the prefix invalidates the cache
from that point on.

**Milestone:** when the detector ships ([ROADMAP.md](../ROADMAP.md)).
**Built:** what the M0 prototype reports today (`pkg/detectors`): **Yes**;
**Reduced**, a reduced-fidelity version from usage and harness records; or
**No**, planned.
**Kind:** **Go** = per-turn state machine; **SQL** = cross-session aggregate;
**Go + SQL** = both ([ADR 0010](adr/0010-go-detectors-sql-aggregates.md)).

Detectors are provider- and harness-neutral: each declares the capabilities it
needs ([analyzer.md §10.4](analyzer.md#104-capability-gated-detectors)) and
reports "not applicable" where they are missing, never zero. Caching is not
neutral, so P01 and P02 describe each provider cache model.

| ID | Pattern | Milestone | Built | Kind | Max fix level |
|---|---|---|---|---|---|
| P01 | Cache-prefix churn | M0 (model switches, reduced: [ADR 0050](adr/0050-second-gap-analysis-findings.md)) | Reduced | Go | 3 |
| P02 | Cache TTL expiry | M0 ([ADR 0042](adr/0042-p02-p14-and-main-session-models.md)) | Reduced | Go | 2 |
| P03 | Repeated file reads | M0 | No (re-reads are shown under P04) | Go | 1 |
| P04 | Oversized and noisy tool output | M0 | Yes | Go | 3 |
| P05 | Tool-call loops | M0 | No | Go | 0 |
| P06 | Unused tool definitions | M0 | No | SQL | 1 |
| P07 | Bloated standing and injected context | M0 | Yes | Go ([ADR 0033](adr/0033-reconciliation-history-standing-context.md)) | 1 |
| P08 | Stale context | M3 | No | Go | 3 |
| P09 | Resume bloat | M0 ([ADR 0027](adr/0027-measured-m0-findings.md)) | Yes | Go | 1 |
| P10 | Compaction thrash | M0 reduced ([ADR 0028](adr/0028-harness-recorded-signals.md), [ADR 0045](adr/0045-m0-measurement-method.md)); M3 full | Reduced | Go | 1 |
| P11 | Post-failure burn | M3 | No | Go | 0 |
| P12 | Retry storms | M0 reduced ([ADR 0028](adr/0028-harness-recorded-signals.md); replies cut off: [ADR 0050](adr/0050-second-gap-analysis-findings.md)); M3 full | Reduced | Go | 1 |
| P13 | Model over-provisioning | M0 sub-agents and main sessions, reduced ([ADR 0031](adr/0031-m0-order-from-measured-spend.md), [ADR 0042](adr/0042-p02-p14-and-main-session-models.md)); M4 full | Reduced | Go | 3 |
| P14 | Thinking-budget overspend | M0 reduced ([ADR 0028](adr/0028-harness-recorded-signals.md), [ADR 0042](adr/0042-p02-p14-and-main-session-models.md)); M4 full | Reduced | Go | 3 |
| P15 | Duplicated exploration across a team | M4 | No | SQL | 1 |
| P16 | Repeated workflow (skill candidate) | M4 | No | Go + SQL | 1 |
| P17 | Repeated instructions | M4 | No | SQL | 1 |
| P18 | Hot documentation | M4 | No | SQL | 1 |
| P19 | Large working context | M0 ([ADR 0027](adr/0027-measured-m0-findings.md)) | Yes | Go | 1 |

---

## P01 — Cache-prefix churn

**In M0 (reduced fidelity, built):** one cause the logs show without the
request bodies: a session moving to another model part-way through, whose
first turn on the new model saves the whole conversation again while the
old model's cache was still alive ([ADR 0050](adr/0050-second-gap-analysis-findings.md)). Other churn, measured
from usage alone, was too small to report on the founder's logs.

**What happens:** Something early in the request changes between turns, so the
provider cannot reuse its cached copy of the prefix. It charges full price —
or, on providers that bill cache writes, a write premium — for content it saw
moments ago.

**By cache model** ([analyzer.md §10.2](analyzer.md#102-provider-profiles)):

| Cache model | Example providers | Signal | Saving |
|---|---|---|---|
| Explicit breakpoints | Anthropic; Claude on Bedrock/Vertex | `cache_write` high relative to `cache_read` on consecutive turns; first differing segment sits before the last breakpoint | `(cache_write − expected_write) × (write_price − read_price)` |
| Automatic prefix | OpenAI | `cache_read ÷ input_total` low on consecutive turns whose prefixes should match; first differing segment early in the request | `uncached_but_cacheable × (input_price − cached_price)` |
| Implicit + managed | Gemini | Implicit: as automatic prefix. Managed: the same cached content re-created repeatedly | Implicit: as automatic prefix. Managed: re-creation cost plus overlapping storage |

**Detection:** For each consecutive pair of turns, walk the ordered segment
fingerprints and record the first divergence. Group by the segment type and
source of the divergence (e.g. `system` line 3, `tool_definition` order,
`memory_file` changed). Without the full request body the detector
still sees *that* churn happened from usage figures, but runs at reduced
fidelity and cannot name the cause.

**Common root causes:**
- Timestamps, git status or random IDs embedded in the system prompt or a
  memory file.
- MCP servers returning tool lists in a non-deterministic order.
- Context editing: the provider or harness clearing old tool results or
  compacting history (`history_edit_at`) — reported as its own cause.
- Key or block order changed: same content hash, different exact hash
  ([ADR 0025](adr/0025-exact-hash-order-preserving-json.md)).
- Tools added or removed mid-session.

**Fix:**
- L1 (all models): diff that moves volatile content below stable content in
  the memory file, or removes it.
- L2, explicit breakpoints: sort tool definitions; place breakpoints within
  the provider's limit (`sort_tools`, `set_cache_breakpoint`).
- L2, automatic prefix: sort tool definitions; set a per-session cache key
  where the provider supports one (`sort_tools`, `set_cache_key`).
- L3 (all models, opt-in): move volatile segments behind stable ones
  (`move_segment`) — it changes what the model sees
  ([ADR 0013](adr/0013-decide-path-and-transform-safety.md)).
- Managed caches: reuse one cache per session instead of re-creating it (L1).

**Requires:** `usage.cache_read`. **Optional:** `request.full_body`,
`usage.cache_write`, `request.cache_markers`.

## P02 — Cache TTL expiry

**In M0 (reduced fidelity):** turns in sessions on the 5-minute cache (in
the founder's logs, sub-agents and SDK runs) after a wait of 5 minutes to an
hour, priced against the cache still being there; what the hour's cache
would save all such sessions net of its dearer writes; and, the other way,
what the hour saved the sessions that used it, as a range
([ADR 0042](adr/0042-p02-p14-and-main-session-models.md), [ADR 0045](adr/0045-m0-measurement-method.md); [measurements.md](measurements.md)).

**What happens:** The developer pauses longer than the cache lives, so the
next turn pays again for a prefix that was cached moments before the pause.

**By cache model:**

| Cache model | Lifetime | Signal | Fix |
|---|---|---|---|
| Explicit breakpoints | Short default plus a longer paid option (Anthropic: 5 min and 1 h) | Gap longer than the TTL, then a large cache write of an unchanged prefix | L1: recommend the longer TTL for sessions with this pause profile. L2: `set_cache_ttl` when pause history predicts it pays off |
| Automatic prefix | Provider-managed, typically minutes of inactivity; some models offer extended retention | Gap, then `cache_read` falls to near zero for an unchanged prefix | L1: recommend extended retention where the model offers it; otherwise report only (L0) |
| Implicit + managed | Implicit: provider-managed. Managed: TTL you set, storage billed over time | Implicit: as automatic prefix. Managed: cache expired, then re-created | L1: tune the managed-cache TTL against storage cost |

**Detection:** Join turns on session; flag gaps over the profile's TTL where
the prefix fingerprint is unchanged and cached reads drop.

**Saving:** Cost of re-caching minus the cost of the longer-lifetime option.
Keep-alive requests are **off by default** on every model: they cost money and
may conflict with provider terms.

**Requires:** `usage.cache_read`, `timing.per_turn`. **Optional:**
`usage.cache_write`.

## P03 — Repeated file reads

**What happens:** The agent reads the same unchanged file several times in one
session, adding it to context each time.

**Signals:** The same file-content fingerprint appears in N > 2 `tool_result`
segments with `source_kind = file` within a session.

**Detection:** Count fingerprint occurrences per session, excluding reads after
the file changed (the fingerprint differs).

**Saving:** `(N − 1) × file_tokens × input_price` (adjusted for cache reads).

**Fix:**
- L1: recommend a repo-level note in the memory file pointing at the file's
  key facts; or flag the harness behaviour upstream.
- No L2: replacing a re-read with a reference changes what the model sees.

## P04 — Oversized and noisy tool output

**What happens:** A tool returns much more than the agent needed — a whole
file, a broad search, a verbose build log, a data dump. The cost is not paid
once: the output stays in the conversation, so every later turn re-sends it
(at cache-read price while the cache holds, at cache-write price whenever it
is rebuilt). It also makes the context large sooner (P19) and every cache
re-write after a pause bigger (P09).

Most of the cost is **volume times how long it is carried**, not single huge
results. Claude Code already caps what the model sees of one result (about
30 KB; larger output is saved to disk), so a single dump is bounded; thousands
of moderate outputs carried for hundreds of turns are not.

In the founder's logs the noisiest shell programs were file viewing and
searching, not builds or tests, and many file views re-read a file already
seen. Half or more of those commands were already piped through a filter;
the waste is whole-file views, broad searches and repeats. A one-off script
that carried output past compactions measured more than the M0 detector,
which drops output at each compaction as the harness does: compaction
accounts for the gap. Measured figures: [log-signals.md §8](log-signals.md#8-measured-figures-at-a-glance); method: [measurements.md](measurements.md).

**Kinds of noise:**

| Kind | Example shapes | Recommended fix |
|---|---|---|
| File viewing through the shell | `cat`, `sed -n`, `head -n`, `tail -n` on a file | Use the harness's file-read tool with offset and limit; read only the lines needed; avoid re-reading an unchanged file (P03) |
| Broad searches | `grep -r`, `ls -R`, `find` | `grep -l`, `-m N`, a narrower path, or the harness's search tool with a result limit |
| Build, test and install logs | `go test -v`, `npm install`, `docker build`, `pytest -v` | Quiet flags, failures only, `\| tail -n 50` |
| Data dumps | inline scripts printing whole structures, `gh api`, `curl` JSON, `kubectl get -o yaml` | Filter first (`jq`, selected fields) |
| Repeated polling | the same status or log command again and again | Check less often or wait for the event (P05) |

**Signals:**
- A tool result's size (bytes and estimated tokens), and how many later turns
  carry it before the session ends or is compacted.
- For shell tools, the **command shape**: program, subcommand and the flags
  that change output size (`sed -n`, `grep -r`, `go test -v`), and whether
  the output was already piped through a filter.
- For file views, the file (as a keyed `path`) and whether it was already
  viewed, unchanged, in the session.
- Results the harness saved to disk because they exceeded its cap
  (Claude Code: `persistedOutputPath`, `persistedOutputSize`).

**Detection:** Per tool result, compute its **carried cost**: its tokens at
the arrival turn's write price, plus its tokens at each later turn's price
for cached history, until the session ends or is compacted. Group by tool
and, for shell tools, by command shape; rank groups by carried cost. Command
shapes are derived on the developer's machine from the command text; only
the keyed shape code (`fp("cmd.shape", …)`), sizes and costs leave it
(ADRs [0011](adr/0011-keyed-fingerprints.md),
[0024](adr/0024-developer-identity-and-key-handling.md)).

**Saving:** For each group, the carried cost of the tokens above a target
size per result (default 2k tokens), as an upper bound. Reported as part of
P19's breakdown of what fills the context; never added to P19's saving.

**Fix:** chosen and presented by the fix ladder of
[ADR 0030](adr/0030-how-fixes-are-chosen.md): one primary fix and up to two
alternatives of different kinds, all failure-preserving, measured after.

| Kind | Level | Fixes for noisy output | Claude Code rendering |
|---|---|---|---|
| `source` | L1 | Make the tool print less: quiet flags (`-q`, `--silent`), a failures-only test reporter, a small wrapper script that prints a summary and the failures; filter data at the source (`jq`, selected fields, `--limit`); configure MCP servers to return summaries or pages; disable MCP tools the repo never uses (P06) | A diff adding the wrapper script or flags; an MCP configuration change |
| `structure` | L1 | Keep noise out of the main session: delegate exploration (wide searches, many file reads) to a sub-agent, whose summary is all that returns; compact or start a new session after an exploring phase (P19); tune when old tool output is cleared | A memory-file line asking for sub-agents on exploration; context-management settings where the harness exposes them |
| `enforce` | L1 | Stop or rewrite the noisy action: warn or block with a reason (for example, "use Read with a line range instead of `cat` on a 3,000-line file"), or add a filter | A pre-tool hook; it can warn or block, and on versions that support it, rewrite the command |
| `cap` | L1 | A hard per-result output limit | The harness's output-length settings for shell and MCP results (names per harness version, held in the harness profile) |
| `advise` | L1 | Instructions per kind of noise (table above) | A memory-file line, for example *"View files with the Read tool, using offset and limit; don't use `cat` or `sed` to view files. Pipe long command output through `tail -n 100`."* |
| `trim` | L3 (M4) | Op `truncate_tool_result`: keep head and tail plus a pointer the agent can use to fetch the rest; opt-in, canary-gated | — (applied in the request path) |

Until `truncate_tool_result` ships, P04's fixes are L1.

**Choosing among them.** `advise` is cheapest and often enough;
`enforce` is the step up when the habit persists; `source` and
`structure` last longest where the tool or the way of working can change;
`cap` is blunt and is offered last among the L1 kinds. For the founder's
logs, the primary fix is `advise` (Read with ranges) with `structure`
(sub-agents for exploration) and `enforce` (a hook on `cat` and `sed -n`) as
alternatives, aimed at file-view output and repeat file views.

**Cautions** (ADR 0030):
- **Keep failures visible.** A filter or cap that hides the failing line
  makes the agent re-run commands or read more files. Templates keep exit
  status, error output and the end of test and build runs, where failures
  are summarised.
- **Measure after.** After a fix is applied, compare the same command
  shape's carried cost over the next week of activity, and watch for re-runs
  of the same shape, more turns per session and more tool errors. No saving:
  escalate (`advise` → `enforce` → `cap`). Side effects: roll back.

**Requires:** `response.tool_calls`. **Optional:** `content.local` (command
shapes and file paths, derived locally), `harness.compaction` (where
carrying stops).

## P05 — Tool-call loops

**What happens:** The agent repeats the same action, or cycles edit → test →
fail without progress.

**Signals:**
- Identical `(tool_name, args_fingerprint)` three or more times in a row.
- Repeating sequences of tool calls with no change in the working-tree diff
  fingerprint.

**Detection:** Sequence matching over tool calls (Go detector, not SQL).

**Saving:** Tokens spent from the first repeat until the loop ends.

**Fix:** L0 only — report with the loop highlighted. Loops point at prompt,
environment or test problems a human must fix. Optional live alert in the
proxy.

## P06 — Unused tool definitions

**What happens:** MCP servers and tools are defined on every request but never
called in this repo, inflating every prompt.

**Signals:** Tool definitions present in requests; zero calls across the last N
sessions in the repo.

**Detection:** Join tool-definition fingerprints against tool-call names per
repo over a rolling window.

**Saving:** Definition tokens × requests × (cache-read price, or input price
when uncached).

**Fix:** L1 — diff to disable the MCP server, or the specific tools, for that
repo.

## P07 — Bloated standing and injected context

**What happens:** Memory files, system rules, tool definitions and the context
the harness injects (skill listings, MCP instructions, environment and sandbox
text, agent listings, reminders) are much larger than needed.

**Signals:** Standing-context segments (system prompt + memory files + tool
definitions) and harness-injected kinds above a threshold share of total
input tokens. Where `harness.injected_context` is available, each injected
kind is sized separately.

**Detection:** Per repo: standing tokens ÷ total input tokens; list the largest
contributors by kind — memory file, system prompt, tool definitions, and each
injected kind — so the fix names the skill, plugin or MCP server. In M0
([ADR 0033](adr/0033-reconciliation-history-standing-context.md)) the system
prompt and tool definitions are priced on every turn (cache-read price, or
the write price when new or changed), tool definitions grouped by MCP
server; injected text is carried like P04's tool output. In the founder's logs
the built-in tool definitions cost the most, then the skill listing; MCP
servers and memory files cost far less. Measured figures: [log-signals.md §8](log-signals.md#8-measured-figures-at-a-glance); method: [measurements.md](measurements.md).

**Saving:** Hard to estimate precisely; reported as an upper bound with low
confidence. Injected kinds can be priced like tool output (P04): carried
from when they appear until the session ends or is compacted.

**Fix:** L1 — report the largest sections. Where a section is relevant to
only some sessions, recommend moving it into a skill (or the harness's
equivalent) so it loads on demand instead of on every request — see P16.
No automatic rewrite: a human decides what instructions matter. Judging
relevance needs the section text, so that part runs locally
(`content.local`).

## P08 — Stale context

**What happens:** Context that is superseded or no longer useful (an old
version of a file that has since been edited, a finished sub-task's output)
keeps being re-sent. Research on trajectory reduction (AgentDiet) found such
content is widespread and that removing it cut input tokens by roughly 40–60%.

**Signals:** A file's older fingerprint still in history after a newer
fingerprint of the same path appears.

**Fix:** L1 in M3 — report the stale share and recommend compaction or
clearing settings. L3 in M4 (op `elide_segment`): elide superseded segments,
with a marker; behaviour-changing and harness-specific, opt-in, canary-gated
([ADR 0021](adr/0021-decide-body-ops-and-policy-signing.md)).

## P09 — Resume bloat

Reports call it "long pauses" or "start fresh after a long break".

**What happens:** A session is picked up again after the cache has expired —
by a resume, or simply by carrying on after a long break — and re-sends its
full history with a cold cache, or in a form that does not match the earlier
cached prefix.

**Signals:** A turn after idle longer than the longest cache lifetime the
provider offers (Anthropic: 1 hour) shows a cache write. At full fidelity:
prefix fingerprints differ from the pre-resume turn despite identical
content.

**Detection (M0, reduced fidelity):** Idle time is measured from the last
activity in the session's log. Saving: the cache-write cost of those turns,
an upper bound (a compacted or new session still writes its smaller
context). A main session also goes idle while a subagent runs; such gaps are
not breaks, and the advice must allow for them
([ADR 0027](adr/0027-measured-m0-findings.md)). A break the user took is a
pause in a main session that the user ended: by typing, answering Claude's
question, or allowing or refusing a tool call. The report counts the other
waits (on a tool, a sub-agent, a timer or another agent, or inside a
sub-agent), and the "start fresh" saving leaves them out
([ADR 0045](adr/0045-m0-measurement-method.md)).

**Fix:** L1 — report the cost of cold resumes and recommend continuing within
the cache lifetime or starting fresh. (No automatic fix: rewriting a resumed
request has no safe, defined operation.) The M0 report recommends starting a new session after a break of
over an hour (or compacting *before* the break, while the cache is warm:
compacting after it re-sends the whole conversation), with a likely saving
from pricing each post-pause turn as a new session's first request
([ADR 0032](adr/0032-readable-reports.md)). Live nudges show the cache
countdown and, once it has expired, the cost of the next message; over
500k tokens the first message back is held once
([ADR 0034](adr/0034-live-nudges.md)); after a `/compact` the status line
says "compacted", shows a size again after the next reply, and holds
nothing (ADR 0037, ADR 0038).

## P10 — Compaction thrash

**What happens:** The harness compacts (summarises) the conversation
repeatedly, each time paying for the summary and losing cache.

**Signals:** Several compaction events per session; short intervals between
them. Where the harness records compaction (`harness.compaction`): tokens
before and after, tokens dropped, and the time each compaction took.

**Detection:** From harness compaction records (M0), else from history
fingerprints that reset while the system prompt is unchanged. In reduced
fidelity (M0, [ADR 0031](adr/0031-m0-order-from-measured-spend.md)) the
report lists compactions — manual or automatic, typical size before and
after, and the longest wait (a median misleads: Claude Code prepares some
compactions in advance, which then take milliseconds) — alongside P19, and
counts sessions that grew past 500 k tokens without compacting.

**Fix:** L1 — recommend splitting the task, trimming standing context (P07),
or tackling P04 and P08 first; report what each compaction kept and cost.

**Requires:** `harness.compaction` **or** `request.full_body`.

## P11 — Post-failure burn

**What happens:** The run has effectively failed (repeated errors, the user's
goal unreachable) but continues spending.

**Signals:** Error-rate spike, then continued turns without diff progress; the
session ends without a commit.

**Fix:** L0 — report and optional live alert with a "stop here?" threshold.

## P12 — Retry storms

**What happens:** Rate-limit (429) or overload responses trigger repeated
retries, each re-sending the full request.

**Signals:** Several error responses with the same request fingerprint within
seconds; or, where the harness records retries (`harness.retries`), retry
attempts, delays and causes per turn, and time spent retrying.

**In M0 (reduced fidelity, built):** requests Claude Code's own error
replies show were cut off and sent again — the computer sleeping
mid-response, network and server errors — with the context re-read as a
lower bound; the card is about keeping the computer awake ([ADR 0050](adr/0050-second-gap-analysis-findings.md)).

**Detection:** From harness retry records (M0, not built yet), else from
repeated request fingerprints (M3).

**Fix:** L1 — recommend backoff or concurrency settings; alert on rate-limit
headroom.

**Requires:** `harness.retries` **or** `request.full_body`. **Optional:**
`timing.per_turn`.

## P13 — Model over-provisioning

**In M0:** sub-agents on a cheaper model ([ADR 0031](adr/0031-m0-order-from-measured-spend.md)),
and main sessions on a newer model in the same line or, as a trade-off,
one tier down ([ADR 0042](adr/0042-p02-p14-and-main-session-models.md)).

**What happens:** The most capable (and most expensive) model handles turns
that a smaller model handles just as well, such as file listing, grep-and-read,
or formatting.

**Signals:** Turn classifier labels the turn low-complexity; the model used is
in the top price tier.

**Detection:** Needs a turn-complexity classifier trained on outcome data, so
it waits until outcome signals exist.

**Fix:** L3 (M4, op `route_model`) — routing policy per turn class, canary-gated on outcome signals.
Many harnesses already support sub-agents on cheaper models; the L1 version
recommends that configuration.

**Reduced fidelity for sub-agents (M0, [ADR 0031](adr/0031-m0-order-from-measured-spend.md)):**
no classifier is needed to see that sub-agents inherit the main model. Report
sub-agent spend by agent type and model, and the same turns priced on the
next-cheaper model; recommend setting a model for exploration and search
agents (Claude Code: the Agent tool's `model` argument or a custom agent's
`model` field), stated as untested for quality and checked after with the
agent type's outcome signals (ADR 0030). Agent types that continue the main
conversation and read its cache (Claude Code's `fork`) get no suggestion,
since another model would lose that cache. In the founder's logs, no Agent
call set a model, so every sub-agent ran on the main model. Measured figures: [log-signals.md §8](log-signals.md#8-measured-figures-at-a-glance); method: [measurements.md](measurements.md).

## P14 — Thinking-budget overspend

**In M0 (reduced fidelity, built):** spend by effort level, and how much
more xhigh and max turns think per turn, on average, than the same model's
high-effort turns ([ADR 0042](adr/0042-p02-p14-and-main-session-models.md), [ADR 0045](adr/0045-m0-measurement-method.md)),
and thinking's share of all output spend ([ADR 0050](adr/0050-second-gap-analysis-findings.md)).

**What happens:** Extended reasoning tokens are spent on trivial turns.

**Signals:** Reasoning tokens high on turns labelled low-complexity. In
reduced fidelity (M0): thinking tokens (Claude Code logs them in
`output_tokens_details`, as part of output) and output cost by requested
effort level (`request.effort`), and how much more per turn, on average, the
highest levels think than high does ([ADR 0045](adr/0045-m0-measurement-method.md)).

**Fix:** L1 (M0) — report spend by effort level and recommend a lower
default or per-turn effort where the harness supports it. In the founder's
logs the lever is smaller than its token share suggests: the highest
levels' extra thinking costs far less than their share of spend. Measured figures: [log-signals.md §8](log-signals.md#8-measured-figures-at-a-glance); method: [measurements.md](measurements.md). L3 (M4, op
`cap_thinking`) — cap the budget per turn class; canary-gated.

**Requires:** `request.effort` (reduced, M0) or `usage.reasoning` (full). **Optional:** `usage.reasoning` in reduced fidelity, where it replaces output tokens.

## P15 — Duplicated exploration across a team

**What happens:** Many developers' agents explore the same repo from scratch
(reading the same files to understand the architecture) every session.

**Signals:** The same file fingerprints read early in sessions by many
developers in one repo.

**Fix:** L1 — recommend a shared, version-controlled architecture summary in
the repo's memory file, and generate a draft of it.

## P16 — Repeated workflow (skill candidate)

**What happens:** The same multi-step task — "add a database migration", "cut
a release", "fix a failing test" — is done from scratch in many sessions. The
agent rediscovers the steps every time.

**Signals:**
- A normalised tool-call sequence (tool names plus argument templates, e.g.
  run tests → read failing test → edit → re-run) recurring across N or more
  sessions.
- The same files read at the start of each occurrence.
- High turn count per occurrence.

**Detection:** Normalise each session's tool calls into a sequence of
`(tool, argument-template fingerprint)` and mine frequent sub-sequences across
sessions: per developer locally, per repo and team at the collector, on
fingerprints only. Optionally, and locally only, cluster the user prompts that
start each occurrence to give the task a readable label.

**Saving:** `(turns per occurrence − expected turns with a skill) × average
turn cost × occurrences per week`, plus developer time. Low confidence until
measured before and after.

**Fix:** L1 — recommend a skill, or the harness's equivalent (slash command,
custom prompt), with a draft generated locally from the recorded occurrences.
A human edits and commits it. For harnesses without skills, recommend a
memory-file section instead.

**Requires:** `response.tool_calls`. **Optional:** `content.local` (draft and
task labels), `harness.skills` (otherwise the memory-file fallback).

## P17 — Repeated instructions

**What happens:** The developer types the same guidance into many sessions —
"use our logger", "don't touch the generated files", "run the linter first".

**Signals:** The same user-message line hash appearing in N or more
sessions for a developer or repo.

**Detection:** Frequency of line hashes in `user_text` segments, per
developer and per repo.

**Saving:** Mostly developer time and consistency; the token saving is small.
Reported as frequency rather than money.

**Fix:** L1 — move the guidance into the memory file if it applies to every
session, or into a skill if it applies to one kind of task. The draft is
rendered locally.

**Requires:** — (user messages are always present). **Optional:**
`content.local`, `harness.skills`.

## P18 — Hot documentation

**What happens:** The same documentation is pulled into context again and
again — web pages fetched, large doc files read, documentation looked up
through MCP tools — across sessions and developers.

**Signals:**
- The same URL or file fingerprint in tool results across many sessions.
- Large token counts per fetch.
- Little of the fetched content used afterwards (the P04 ratio).

**Detection:** Aggregate tool-result segments by source (URL or path hash)
and content fingerprint across sessions; rank by total tokens × frequency.
A source whose content fingerprint keeps changing is flagged as volatile.

**Saving:** Repeated fetch tokens minus the cost of the cheaper alternative,
plus fetch latency.

**Fix:** L1, simplest option first:

1. **Short summary** in the memory file or a skill — for small, stable docs.
2. **Local documentation search tool** (an MCP server over an index), so the
   agent retrieves only the parts it needs — for large or changing docs.
3. **Full retrieval-augmented generation (RAG)** — only where measured data
   shows it beats the agent's own search. Trimloop exports the hot-doc list
   (sources, frequency, sizes, volatility) as input to a RAG builder; it does
   not build or host the index itself
   ([ADR 0007](adr/0007-recommend-skills-and-doc-retrieval.md)).

Volatile sources get option 2 or 3 with a refresh schedule, never a static
summary. Every recommendation is measured before and after.

**Requires:** `response.tool_calls`. **Optional:** `content.local` (to draft
summaries), `harness.skills`.

## P19 — Large working context

**What happens:** A session keeps going until its context is very large.
Every turn re-sends the whole conversation, so each later turn pays again,
at cache-read price or worse, for everything accumulated so far. In the
founder's sessions, turns over 200k tokens made more than half of spend
([ADR 0027](adr/0027-measured-m0-findings.md)).

**Signals:** A turn's input tokens (uncached, cache read and cache write)
above a threshold, default 200k.

**Detection:** Per turn, from usage alone. Report the turns and sessions
above the threshold, the largest contexts, and how fast each session grew.
Also report where spend piles up ([ADR 0031](adr/0031-m0-order-from-measured-spend.md)):
the share in the costliest 1, 5 and 10 conversations (a main session with
its sub-agents), the mean cost of a turn by context size (under 50 k, to
200 k, to 500 k, over 500 k), and sessions past 500 k that never compacted.
In the founder's logs a few conversations made most of the spend, and a
turn over 500 k cost several times one under 50 k. Measured figures: [log-signals.md §8](log-signals.md#8-measured-figures-at-a-glance); method: [measurements.md](measurements.md).

**Saving:** The cost of the tokens above the threshold. They are the oldest
history, so each is priced by where it sat in that turn's request: read
from the cache, or written where the cache was rebuilt
([ADR 0045](adr/0045-m0-measurement-method.md);
[measurements.md](measurements.md) §2). An
upper bound for keeping every turn under the threshold: some context is
needed. Overlaps P09; the two are never added. The compaction card's
figures differ because they measure another change: compacting down to
the typical size of the first request after a compaction (the summary,
the system prompt and re-attached files)
whenever a session passes the threshold, which removes more than the part
above it, priced the same way.

**Fix:** L1 — compact when a session passes the threshold or between tasks
(Claude Code: `/compact`; it compacts on its own only near the context
limit), or start a new session per task. The report estimates a likely
saving for compacting by replaying each main session as if compacted to the
typical size of the first request after a compaction (the summary plus the
system prompt and re-attached files) whenever it passed the threshold,
including each compaction's cost, with a range from how much that size
varies ([ADR 0032](adr/0032-readable-reports.md),
[ADR 0045](adr/0045-m0-measurement-method.md); [measurements.md](measurements.md)). Live
nudges remind the user at 200k and 500k tokens and show the size in the
status line ([ADR 0034](adr/0034-live-nudges.md)). Where P04, P07 or P08 are
detected, report them as the causes: they say *what* fills the context.

**Requires:** `usage.cache_read`. **Optional:** `usage.cache_write`,
`response.tool_calls` (to break the context down by cause).
