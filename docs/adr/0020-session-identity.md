# ADR 0020: Session identity that survives system-prompt churn

- Status: Proposed
- Date: 2026-11-05
- Amends: [ADR 0012](0012-gateway-sessions-and-parity.md), [ADR 0013](0013-decide-path-and-transform-safety.md), [ADR 0016](0016-m1-sidecar-operation.md), [ADR 0018](0018-rollout-refinements.md)

## Context

The third cold review found that gateway sessionising and canary buckets were
keyed on the **conversation root**, defined as the system segments plus the
first user message, and on prefix chains that start with the system segments.
P01's most common cause is a system prompt that changes every turn
(timestamps, git status). When it does, no request matches the previous one,
every turn opens a new session, P01 never sees the churn it exists to catch,
and the canary bucket changes every turn.

The "join on any shared prefix" rule (ADR 0016) also merged unrelated
sessions: the root alone is a prefix, so two agents with the same prompt and
first message — CI jobs, parallel sub-agents — became one session.

## Decision

**Session anchor.** Each session has an anchor:

1. the harness's own session id, when the request carries one (to be
   confirmed per harness at the gateway; transcripts always have it);
2. otherwise `fp("root", harness ‖ 0x00 ‖ principal ‖ 0x00 ‖ first_user)`,
   where `principal` is the gateway key alias (or developer id) and
   `first_user` is the content hash of the conversation's first `user_text`
   segment. System, tool-definition, memory-file and injected-context
   segments are **excluded**, so churn in them does not change the anchor.

**Joining a session.** A request joins open session *S* when it has the same
anchor **and** its history contains *S*'s most recent assistant response
(by content hash of that response's segments). Otherwise it opens a new
session.

- System-prompt churn no longer splits sessions: the message history still
  contains the previous response.
- History edits (cleared tool results, truncation) no longer break
  continuity, because the latest response survives them; the first
  divergence from *S*'s previous request is still recorded as
  `history_edit_at`.
- Identical-prompt sessions stay separate: a new conversation's first
  request contains no earlier response.
- If two open sessions would match (identical histories so far), the one
  with the most recent turn wins; this is deterministic and logged.
- Compaction replaces the history, so the request opens a new session linked
  by `resumed_from` when its anchor matches.

**Prefix chains for cache analysis** are unchanged: they still run over
system and tool segments first, because that is what the provider caches.
Sessionising and cache analysis now use different keys on purpose.

**Canary buckets** use the anchor:
`fp("bucket", anchor ‖ 0x00 ‖ experiment_id)`, `experiment_id` being the
UTF-8 string `<op>@<policy_version>`. Sub-agents inherit their parent's bucket
only when the harness supplies a link (a parent session id) visible to the
replica handling the request; otherwise they use their own anchor. This is
stated, not hidden, in rollout reports.

## Consequences

- P01 works at the gateway for the churn it targets.
- Buckets are stable per session across replicas when the anchor comes from
  a harness session id; with the fallback anchor they are stable as long as
  the first user message and principal are.
- Two sessions that genuinely share their first user message, principal and
  harness, and diverge only after an identical first response, are
  indistinguishable until they diverge — an accepted, rare case.
- The data model's `root` label and the analyzer's §6 rules change.
