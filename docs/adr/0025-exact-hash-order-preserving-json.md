# ADR 0025: Exact hash over order-preserving compact JSON, at every capture point

- Status: Proposed
- Date: 2026-11-05
- Amends: [ADR 0016](0016-m1-sidecar-operation.md) (gateway exact hash)

## Context

`exact_hash` was defined as "the segment's bytes exactly as serialised in the
request". Only the local proxy sees those bytes. Transcripts let the request
be rebuilt only largely, and gateway plugins see Python objects. ADR 0016
redefined the gateway variant as RFC 8785 canonical JSON, but RFC 8785 sorts
object keys, so unstable key order inside tool schemas — which changes the
prompt the provider caches — would produce the same hash and hide a real
cause of cache churn.

## Decision

- `exact_hash` is the keyed hash (`seg.exact`) of the segment's
  **order-preserving compact JSON**: the provider-format JSON value with
  object members in their original order, no insignificant whitespace,
  strings and numbers serialised as in RFC 8785, and cache markers removed.
- The same definition applies at every capture point:
  - **wire** (local proxy): the request bytes are parsed and re-serialised;
    whitespace between JSON tokens is dropped, which is not part of the prompt
    the model receives;
  - **gateway**: computed from the provider-bound request as LiteLLM holds
    it, member order preserved;
  - **transcript**: computed from the recorded request (`api-request-shape`
    and blobs) where the transcript has it; where it does not,
    `exact_hash` is null and P01 runs `reduced`.
- `content_hash` keeps RFC 8785 canonical form (sorted keys), so it still
  matches the same content regardless of key order.
- A difference between `exact_hash` and `content_hash` behaviour is itself a
  signal: same content hash, different exact hash means a reordering, which
  P01 reports as "key or block order changed".

## Consequences

- Exact hashes for the same request agree across capture points when the
  member order agrees, so `fidelity` no longer changes the meaning of
  `exact_hash`; it records only where the data came from.
- Key-order churn in tool schemas becomes visible everywhere.
- Test vectors cover member order explicitly.
