# ADR 0043: Weekly figures for P02, P14 and main-session models

- Status: Accepted — built in the M0 prototype; labels and the meaning of `short_waits` (analysis version 6) amended by [ADR 0044](0044-effort-thinking-and-cache-figures.md)
- Date: 2026-11-08
- Amends: [ADR 0033](0033-reconciliation-history-standing-context.md) (what the history file keeps), [ADR 0042](0042-p02-p14-and-main-session-models.md) (how its cards are checked)

## Context

ADR 0042's cards are checked by their effect over the following weeks, but the weekly
figures (kept in the history file, ADR 0033) had no column for them, so a
change could not be seen week by week as it can for long breaks and tool
output. A review of the reports also found P02's top sessions unnamed in
the JSON and missing from the web page, and "effort level" undefined in the
page's word list.

## Decision

1. Each week also keeps, as numbers only: what the hour's cache would have
   saved after 5–60 minute waits (`short_waits`, P02), the output cost of
   xhigh and max turns (`top_effort`, P14), and main-session spend on a
   model with a newer version or a tier down (`older_models`, P13). The
   text and HTML weekly tables show them. Weekly figures are analysis
   version 5, so weeks stored before it are marked as older.
2. P02's top sessions are named like the others, in the JSON and on the web
   page; the page's word list explains "effort level".

## Consequences

- The weekly totals equal the findings on the founder's logs (2026-11-08):
  short waits $101, top effort $110, older models $214.
- The history file holds three more numbers per week; still no names,
  titles or paths (ADR 0004).
