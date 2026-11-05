# ADR 0016: M1 sidecar operation — output, ordering, forwarded body, history edits

- Status: Proposed
- Date: 2026-11-05
- Amends: [ADR 0012](0012-gateway-sessions-and-parity.md) (idle window rule)

## Context

The second cold review found gaps that block M1:

1. **No output.** M1 promises live findings for every developer, but the
   dashboard, OTel export and webhooks were M2, and the sidecar listens only on
   loopback. The memory store loses findings on restart; a DuckDB store needs a
   volume the Helm chart does not define.
2. **Out-of-order turns.** The plugin batches events (every 1 s or 100
   events), and LiteLLM often runs several worker processes, each with its own
   buffer. Turn *n+1* can reach the sidecar before turn *n*, which today would
   open a new session.
3. **What the plugin sees.** LiteLLM hands hooks Python objects — often already
   translated between provider formats — not the bytes sent on the wire.
   "Exact bytes as sent" is not available at the gateway.
4. **Unproven overhead.** Each turn the plugin serialises the whole, growing
   request on LiteLLM's event loop.
5. **Sidecar state for decide.** When a collector runs session-scope
   detectors (ADR 0012), the sidecar still needs per-session history to answer
   decide requests (for example P02's TTL choice).
6. **History is not append-only.** A structure-only survey of local Claude
   Code transcripts (2026-11-05) found every recorded request sending
   `context_management` settings, `clear_at` markers, compaction summaries,
   and requests recorded as an earlier request truncated to `keep` messages
   plus a new `tail`. Prefix continuity that assumes pure appending breaks.
7. **Long cache retention.** OpenAI offers extended prompt-cache retention on
   some models and Gemini managed caches accept any TTL, so "idle window
   longer than the longest TTL" can force very long windows.

## Decision

**1. Output in M1.**

- `trimloop serve` exposes a **read-only findings API and page** on a second
  port (default `:7342`), separate from the loopback ingest port, with
  token authentication. It lists findings, coverage and session summaries —
  keyed codes and counts only.
- A **webhook sink** moves into M1 (Slack-compatible JSON), so findings reach
  a team channel without a collector.
- The Helm sidecar mode gains `persistence` values (volume size, storage
  class) for the DuckDB store; with persistence off, the memory store is used
  and the page says findings are lost on restart.
- `trimloop report --from http://sidecar:7342` renders the same reports as
  `analyse` from a sidecar.

**2. Ordering.**

- The sidecar holds incoming turns per gateway key for a **reorder window**
  (default 5 s) and releases them to sessionising in request-timestamp order.
- A turn that arrives after its window has closed is still placed by prefix
  continuity. If it shows that two open sessions are one (it continues the
  first and is continued by the second), the sessions are **merged** and their
  detectors replayed from stored turns.
- The parity test asserts identical findings when turns arrive in any order
  within the reorder window, and when a late turn forces a merge.

**3. Forwarded body.**

- The plugin forwards the **provider-bound request** — LiteLLM's request after
  translation for the target provider — plus the provider name, model and
  LiteLLM call id. A `normalize/litellm` envelope parser unwraps it and hands
  the body to the provider parser.
- At the gateway, `exact_hash` is defined over a **canonical re-serialisation
  of the provider-bound body** (RFC 8785 JSON with cache markers removed), and
  the capture records `fidelity = gateway`. Cache-churn detection still works,
  because any content change changes the canonical form; differences that
  exist only in whitespace or key order on the wire are not visible.
- Whether cache markers survive LiteLLM's translation is checked per LiteLLM
  version by the conformance kit.

**4. Overhead first.** The first M1 task is a **benchmark** of the plugin on
LiteLLM under realistic load (long sessions, several workers). If the
≤ 1 ms p99 budget is missed, the plugin sends only the part of each request
that is new since the previous turn of the same conversation (a delta against
its own last-sent prefix), and serialises off the event loop. The 50 MB
buffer cap is per worker process; the benchmark reports the total.

**5. Sidecar session state.** Whatever else runs, each sidecar keeps a small
per-session record for decide: conversation root, last prefix hashes, turn
timestamps (pause history), cache usage of recent turns and canary bucket.
It is bounded by the open-session LRU and the idle window.

**6. History edits.** Sessionising matches a new request to an open session
when it shares the conversation root and its history starts with **any
prefix** of that session's last request — not necessarily all of it. The
point where it diverges is recorded as a `history_edit` event (clearing,
compaction, truncation). P01 counts cache breaks at a `history_edit` as their
own root cause ("context editing"), not as churn in the memory file.

**7. Idle window.** The window is the larger of 2 hours and the longest
cache TTL **actually observed** in the session, capped at 24 hours. Pauses
beyond the cap are a documented P02 blind spot.

## Consequences

- M1 grows: findings API and page, webhook sink, Helm persistence values,
  LiteLLM envelope parser, reorder buffer, session merging.
- Gateway fingerprints are not byte-identical to transcript or proxy
  fingerprints for the same request; `fidelity` keeps them apart, and parity
  compares findings, not hashes.
- History edits become visible as a cause of cache breaks, which is a useful
  finding in itself.
