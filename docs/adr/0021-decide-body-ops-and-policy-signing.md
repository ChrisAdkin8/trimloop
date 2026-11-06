# ADR 0021: Decide body format, the op catalogue, and policy signing

- Status: Proposed
- Date: 2026-11-05
- Amends: [ADR 0013](0013-decide-path-and-transform-safety.md), [ADR 0018](0018-rollout-refinements.md)

## Context

The third cold review found:

1. **Two body formats.** `/v1/events` carries the provider-bound request
   (after LiteLLM's translation, ADR 0016), but LiteLLM's pre-call hook sees
   the request **before** translation, in the client's format. Anchors,
   segment indexes and ops computed for decide could disagree with those from
   the observe path. It is also unconfirmed whether pass-through routes (for
   example Claude Code on `/v1/messages`) run the pre-call hook at all.
2. **An impossible gate.** `sort_tools` could leave shadow mode only when the
   conformance kit showed tool order does not matter; the kit cannot measure
   model behaviour.
3. **Fixes without ops.** L3 fixes for P04, P08, P13 and P14 (truncation,
   elision, model routing, thinking caps) had no operation in the closed op
   set, and P08 was scheduled for M3 while L3 ships in M4.
4. **Latency.** Sending a whole request of up to about 1 MB to decide may not
   fit 25 ms on LiteLLM's event loop.
5. **Policy signing.** Policies are "signed by the collector", but nothing
   says who holds the signing key. In hosted mode that would let the vendor
   authorise changes to customer traffic.

## Decision

1. **Decide works on the client-format request**, the one the pre-call hook
   receives. Ops are defined per API surface (Anthropic Messages, OpenAI Chat
   Completions, OpenAI Responses) and applied to that format; the sidecar
   parses it with the matching provider parser. The anchor (ADR 0020) is
   computed from message content, which is the same before and after
   translation, so observe and decide agree on sessions and buckets. Segment
   indexes in an op refer to the client-format request. Whether the
   pre-call hook runs on pass-through routes is checked in the M1 benchmark;
   if it does not, decide is unavailable on those routes and says so.
2. **`sort_tools` gate.** It leaves shadow mode through an ordinary
   canary on gateway quality signals and cost (ADR 0013), recorded per
   provider and model. No conformance-kit test is required.
3. **Op catalogue.** The closed set is:

   | Op | Level | Status |
   |---|---|---|
   | `sort_tools` | L2 | M3 |
   | `set_cache_breakpoint` | L2 | M3 |
   | `set_cache_ttl` | L2 | M3 |
   | `set_cache_key` | L2 | M3 |
   | `move_segment` | L3 | M4 |
   | `truncate_tool_result` (head + tail + retrievable pointer) | L3 | M4 |
   | `elide_segment` (superseded content, with a marker) | L3 | M4 |
   | `route_model` | L3 | M4 |
   | `cap_thinking` | L3 | M4 |

   Until an op ships, the pattern's fix is an **L1 recommendation only**. P08
   ships in M3 with an L1 fix; its L3 fix arrives with `elide_segment` in M4.
4. **Latency.** Decide requests use the content-addressed transport of
   ADR 0023, so they normally send references, not whole bodies. The M1
   benchmark includes a decide round trip; the 10 ms server and 25 ms plugin
   budgets are confirmed or revised from it before M3.
5. **Policy signing.** Policies are signed by a **customer-held policy key**,
   separate from the fingerprint key, created by `trimloop policy key init`
   and stored in the customer's secret manager. Sidecars and proxies carry the
   public key as their trust anchor and refuse unsigned or unknown-signer
   policies. The collector stores and distributes policies but cannot sign
   them; in hosted mode the vendor therefore cannot authorise changes to
   customer traffic. Rotating the policy key means distributing a new trust
   anchor; the old one is accepted for a stated overlap period.

## Consequences

- Ops have two definitions where formats differ (for example Anthropic
  `cache_control` placement in Messages format versus OpenAI cache keys).
- Decide may be unavailable on some LiteLLM routes; reports show it.
- Teams need a second key for policies, but only from M3.
