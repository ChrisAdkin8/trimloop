# Contributing

Trimloop is an early prototype. The most useful thing you can give it now
is what happened when you ran it.

## What helps most

- **Tell us whether the advice was right.** Run `trimloop`, look at "What
  to do first", and say in
  [Discussions](https://github.com/ChrisAdkin8/trimloop/discussions)
  whether those changes were worth making. The first milestone is done when
  developers agree at least half of their top five are worth fixing
  ([ROADMAP.md](ROADMAP.md)).
- **Report a figure that looks wrong.** Open an
  [issue](https://github.com/ChrisAdkin8/trimloop/issues/new/choose) with
  "A figure looks wrong". How each figure is worked out is in
  [docs/measurements.md](docs/measurements.md).
- **Suggest a finding.** If Claude Code wastes tokens in a way trimloop
  does not report, open an issue with "An idea for a finding". The known
  patterns are in [docs/patterns.md](docs/patterns.md).
- **Report a bug** in the program, the install, the plugin, the status
  line or the hooks.

Security problems go to the private form, not an issue: see
[SECURITY.md](SECURITY.md).

## Before you paste a report

Reports show your project names and the titles Claude Code gives your
sessions, which summarise what you asked. `--no-titles` removes the
titles; project names stay. Paste the lines that matter, not a whole
report, and never a session log.

## Code changes

Pull requests from outside are not merged yet. They will need a
contributor licence agreement, which is not set up. Until it is, please
open an issue that describes the change; a small patch pasted into the
issue is fine to show what you mean, but it cannot be merged as it stands.

To build and check the code yourself, see
[docs/development.md](docs/development.md). Design decisions are recorded in
[docs/adr/](docs/adr/README.md).
