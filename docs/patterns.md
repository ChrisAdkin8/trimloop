# Waste pattern catalogue

Each pattern has an ID, the signals that reveal it, how it is detected, how the
saving is estimated, and the fix (with the actuator safety level from
[design.md §7](design.md#7-actuators-fixing-things)).

"Prefix" means the leading part of a request that a provider can serve from its
prompt cache. Anything that changes inside the prefix invalidates the cache
from that point on.

Priority: **MVP** = ships with the transcript analyser; **v1** = needs the proxy
or the team collector; **later** = needs outcome data or routing.

| ID | Pattern | Priority | Max fix level |
|---|---|---|---|
| P01 | Cache-prefix churn | MVP | 2 |
| P02 | Cache TTL expiry | MVP | 2 |
| P03 | Repeated file reads | MVP | 1 |
| P04 | Oversized tool output | MVP | 3 |
| P05 | Tool-call loops | MVP | 0 |
| P06 | Unused tool definitions | MVP | 1 |
| P07 | Bloated standing context | MVP | 1 |
| P08 | Stale context | v1 | 3 |
| P09 | Resume bloat | v1 | 2 |
| P10 | Compaction thrash | v1 | 1 |
| P11 | Post-failure burn | v1 | 0 |
| P12 | Retry storms | v1 | 1 |
| P13 | Model over-provisioning | later | 3 |
| P14 | Thinking-budget overspend | later | 3 |
| P15 | Duplicated exploration across a team | later | 1 |

---

## P01 — Cache-prefix churn

**What happens:** Something early in the request changes between turns, so the
provider re-writes the cache (paid at a premium) instead of reading it (paid at
a discount).

**Signals:**
- `cache_creation_input_tokens` high relative to `cache_read_input_tokens` on
  consecutive turns of the same session.
- The first differing segment fingerprint between turn *n* and turn *n+1*
  sits before the last cache breakpoint.

**Detection:** For each consecutive pair of turns, walk the ordered segment
fingerprints and record the first divergence. Group by the segment type and
source of the divergence (e.g. `system_prompt` line 3, `tool_definitions`
reordered, memory file changed).

**Common root causes:**
- Timestamps, git status or random IDs embedded in the system prompt or a
  memory file.
- MCP servers returning tool lists in a non-deterministic order.
- Tools added or removed mid-session.

**Saving:** `(cache_write_tokens − expected_write_tokens) × (write_price −
read_price)`.

**Fix:**
- L1: diff that moves volatile content below stable content in the memory file,
  or removes it.
- L2: proxy sorts tool definitions deterministically; moves volatile segments
  after the cache breakpoint.

## P02 — Cache TTL expiry

**What happens:** The developer pauses longer than the cache lifetime, so the
next turn re-writes a prefix that was cached moments before the pause.

**Signals:** Gap between turns greater than the provider's cache TTL, followed
by a large cache write of an unchanged prefix (same fingerprints).

**Detection:** Join turns on session; flag gaps over TTL where the prefix
fingerprint is unchanged.

**Saving:** Re-write cost minus the cost of the longer-TTL option (where the
provider offers one) or of a keep-alive request.

**Fix:**
- L1: recommend the longer cache TTL where the provider supports it, for
  sessions with this pause profile.
- L2: proxy requests the longer TTL automatically when the session's pause
  history predicts it will pay off. Keep-alives are **off by default**: they
  cost money and may conflict with provider terms.

## P03 — Repeated file reads

**What happens:** The agent reads the same unchanged file several times in one
session, adding it to context each time.

**Signals:** The same file-content fingerprint appears in N > 2 `file_read` or
tool-result segments within a session.

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

**Saving:** Tokens above the threshold × input price, multiplied by the number
of later turns that carry it in history.

**Fix:**
- L1: suggest command flags (`--quiet`, `| tail -n 200`, test reporter
  settings) via a memory-file or settings diff.
- L3: proxy truncates with head + tail + a pointer the agent can use to fetch
  the rest. Behaviour-changing, so opt-in only.

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

## P07 — Bloated standing context

**What happens:** Memory files, system rules and instructions sent on every
request are much larger than needed.

**Signals:** Standing-context segments (system prompt + memory files + tool
definitions) above a threshold share of total input tokens.

**Detection:** Per repo: standing tokens ÷ total input tokens; list the largest
contributors.

**Saving:** Hard to estimate precisely; reported as an upper bound with low
confidence.

**Fix:** L1 — report the largest sections. No automatic rewrite: a human
decides what instructions matter.

## P08 — Stale context

**What happens:** Context that is superseded or no longer useful (an old
version of a file that has since been edited, a finished sub-task's output)
keeps being re-sent. Research on trajectory reduction (AgentDiet) found such
content is widespread and that removing it cut input tokens by roughly 40–60%.

**Signals:** A file's older fingerprint still in history after a newer
fingerprint of the same path appears.

**Fix:** L3 — proxy elides superseded segments, with a marker. Behaviour-
changing and harness-specific; opt-in, canary-gated.

## P09 — Resume bloat

**What happens:** A resumed session re-sends the full history with a cold
cache, or in a form that does not match the earlier cached prefix.

**Signals:** First turn after a resume shows a large cache write; prefix
fingerprints differ from the pre-resume turn despite identical content.

**Fix:** L2 — normalise the resumed request so it matches the previous prefix.

## P10 — Compaction thrash

**What happens:** The harness compacts (summarises) the conversation
repeatedly, each time paying for the summary and losing cache.

**Signals:** Several compaction events per session; short intervals between
them.

**Fix:** L1 — recommend splitting the task, trimming standing context (P07),
or tackling P04 and P08 first.

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
seconds.

**Fix:** L1 — recommend backoff or concurrency settings; alert on rate-limit
headroom.

## P13 — Model over-provisioning

**What happens:** The most capable (and most expensive) model handles turns
that a smaller model handles just as well, such as file listing, grep-and-read,
or formatting.

**Signals:** Turn classifier labels the turn low-complexity; the model used is
in the top price tier.

**Detection:** Needs a turn-complexity classifier trained on outcome data, so
it waits until outcome signals exist.

**Fix:** L3 — routing policy per turn class, canary-gated on outcome signals.
Many harnesses already support sub-agents on cheaper models; the L1 version
recommends that configuration.

## P14 — Thinking-budget overspend

**What happens:** Extended reasoning tokens are spent on trivial turns.

**Signals:** Reasoning tokens high on turns labelled low-complexity.

**Fix:** L3 — cap the budget per turn class; canary-gated.

## P15 — Duplicated exploration across a team

**What happens:** Many developers' agents explore the same repo from scratch
(reading the same files to understand the architecture) every session.

**Signals:** The same file fingerprints read early in sessions by many
developers in one repo.

**Fix:** L1 — recommend a shared, version-controlled architecture summary in
the repo's memory file, and generate a draft of it.
