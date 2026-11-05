# ADR 0017: Store segments once, content-addressed

- Status: Proposed
- Date: 2026-11-05

## Context

Every request repeats the conversation so far. Storing segment records per
turn — and the prefix-hash arrays on every turn — makes storage and uploads
grow roughly with the square of session length: in a 50-turn session the
first segments are stored about 50 times. This affects the M0 local store as
well as M1+ uploads.

## Decision

- A **segment** is stored once, keyed by `(fp_key_id, exact_hash)`, with its
  kind, sizes, token count, content hash and line hashes. Identical segments
  across turns, sessions and developers share one record.
- A **turn** stores an ordered list of segment references (compact ids) plus
  the positions of cache breakpoints, instead of embedding segments.
- Prefix chains are stored **incrementally**: a turn records the prefix-chain
  values only for segments it adds beyond the longest prefix it shares with
  the session's previous turn, plus the shared length.
- Uploads (sidecar to collector, developer machine to collector) send each
  segment record once per collector and turns as reference lists.

## Consequences

- Storage and upload volume grow with the amount of new content, not with
  turns × history length.
- Detectors read segments through the turn's reference list; the store API
  hides the indirection.
- Segment records outlive the turns that first used them, so retention
  deletes segments only when no retained turn references them.
- The data model's Turn and Segment tables change before M0 storage is built.
