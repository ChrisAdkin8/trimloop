# Waste pattern catalogue

Each pattern has an ID, the signals that reveal it, how it is detected, how the
saving is estimated, and the fix (with the actuator safety level from
[design.md §7](design.md#7-actuators-fixing-things)).

"Prefix" means the leading part of a request that a provider can serve from its
prompt cache. Anything that changes inside the prefix invalidates the cache
from that point on.

**Milestone:** when the detector ships ([ROADMAP.md](../ROADMAP.md)).
**Kind:** **Go** = per-turn state machine; **SQL** = cross-session aggregate;
**Go + SQL** = both ([ADR 0010](adr/0010-go-detectors-sql-aggregates.md)).

Detectors are provider- and harness-neutral: each declares the capabilities it
needs ([analyzer.md §10.4](analyzer.md#104-capability-gated-detectors)) and
reports "not applicable" where they are missing, never zero. Caching is not
neutral, so P01 and P02 describe each provider cache model.

| ID | Pattern | Milestone | Kind | Max fix level |
|---|---|---|---|---|
| P01 | Cache-prefix churn | M0 | Go | 3 |
| P02 | Cache TTL expiry | M0 | Go | 2 |
| P03 | Repeated file reads | M0 | Go | 1 |
| P04 | Oversized tool output | M0 | Go | 3 |
| P05 | Tool-call loops | M0 | Go | 0 |
| P06 | Unused tool definitions | M0 | SQL | 1 |
| P07 | Bloated standing and injected context | M0 | SQL | 1 |
| P08 | Stale context | M3 | Go | 3 |
| P09 | Resume bloat | M0 ([ADR 0027](adr/0027-measured-m0-findings.md)) | Go | 1 |
| P10 | Compaction thrash | M0 ([ADR 0028](adr/0028-harness-recorded-signals.md)) | Go | 1 |
| P11 | Post-failure burn | M3 | Go | 0 |
| P12 | Retry storms | M0 ([ADR 0028](adr/0028-harness-recorded-signals.md)) | Go | 1 |
| P13 | Model over-provisioning | M4 | Go | 3 |
| P14 | Thinking-budget overspend | M0 reduced, M4 full ([ADR 0028](adr/0028-harness-recorded-signals.md)) | Go | 3 |
| P15 | Duplicated exploration across a team | M4 | SQL | 1 |
| P16 | Repeated workflow (skill candidate) | M4 | Go + SQL | 1 |
| P17 | Repeated instructions | M4 | SQL | 1 |
| P18 | Hot documentation | M4 | SQL | 1 |
| P19 | Large working context | M0 ([ADR 0027](adr/0027-measured-m0-findings.md)) | Go | 1 |

---

## P01 — Cache-prefix churn

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

## P04 — Oversized tool output

**What happens:** A tool returns an enormous result (full test log, recursive
directory listing, minified bundle, large JSON) that the model barely uses.

**Signals:** A tool-result segment above a size threshold (default 8k tokens),
especially when the next assistant turn references only a small part of it.

**Detection:** Size threshold, plus a ratio of tokens in the result to tokens
in the follow-up action.

**Saving:** Tokens above the threshold × input price on the turn it arrives,
plus, for each later turn that carries it in history, the same tokens at that
turn's price for them — usually the cache-read rate when the prefix was
cached.

**Fix:**
- L1: suggest command flags (`--quiet`, `| tail -n 200`, test reporter
  settings) via a memory-file or settings diff.
- L3 (M4, op `truncate_tool_result`): truncate with head + tail + a pointer
  the agent can use to fetch the rest. Behaviour-changing, so opt-in only.
  Until the op ships, L1 only.

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
injected kind — so the fix names the skill, plugin or MCP server.

**Saving:** Hard to estimate precisely; reported as an upper bound with low
confidence.

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
([ADR 0027](adr/0027-measured-m0-findings.md)).

**Fix:** L1 — report the cost of cold resumes and recommend continuing within
the cache lifetime or starting fresh. (No automatic fix: rewriting a resumed
request has no safe, defined operation.)

## P10 — Compaction thrash

**What happens:** The harness compacts (summarises) the conversation
repeatedly, each time paying for the summary and losing cache.

**Signals:** Several compaction events per session; short intervals between
them. Where the harness records compaction (`harness.compaction`): tokens
before and after, tokens dropped, and the time each compaction took.

**Detection:** From harness compaction records (M0), else from history
fingerprints that reset while the system prompt is unchanged.

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

**Detection:** From harness retry records (M0), else from repeated request
fingerprints.

**Fix:** L1 — recommend backoff or concurrency settings; alert on rate-limit
headroom.

**Requires:** `harness.retries` **or** `request.full_body`. **Optional:**
`timing.per_turn`.

## P13 — Model over-provisioning

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

## P14 — Thinking-budget overspend

**What happens:** Extended reasoning tokens are spent on trivial turns.

**Signals:** Reasoning tokens high on turns labelled low-complexity. In
reduced fidelity (M0): reasoning tokens and output cost by requested effort
level (`request.effort`), and the share spent at the highest levels.

**Fix:** L1 (M0) — report spend by effort level and recommend a lower
default or per-turn effort where the harness supports it. L3 (M4, op
`cap_thinking`) — cap the budget per turn class; canary-gated.

**Requires:** `usage.reasoning`. **Optional:** `request.effort`.

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
founder's sessions, turns over 200k tokens were 57% of spend
([ADR 0027](adr/0027-measured-m0-findings.md)).

**Signals:** A turn's input tokens (uncached, cache read and cache write)
above a threshold, default 200k.

**Detection:** Per turn, from usage alone. Report the turns and sessions
above the threshold, the largest contexts, and how fast each session grew.

**Saving:** The input cost of the tokens above the threshold, at the turn's
average input price per token. An upper bound: some context is needed.
Overlaps P09; the two are never added.

**Fix:** L1 — start a new session per task, or compact earlier. Where P04,
P07 or P08 are detected, report them as the causes: they say *what* fills
the context.

**Requires:** `usage.cache_read`. **Optional:** `usage.cache_write`,
`response.tool_calls` (to break the context down by cause).
