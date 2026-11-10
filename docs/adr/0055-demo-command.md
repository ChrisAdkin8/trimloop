# ADR 0055: A demo command on made-up logs

- Status: Accepted — built: `trimloop demo`, `internal/demo`, `scripts/gen-demo.py`
- Date: 2026-11-10
- Related: [ADR 0032](0032-readable-reports.md) (what reports show), [ADR 0033](0033-reconciliation-history-standing-context.md) (the history file), [ADR 0004](0004-local-first-hashes-only.md) (privacy)

## Context

Someone who has just installed trimloop often has few Claude Code logs, or
none. The README's way to try it without logs copied one sample session
(`testdata/sessions/tools.jsonl`) into a scratch folder. Its report reads
"1 session, 6 turns, $0.07" and has no findings, so it shows nothing of
what trimloop is for. The golden logs (`testdata/golden`) are no better as
a demo: they hold deliberate edge cases (bad lines, orphan sub-agents,
unpriced turns) that would read as bugs.

## Decision

1. **Made-up logs for one invented developer.** `scripts/gen-demo.py`
   writes five weeks of logs (5 October to 6 November 2026) on four
   made-up projects, laid out as Claude Code keeps them, into
   `internal/demo/data`. It is deterministic and uses Python's standard
   library only. The logs are shaped so that every main section of the
   report has something in it, and so that "What to do first" leads with
   compacting earlier, starting fresh after a long break, and running
   sub-agents on a cheaper model. They are billed per token (no plan file),
   about $150 at list prices, with no bad lines and no unpriced turns.
   Every name, path and text is invented. Token counts are in the usage
   fields, so the text stays short: the folder is about 1.5 MB.
2. **Embedded in the binary.** `internal/demo` embeds the folder and holds
   the demo's clock, `demo.Now`: 7 November 2026, 09:00 UTC, the morning
   after the last reply.
3. **`trimloop demo` runs the normal analysis on them.** It takes
   analyse's options (`--format`, `--out`, `--open`, `--since`,
   `--no-titles`). It copies the logs to a private temporary folder, which
   it removes when it ends, and runs the same code path as `trimloop`, with
   these differences:
   - the history file is never read or written; `--history` and
     `--no-history` are refused;
   - nothing of the user's is read: not their logs, plan, usage-limit
     readings or log retention setting;
   - the report's clock is `demo.Now`, and weeks are in UTC, so the report
     is the same on every machine and date; `--since 7d` counts back from
     `demo.Now`;
   - a one-line banner on standard error says the logs are made up, so
     JSON on standard output stays valid.
4. **Pointers to it.** The no-logs message, `trimloop --help`, the
   unknown-command hint, the README and the plugin's skill mention
   `trimloop demo`; the skill says the figures are invented.
5. **The data cannot drift from its script.** `scripts/check.sh` has a
   "demo drift" step, like golden drift: it regenerates the logs into a
   scratch folder and fails on any difference.

## Consequences

- A new user sees a full report, with findings, before trusting trimloop
  with their own logs.
- The binary is about 1.5 MB larger.
- The demo's figures change when a measurement method changes, like
  anyone's; `TestDemoSummary` holds its summary, so such a change shows
  up there too. When it does, check the new summary still leads with the
  three actions above, and adjust `scripts/gen-demo.py` if it does not.
- Dates in the summary use the machine's local zone, as everywhere in the
  report; the demo's first and last replies fall between 10:00 and 11:00
  UTC, so they print the same in zones from UTC−10 to UTC+13. Clock times
  in the full report and the web page still show local time.
