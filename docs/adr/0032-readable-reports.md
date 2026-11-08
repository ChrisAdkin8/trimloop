# ADR 0032: Readable reports — session titles, likely savings, plain language

- Status: Accepted — built in the M0 prototype; amended by [ADR 0035](0035-measurement-corrections.md), [ADR 0037](0037-partial-weeks-prompts-compaction-nudges.md) and [ADR 0038](0038-real-size-after-compaction.md) (compaction replay); method summarised and amended by [ADR 0045](0045-m0-measurement-method.md); what reports lead with for subscribers: [ADR 0048](0048-usage-limits-for-subscribers.md)
- Date: 2026-11-07
- Amends: [ADR 0004](0004-local-first-hashes-only.md) (what a local report may show), [ADR 0030](0030-how-fixes-are-chosen.md) §2 (ranking)

## Context

The M0 report gives correct figures, but a review of it for readability
found that readers could not act on several of them:

- Sessions were shown as identifiers (`024a1471-…`), which nobody
  recognises.
- Savings were upper bounds only. The largest action, compacting earlier,
  quoted P19's bound, which is not a bound for that fix: compacting shrinks a
  session far below the P19 threshold, so a replayed estimate came out
  higher than the "up to" figure beside it.
- Long pauses (P09), 17 % of spend, had no action of their own.
- Terms such as "cache writes, 1 hour", "carried", "shape" and pattern codes
  were unexplained, and some names were internal (`claude-sonnet-5-5`, MCP
  servers shown as identifiers).
- The default text output was about 200 lines, ordered by pattern rather
  than by what to do.

Claude Code writes a title for most sessions (`ai-title`, 350 of 422 main
sessions in the founder's logs), and the user can set one (`custom-title`).
A generated title summarises the conversation, so it is derived from
content.

## Decision

1. **Session titles in local reports.** Reports name sessions by title,
   project and start date, with the identifier in small print. A title the
   user set wins over generated ones; for a sub-agent, its main session's
   title is used. Titles are content-derived, so they are shown only in
   reports written on the developer's machine, and `--no-titles` leaves them
   out (including from JSON). Anything Trimloop stores or sends never carries
   titles unless a later decision allows it; ADR 0004 stands for stored and
   uploaded data.
2. **A likely saving beside the upper bound.** Where a fix can be replayed
   over the logged turns, a recommendation carries a likely saving, how it
   was worked out, and an upper bound from the same replay, so likely never
   exceeds it:
   - *Compact earlier (P19):* each session is replayed as if compacted to the
     typical post-compaction size (the median from compaction records, else
     30 k) whenever it passed 200 k, including each compaction's own cost,
     assuming the session then went the same way. (Since
     [ADR 0038](0038-real-size-after-compaction.md): the measured size of the
     first request after a compaction, else 80 k, and main sessions only.)
   - *Start fresh after a long break (P09):* each turn after a pause of over
     an hour is priced as if it had started a new session of the typical
     first-request size.
   Actions are ranked by the likely saving where there is one, else by the
   upper bound (amends ADR 0030 §2 until measured outcomes exist). Amended
   by [ADR 0035](0035-measurement-corrections.md): ranked by the upper bound.
3. **P09 gets an action.** "Start fresh after a long break", with compacting
   *before* the break as the alternative: compacting after it re-sends the
   whole conversation to an expired cache.
4. **Plain language.** Action titles say what to do; each card says what
   was found in a sentence; pattern codes are small print; models, MCP
   servers and agent types get display names; the page ends with a short
   glossary; amounts over $10 are whole dollars.
5. **Report period.** Every format states the dates the logs cover and the
   spend per week.
6. **A short default.** With no `--format`, the terminal gets the summary
   (spend, period, the top three actions); `--format text` gives the full
   text report, `html` the page and `json` every figure.

## Consequences

- Local reports now show content-derived text (titles). The page's footer
  and the README say so; `--no-titles` exists for reports that will be
  shared.
- Likely savings are model estimates, labelled as such, with their method on
  the card. They need checking against measured outcomes once results are
  kept between runs (ADR 0031 item 7).
- The compaction card's "up to" is no longer P19's bound; the P19 section
  keeps its own figure for "keeping every turn under 200 k".
- JSON gains `period`, `labels`, `p09_fix`, and per-recommendation
  `finding`, `likely` and `likely_note`; the format version stays
  `trimloop-report/1`, since fields were only added.
