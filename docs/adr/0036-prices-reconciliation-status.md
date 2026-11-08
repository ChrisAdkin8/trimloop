# ADR 0036: Prices checked against the pricing page, and what "reconciliation done" means in M0

- Status: Accepted — built in the M0 prototype; decision 3 amended by [ADR 0037](0037-partial-weeks-prompts-compaction-nudges.md)
- Date: 2026-11-08
- Amends: [ADR 0028](0028-harness-recorded-signals.md) (the reader's acceptance test), [ADR 0031](0031-m0-order-from-measured-spend.md) (item 5), [ADR 0033](0033-reconciliation-history-standing-context.md) (history, P07 sources), [ADR 0034](0034-live-nudges.md) (countdown), [ADR 0035](0035-measurement-corrections.md) (history)

## Context

A third review of the M0 prototype raised six questions:

1. **Is Sonnet 5.5's cache-read price wrong?** Claude Code's cost records
   show $0.18 more for Sonnet 5.5 than the logs, on identical tokens: they
   price its cache reads at $0.20 per million, the catalogue at $0.10.
2. **Is reconciliation done?** ADR 0028 made "within 2 % of the harness
   record, per session and token class, auxiliary calls included" the
   reader's acceptance test. ADR 0031 marks reconciliation done, but the
   auxiliary turns are not built and the measured gap is about 13 %.
3. **Weeks kept at old prices.** ADR 0035 replaces a stored week worked out
   by an older analysis; a week priced with an older catalogue was kept.
4. **The compaction card was shown or hidden on the wrong figure**: the
   cost of sessions never compacted, not the saving it shows.
5. **Standing-context fixes named things the user cannot change**: the
   desktop app's own MCP servers, servers that plugins bring (removed with
   the plugin, not `claude mcp remove`), and skills that were used.
6. **The cache countdown assumed an hour after a reply that wrote nothing
   to the cache**, even in a session that writes 5-minute entries.

## Decision

1. **The pricing page is the source for prices.** The catalogue was checked
   row by row against platform.claude.com/docs/en/about-claude/pricing on
   2026-11-08. Its footnotes price cache reads at 0.05× input on Opus 5.5
   and Sonnet 5.5 and 0.025× on Fable 5.1, so Sonnet 5.5 stays at $0.10.
   Where Claude Code's records use another price, the difference shows as
   reconciliation gap; it is $0.18 on these logs. Opus 5.5 and Fable 5.1
   records match the page. The still-served models from 4.6 on are added
   (Fable 5, Opus 4.8 with fast mode at 2×, Opus 4.7 and 4.6, Sonnet 4.6),
   with `claude-haiku-4-5` beside its dated id; from 4.6 on, only Haiku 5.5
   has a long-prompt tier. The catalogue version is
   `anthropic-list-2026-11-08`.
2. **In M0, reconciliation reports the gap; it does not pass ADR 0028's
   test.** Without auxiliary turns the logs cannot come within 2 %, so the
   report shows the gap as spend the logs do not record. The 2 % test stays
   the target for when auxiliary turns are built (after M0). A record that
   says it could not price some usage (`hasUnknownModelCost`) is left out
   of the comparison, like a session with an unpriced turn.
3. **A stored week priced with another catalogue is replaced** while the
   logs still hold it, as ADR 0035 does for an older analysis.
4. **Every card is shown when its saving ("up to") reaches the minimum**,
   the compaction card included.
5. **Standing context is sorted by who can change it.** MCP servers a
   plugin brings go with skills and plugins (fixed with `/plugin`); the
   desktop app's own servers (`ccd_…`) and skills that were used go with
   the harness, which has no setting to change.
6. **The status-line countdown keeps the cache lifetime of the latest reply
   that wrote to the cache** when the latest reply wrote nothing
   ([ADR 0034](0034-live-nudges.md)); with no such reply in the log's tail,
   it assumes the hour, as before.

## Consequences

On the founder's logs (2026-11-08, $2,149 over 44 days):

- Claude Code's records show 13.2 % ($202 on 409 sessions) more than the
  logs; 4 sessions are left out as not covered.
- Running sub-agents on Sonnet 5.5 would save up to $281.
- The skills-and-plugins card is $37 and the MCP card $19.
- ROADMAP, data-model.md and log-signals.md say reconciliation is reported,
  not passed; earlier figures keep dated notes.
