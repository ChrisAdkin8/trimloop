# ADR 0041: Compaction edge cases and the quartile rule

- Status: Accepted — built in the M0 prototype
- Date: 2026-11-08
- Amends: [ADR 0040](0040-position-pricing-and-one-method-page.md) (decision 4), [ADR 0039](0039-rebuilt-cache-and-likely-ranges.md) (decision 3), [ADR 0038](0038-real-size-after-compaction.md) (decision 6)

## Context

An eighth review found no wrong figures, and four small method gaps:

1. ADR 0040 decision 4 gave text logged *after* a compaction marker to the
   first reply after the compaction, but dropped text logged just *before*
   the marker (7 of 20 compactions, $0.07).
2. Two compactions with no reply between them kept only the second.
3. ADR 0039's quartiles used a different rule from the median, so with two
   or four compactions measured the likely saving sat on one end of its
   range; and the range note could pair a size with the clamped figure
   rather than the replay at that size.
4. The test for ADR 0040 decision 2 did not exercise it: its reply wrote
   nothing to the cache.

## Decision

1. Text logged around a compaction marker, before or after, goes with the
   first reply after the compaction (or with the late-logged triggering
   reply only when it came before the marker).
2. Every compaction no reply followed is kept, with its time, and counted
   in its own week.
3. Quartiles use the median's rule (the value at position q·n of the sorted
   list), so the median always lies between them; the range note gives the
   replay at each quartile as computed.
4. The status-line test uses the real reply's shape and fails on the
   reader before ADR 0040.

## Consequences

- No reported figure changes by more than a few cents on the founder's
  logs.
- [measurements.md](../measurements.md) now covers every figure the reports
  show, and defines its terms.
