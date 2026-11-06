# ADR 0023: Content-addressed transport between plugin and sidecar

- Status: Proposed
- Date: 2026-11-05
- Amends: [ADR 0016](0016-m1-sidecar-operation.md) (delta fallback)

## Context

ADR 0016's overhead fallback had the plugin send only what is new since its
own last-sent prefix of a conversation. Turns of one conversation land on
different LiteLLM worker processes (and, without sticky routing, different
replicas), so "its own last-sent prefix" means nothing to the sidecar, and
recognising conversations would push session logic into Python. Decide
requests had the same size problem (ADR 0021).

## Decision

- The plugin splits each request into **messages** (system block, tool list,
  and each conversation message) and computes a plain SHA-256 **transport
  digest** of each message's JSON as LiteLLM holds it. Digests are transport
  identifiers only: never stored, never uploaded, not fingerprints.
- It sends the request as an ordered list of digests, attaching the full
  message only for digests it has not sent to this sidecar recently (a
  per-worker LRU).
- The sidecar keeps a **digest → message cache** shared by all workers in the
  pod. If a request references a digest it does not hold — a worker's LRU
  was wrong, or the sidecar restarted — it replies with the missing digests
  and the plugin resends those messages. Normally there is one round trip.
- The same protocol carries `/v1/events` and `/v1/decide`.
- All fingerprinting, segmentation, canonicalisation and redaction stay in
  the sidecar (ADR 0006). The plugin's extra work is splitting, hashing and
  bookkeeping: a few hundred lines more than before, still no detection or
  fingerprint logic.
- **Remote mode** (no co-located sidecar, ADR 0006) does not use this
  protocol. It requires the full fingerprint specification in Python and is
  documented as a heavier, less-tested fallback.

## Consequences

- Bytes sent per turn grow with new content, not with history length.
- The sidecar holds recent messages in memory; the cache is bounded and
  evicts by age.
- A missing-digest reply adds a second round trip; for decide, if that would
  exceed the plugin's timeout, the request proceeds untransformed.
