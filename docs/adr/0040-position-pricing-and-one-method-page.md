# ADR 0040: Removed history priced by position, and one page for the current method

- Status: Accepted — built in the M0 prototype; decision 4 amended by [ADR 0041](0041-compaction-edges-and-quartile-rule.md)
- Date: 2026-11-08
- Amends: [ADR 0035](0035-measurement-corrections.md) decision 2 (as built), [ADR 0039](0039-rebuilt-cache-and-likely-ranges.md) (decisions 1, 2 and 5), [ADR 0034](0034-live-nudges.md) (status line after a compaction), [ADR 0038](0038-real-size-after-compaction.md) (decisions 5 and 6, through ADR 0039)

## Context

A seventh review checked ADR 0039's pricing rule against a second method,
pricing each removed token by where it sits in the request, and found the
two agree on the founder's logs to within $7. It also found:

1. **ADR 0039's rule is all-or-nothing.** A turn that read a long cached
   history and wrote large new output (several big tool results) would
   have its removed history priced as written: 25–40 times too high on Opus
   5.5 (write against read price), 50–80 times on Fable 5.1.
   Nothing like it is in these logs, but nothing stops it.
2. **The status line still missed the compaction** when the reply that
   triggered it wrote a few tokens to the cache, as the real one did: the
   reader stopped at that reply.
3. **ADR 0039 changed three weekly figures without a new analysis version**,
   so weeks stored before it were not marked as older.
4. **Text re-attached after a compaction** went to the reply before it when
   that reply was logged late.
5. **Superseded figures in older ADRs were still called "corrected"**: each
   round's correction notes went stale the next round, and the current
   method was spread over five ADRs.

## Decision

1. **Removed history is priced by where it sits** (`removedCost`): just
   after the fixed start, read where the request read from the cache,
   written where it wrote. P19 and the compaction replay use it. Carried
   tool output and injected text keep ADR 0039's rule (`historyRate`); the
   two agree on these logs.
2. **The tail reader looks back to the previous reply** before deciding
   whether a compaction followed the latest one.
3. **Weekly figures are analysis version 4.**
4. **Text logged after a compaction marker goes with the first reply after
   the compaction**, even when the triggering reply is logged later.
5. **[measurements.md](../measurements.md) says how every reported figure is
   worked out today**, with its code and the ADRs behind each part. When a
   method changes, it changes with it. Older documents keep the figures of
   their day with a pointer to it and to log-signals §8, rather than a
   "corrected to" figure that goes stale.

## Consequences

- The figures barely move on the founder's logs (2026-11-08, $2,200 over
  44 days): P19 up to $673 (31 %); compacting earlier likely about $535
  ($513–$644), up to $824.
- A turn like the one in point 1 no longer inflates P19 or the replay.
- New readers start from measurements.md; the ADR chain explains why.
