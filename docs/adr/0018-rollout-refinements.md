# ADR 0018: Rollout refinements — experiment buckets, sub-agents, rejection trigger, tool sorting

- Status: Proposed — bucket input amended by [ADR 0020](0020-session-identity.md); `sort_tools` gate by [ADR 0021](0021-decide-body-ops-and-policy-signing.md)
- Date: 2026-11-05
- Amends: [ADR 0013](0013-decide-path-and-transform-safety.md)

## Context

ADR 0013 derived canary buckets from the conversation root alone, so the same
sessions are in the canary arm of every experiment, and sub-agents (different
roots) can land in a different arm from their parent. It disabled an op on any
non-rate-limit 4xx after a transform, which includes unrelated errors (401,
403, 413, context-length). It allowed `sort_tools` at L2 "always", although
reordering tools changes what the model sees — the reason `move_segment` was
moved to L3 — and sorting can move a cache marker placed on a tool entry.

## Decision

- **Experiment buckets.** `bucket = first 4 bytes of fp("bucket", root_hash ‖
  experiment_id)`, read as an unsigned integer, mod 10000 — where `root_hash`
  is `fp("root", …)` for the session. Each experiment (one op, one policy
  version) gets an independent split.
- **Sub-agents inherit** their parent session's bucket when the parent is
  known (`parent_session_id`); otherwise they use their own root.
- **Rejection trigger.** An op is disabled only when the provider returns an
  invalid-request error for the transformed request **and** the untransformed
  resend succeeds — at least 3 times within 1 hour for the same provider and
  model. Auth errors, payload-too-large, context-length and server errors never
  count against an op.
- **`sort_tools`.** Allowed at L2 only when no tool entry carries a cache
  marker (or the marker is kept on the same tool after sorting) and the
  provider profile does not mark tool order as significant. Each provider
  profile states whether tool order affects behaviour; until the conformance
  kit confirms it does not, `sort_tools` runs in shadow mode only. *(Superseded
  by ADR 0021: the gate is a canary on gateway quality signals and cost.)*

## Consequences

- Experiments are independent, at the cost of a session being in different
  arms for different ops.
- Fewer false disables; a genuinely bad op takes a few failures to disable,
  each recovered by the resend.
- `sort_tools` may stay shadow-only for a provider until evidence shows tool
  order does not matter there.
