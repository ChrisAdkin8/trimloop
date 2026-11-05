# ADR 0013: Self-contained decide requests, gateway quality signals, transform safety

- Status: Proposed
- Date: 2026-11-05

## Context

Three gaps in automatic fixes (M3) came out of the cold review:

1. **Race.** The plugin sends finished turns asynchronously (`/v1/events`,
   batched about every second) but calls `/v1/decide` synchronously before
   each request. When the decide call for turn *n+1* arrives, turn *n* may not
   have arrived, so the session — and its canary group — is unknown.
2. **No quality signals at the gateway.** Rollout gates on outcome signals
   (git commits, tests) that gateway traffic does not carry, yet M3 ships
   transforms through the gateway first.
3. **"Safe" transforms can break requests.** Anthropic allows a limited number
   of cache breakpoints and the harness already places some; mixed cache TTLs
   have ordering rules; moving segments changes what the model sees; a request
   the provider rejects after a transform was not retried. Two examples in the
   docs (de-duplicating tool definitions, normalising resumed requests) had no
   operation behind them.

## Decision

**Self-contained decide requests.**

- `/v1/decide` carries the full request body (over loopback, ADR 0006). The
  sidecar fingerprints it and sessionises it from the request's own history,
  without waiting for `/v1/events`.
- Canary assignment needs no session state: the bucket is derived from the
  keyed fingerprint of the **conversation root** (system prompt plus first
  user message), which is present in every request of a session. The same
  session always lands in the same bucket, on any replica.

**Quality signals at the gateway.**

- Gateway rollouts use signals the gateway can see: provider error and retry
  rates, tool-call error rate, turns per session, sessions ending on an error,
  and user corrections (a user turn immediately undoing or contradicting the
  previous assistant turn, by heuristic).
- **L2** transforms may canary at the gateway on these signals.
- **L3** transforms are **not** rolled out at the gateway until developer-side
  outcome signals (git, tests) can be joined to gateway sessions (M4).

**Transform safety.**

- Every op has written preconditions, checked before it is returned:
  - `set_cache_breakpoint`: only if the request stays within the provider's
    breakpoint limit after the change (Anthropic: 4), counting the harness's
    own markers;
  - `set_cache_ttl`: only in an order the provider accepts (for Anthropic,
    longer-lived breakpoints before shorter ones — to confirm against current
    docs in the conformance kit);
  - `sort_tools`, `set_cache_key`: always allowed where the provider profile
    lists them.
- `move_segment` changes the order of text the model sees, so it is
  reclassified from L2 to **L3**.
- **Rejection fallback.** If the provider rejects a transformed request
  (HTTP 4xx other than rate limits), the plugin resends the original once and
  reports `transform_rejected`. The sidecar disables that op for that provider
  and model until a person re-enables it.
- The two examples without an op are removed: de-duplicating tool definitions
  (design §7) and normalising resumed requests (P09, now L1).

## Consequences

- Decide requests are larger (full body over loopback); the 10 ms server
  budget and 25 ms plugin timeout stand and are benchmarked.
- The L2 set at the gateway is `sort_tools`, `set_cache_breakpoint`,
  `set_cache_ttl` and `set_cache_key`.
- P01's L2 fix no longer moves segments; moving volatile segments is L3 or an
  L1 config change.
- A rejected transform costs one extra round trip for that request only.
