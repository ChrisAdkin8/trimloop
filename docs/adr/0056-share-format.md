# ADR 0056: A report format that is safe to share

- Status: Accepted — built: `trimloop --format share`, `report.WriteShare`
- Date: 2026-11-10
- Related: [ADR 0032](0032-readable-reports.md) (what reports show, and `--no-titles`), [ADR 0004](0004-local-first-hashes-only.md) (privacy), [ADR 0055](0055-demo-command.md) (the demo, which the test of this format runs on)

## Context

Reports show session titles and project names
([ADR 0032](0032-readable-reports.md)). `--no-titles` hides the titles, but
project, skill, plugin and server names stay, so showing a report to
anyone else means reading it through first. People who want to say "this
is what trimloop found for me", in a chat or a post, had nothing they
could paste without that check.

## Decision

1. **A fifth format, `share`.** `trimloop --format share` prints a few
   lines: the spend and the period's length, the share of spend that went
   on turns over the large-session threshold, the three actions that would
   save the most with their savings, and a line naming trimloop, its
   version and its address. It renders the same `report.Results` as the
   other formats.
2. **Figures and trimloop's own wording only.** The format leaves out
   everything the report took as text from the user's machine: project
   names, session titles and ids, folder paths, and the names of skills,
   plugins, MCP servers and agents. It also leaves out the dates (only
   the number of days is given) and the plan's name (a subscriber's
   summary says "a flat-fee plan"). An action's title is trimloop's own
   sentence; the only names in one are Claude models.
3. **A new line may only use what item 2 allows.** `WriteShare` reads an
   action's title and figures, never its finding, examples or fix text,
   which can quote the logs.
4. **Written like the summary.** With `--out FILE` it goes to a file
   readable by the owner only, and no second summary is printed. A file
   name ending in `.txt` suits it.

## Consequences

- A user can show their figures without checking a report line by line.
- The share format says less than the summary: it has no session to point
  to and no dates.
- `TestDemoShare` holds the format's output on the demo logs and fails if
  a project name, session id or title from the demo report appears in it.
  It cannot prove that a future line quotes nothing: item 3 is the rule
  for whoever adds one.
