# ADR 0044: Effort by thinking tokens, and fairer cache figures

- Status: Superseded by [ADR 0045](0045-m0-measurement-method.md) (kept as history). Was: Accepted — built in the M0 prototype
- Date: 2026-11-08
- Amends: [ADR 0042](0042-p02-p14-and-main-session-models.md) (decisions 1 and 3), [ADR 0043](0043-weekly-trend-for-new-findings.md) (labels, the meaning of `short_waits`, analysis version 6), [patterns.md P14](../patterns.md#p14--thinking-budget-overspend) and [analyzer.md](../analyzer.md) (what P14 requires in M0: the effort level, with thinking tokens where logged)

## Context

A ninth review found three of ADR 0042's figures unfair:

1. **P14 clipped each turn at high's mean.** Output varies so much between
   turns that the same rule "saves" 43 % of high's own output cost; the
   $77 was mostly variation, not effort. It also used all output, though
   Claude Code logs thinking tokens (`output_tokens_details`) on most
   turns, and thinking is what the effort level controls.
2. **P02's saving was "what the hour's cache would have saved"**, but the
   hour makes every other write dearer: net, about $65 of the $101. The
   card's advice (keep slow commands out of sub-agents) avoids the wait
   rather than changing the cache, so its saving is the cost of the expiry.
3. **"The hour saved $761"** treated any logged wait over 5 minutes as
   expiring a 5-minute cache; but the logged wait includes the next reply's
   own time (22 % of sub-agents were still cached after such waits), and it
   skipped turns that read without writing.
4. A tenth review then found that **P14 compared models, not only levels**
   (every xhigh turn was Fable 5.1, most high turns Opus 5.5, and Fable
   thinks more at every level), and that **the P02 card's "sub-agents use
   the 5-minute cache, main sessions the hour's" was false**: 304 of 421
   main sessions, all run through the Python SDK, used the 5-minute cache.

## Decision

1. **P14 compares averages within each model**: for xhigh and max on each
   model, the mean thinking tokens per turn above the same model's high,
   times the level's turns, at its output price. Means are over the turns
   that record thinking (older Claude Code versions do not), applied to all
   the level's turns; output where most turns do not record it. A model
   needs at least 30 high-effort turns from at least 3 sessions to compare
   with; turns on others are left out and counted. The card advises coming
   back to the level most turns ran at (high, if that is itself xhigh or
   max), not to high: for someone usually on medium, "set high" would raise
   most turns. It names the models the xhigh and max turns ran on, since
   Claude Code can set effort per model (`modelSettings`), as the founder's
   settings did for Fable 5.1.
2. **P02's saving is what the expiry cost**, against the cache still being
   there at the 5-minute price. The report also gives what putting every
   session on the 5-minute cache on the hour's would have saved, net of its
   dearer writes: over all of them, as the lifetime is chosen before anyone
   knows which will wait (ADR 0027 found it costs more than it saves).
   A session that wrote with both lifetimes counts as on the hour and is
   left out of that option.
3. **The hour against 5 minutes is a range**, counted from each session's
   first write on the hour: the high end counts every wait over 5 minutes
   as expiring, the low end only waits over 10 minutes. The report says
   what each end assumes; neither is exact, and the low end is not a floor
   (about 14 % of sub-agent waits of 10–60 minutes kept their cache). Not
   every wait over 5 minutes would have lost a 5-minute cache: the logged wait includes the
   next reply's own time, and requests the logs do not record can keep a
   cache alive (some sub-agent caches survived 35–48 minutes). A single
   "likely" figure, from expiry rates measured on 50 and 61 sub-agent waits
   and applied to 577 main-session waits, was tried and dropped: too thin
   a sample for a headline.
4. The P02 card says which kinds of session used which cache, counted from
   the logs, and advises running slow commands where the cache lasts an
   hour (or, if none did, not waiting on them). Weekly short waits change
   meaning to the expiry cost, so weekly figures are analysis version 6;
   columns added in version 5 show a dash for weeks stored before it;
   "older models" is labelled "cheaper choice". The cards name real
   controls (`/effort`, `effortLevel`), say what overlaps, and P02's has a
   check.

## Consequences

On the founder's logs (2026-11-08): P14 up to $41 (was $77; $47 before it
compared within each model), advising a return to medium; P02 $102, and
putting every 5-minute session on the hour's cache would have cost $22
more; the hour's cache saved the sessions that used it $380–$825.
