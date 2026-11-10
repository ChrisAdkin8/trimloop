# Trimloop

[![CI](https://github.com/ChrisAdkin8/trimloop/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/ChrisAdkin8/trimloop/actions/workflows/ci.yml)
[![Licence: Apache-2.0](https://img.shields.io/badge/licence-Apache--2.0-blue)](LICENSE)
[![Go 1.27.3 or later](https://img.shields.io/badge/go-1.27.3%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Platforms: macOS and Linux](https://img.shields.io/badge/platforms-macOS%20%7C%20Linux-lightgrey)](#install)
[![Status: early prototype, M0](https://img.shields.io/badge/status-early%20prototype%20%28M0%29-orange)](ROADMAP.md)
[![Claude Code plugin](https://img.shields.io/badge/Claude%20Code-plugin-D97757)](#ask-claude-about-it-claude-code-plugin)

Trimloop reads the session logs Claude Code keeps on your machine, works out
where your tokens went, and tells you which changes to how you work would save
the most money.

**Status: early prototype** (milestone M0 in the [roadmap](ROADMAP.md)).
Today it analyses Claude Code logs and gives live nudges inside Claude
Code. It recommends; it does not change anything for you. The wider
product (gateway plugin, team collector, automatic fixes) is design only:
see [The plan](#the-plan).

## What you get

A run on one developer's 46 days of logs, shortened (the full output also
covers their usage limits):

```text
Read 611 logs: 594 sessions, 20,256 turns.
Spent $2,465 at list prices from 25 Sep to 10 Nov 2026 (46 days), about $375 a week.
Claude Code's own records show $221 (11%) more than its logs: requests the logs do not record.

What to do first (these savings overlap, so do not add them):
  1. Compact long sessions earlier: likely about $618 ($585–$680), up to $947 (38%)
     Compact when a session passes about 200,000 tokens, or between tasks.
  2. Run sub-agents on a cheaper model: up to $337 (14%)
     Run sub-agents that search, read and summarise on Sonnet 5.5.
  3. Start fresh after a long break: likely about $302, up to $312 (13%)
     After a break of over an hour, start a new session with a short note instead of carrying on.
```

In plain words: every turn re-sends the whole conversation, so long
sessions get expensive. *Compacting* (`/compact` in Claude Code) replaces
the conversation so far with a short summary. *Sub-agents* are helpers
Claude starts for side tasks, such as searching the code.

The full report explains each finding, shows the sessions behind it, and
gives the fix step by step. **See one:**
[docs/examples/example-report.html](docs/examples/example-report.html) is the
web page from the same run, with session titles left out and project
folders renamed to neutral words (oak, larch, ...). GitHub shows it as
code: download it ("Download raw file") and open it in a browser.

**How far to trust the figures.** They are estimates:

- Costs are at Anthropic's *list prices*: what the tokens would cost if you
  paid per token, not your bill.
- On a Pro or Max plan you pay a flat fee instead. The report says so and
  counts how often you hit a usage limit. Once trimloop's status line is set
  up ([Live nudges](#live-nudges-in-claude-code)), it also shows, in time,
  each saving as a share of your weekly limit
  ([ADR 0048](docs/adr/0048-usage-limits-for-subscribers.md); ADRs are the
  project's [decision records](#documents)).
- "Up to" is the most a change could save. "Likely", where an action has
  it, replays your logs as if you had made the change.
- Savings overlap (the same tokens can be saved by several changes), so
  they are never added up.
- Claude Code makes some requests its logs do not record. The report says
  how much: 11% in the run above.
- Quality is not measured. Check a change marked "untested for quality" (a
  cheaper model, lower effort) after you make it.

How each figure is worked out is in [docs/measurements.md](docs/measurements.md).

## What it looks for

| Finding | What it means |
|---|---|
| Long sessions (P19) | Every turn re-sends the whole conversation, so big sessions get expensive. When to compact or start fresh. |
| Long breaks (P09) | After an hour away the saved copy of the conversation (the *cache*) expires, and the whole session is paid for again. |
| Short waits (P02) | Sessions whose cache lasts only 5 minutes (sub-agents, and Claude Code run from a program) losing it during a wait of 5 minutes to an hour, on a slow command for example. |
| Tool output (P04) | Large command output and file reads that stay in the conversation and are re-sent on every turn. |
| What every request carries (P07) | The system prompt, tool definitions, skill lists and memory files sent with each request. |
| Model choice (P13) | Sub-agents and sessions on a more expensive model than the work needs, or on an older, dearer version. |
| Effort level (P14) | Turns at the highest effort levels that think far more than usual, and thinking's share of all output spend. |
| Model switches (P01) | Changing model part-way through a session, which saves the whole conversation to the cache again. |
| Replies cut off (P12) | Replies cut off when the computer slept (or the network failed), so the request was sent again. |

It also checks its totals against Claude Code's own cost records. It shows
spend by project, by skill, by plugin and by MCP server (the add-on tools
Claude Code connects to). And it keeps week-by-week figures, so you can
see whether a change worked. The codes (P19 and so on) refer to
[docs/patterns.md](docs/patterns.md).

## Install

No release is published yet, so for now build it from source, with Go
1.27.3 or later:

```bash
git clone https://github.com/ChrisAdkin8/trimloop.git
cd trimloop
go build -o ~/.local/bin/trimloop ./cmd/trimloop
```

Once releases are public, either of these will work instead:

- **Homebrew** (macOS or Linux): `brew install --cask chrisadkin8/tap/trimloop`
- **Download** the archive for your system from the
  [releases page](https://github.com/ChrisAdkin8/trimloop/releases), unpack
  it, and put `trimloop` somewhere on your PATH (for example
  `~/.local/bin`). [Checking a download](#checking-a-download) says how to
  verify it, and what to do if macOS refuses to open it.

Check it worked with `trimloop --version`. If the shell says "command not
found", the folder is not on your PATH (the list of folders the shell
looks in for commands; on macOS `~/.local/bin` is not on it by default).
Add it, then open a new terminal:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
```

(Use `~/.bashrc` if your shell is bash.) Or run it as
`~/.local/bin/trimloop` meanwhile.

### Checking a download

Download `checksums.txt`, `checksums.txt.sigstore.json` and your archive
into one folder. First check that the release workflow signed the
checksums (put the release's tag in place of `v0.1.0`):

```bash
cosign verify-blob \
  --certificate-identity 'https://github.com/ChrisAdkin8/trimloop/.github/workflows/release.yml@refs/tags/v0.1.0' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  --bundle checksums.txt.sigstore.json checksums.txt
```

Then check the archive against them:

```bash
shasum -a 256 --check --ignore-missing checksums.txt
```

The macOS binaries are not yet notarised by Apple, so macOS may refuse to
open a downloaded one until you clear its quarantine flag
(`xattr -d com.apple.quarantine ./trimloop`); do that only after both
checks pass. The Homebrew cask does it for you.

## Use it

```bash
trimloop                          # short summary, as above
trimloop --open                   # the full report as a web page, opened in your browser
trimloop --format text            # the full report as text
trimloop --format json --out ~/trimloop.json   # every figure, for scripts
```

It reads the logs in `~/.claude/projects` (or in `projects` under
`CLAUDE_CONFIG_DIR`, if you set that) and sends nothing anywhere. The
web page is saved as `trimloop-report.html` in the current folder;
`--out` puts it elsewhere (`--out -` prints it instead), replacing any
file already there. `trimloop help` lists every option (`trimloop setup
--help` lists setup's).

### Did a change work?

Add `--since` with the day you made the change:

```bash
trimloop --since 2026-11-01       # sessions started on or after 1 November
trimloop --since 7d               # sessions started since midnight 7 days ago (2w: two weeks)
```

The report then covers only sessions started from then on, with their
sub-agents. Each run also keeps weekly figures (numbers only) in
`~/.trimloop/history.json` and shows them in the full report's "Week by
week" section, so you can compare weeks even after Claude Code has deleted
old logs; with more than one Claude Code folder (`CLAUDE_CONFIG_DIR`), each
gets its own history file. A `--since` run leaves that file alone, so a
partial view never replaces full weeks.

### Before you share a report

Reports show the titles Claude Code gives your sessions, which summarise
what you asked, and your project names. Check a report before sharing it.
`--no-titles` removes the titles, and the web page then shows no session
ids, nor the ids of MCP connectors that have no name; project, skill and
server names stay.

### Try it without your own logs

The repository has a sample session. `CLAUDE_CONFIG_DIR` tells trimloop to
read another folder laid out like `~/.claude`:

```bash
mkdir -p /tmp/trimloop-demo/projects/demo
cp testdata/sessions/tools.jsonl /tmp/trimloop-demo/projects/demo/
CLAUDE_CONFIG_DIR=/tmp/trimloop-demo trimloop --no-history --format text
```

`--no-history` keeps the demo out of your weekly figures.

## Ask Claude about it (Claude Code plugin)

The trimloop plugin adds a skill to Claude Code. Ask "where did my tokens
go?" or "why do I keep hitting my usage limit?", and Claude runs trimloop
and explains the result in plain words. It needs trimloop
[installed](#install). In Claude Code:

```text
/plugin marketplace add ChrisAdkin8/trimloop
/plugin install trimloop@trimloop
```

Or, from a terminal, in one step:
`claude plugin install trimloop --marketplace ChrisAdkin8/trimloop`. The
plugin changes no settings ([ADR 0049](docs/adr/0049-claude-code-plugin.md)).

## Live nudges in Claude Code

Trimloop can also nudge you while you work:

- a *status line* (the line Claude Code shows below your prompt) with the
  session's size, its cost, and the effort level when it is set to max or
  xhigh; in the cache's last 10 minutes it also counts down to the expiry,
  and after it shows what the next message will cost;
- a message when a session passes 200,000 and 500,000 tokens;
- after a break long enough for the cache to expire (an hour, or 5
  minutes for a session on the 5-minute cache), in a session over 500,000
  tokens, a pause before your first message, once, showing what sending it
  will cost. The pause stops that message: it is not sent, and you send it
  again to go ahead (or `/clear` first to start fresh). It also applies to
  scripted runs (`claude -p`) of such a session.

The messages come from *hooks*: commands Claude Code runs after each reply
and before each message. With trimloop [installed](#install):

```bash
trimloop setup                                      # print the settings to add, and the file
trimloop setup --write <the file setup named>       # or add them for you
trimloop setup --check                              # is everything set up and working?
```

With `CLAUDE_CONFIG_DIR` set, Claude Code's settings file is
`settings.json` in that folder: `trimloop setup` names the right one.

`setup --write` saves a backup of your settings in `~/.trimloop/backups`
first and says how to put it back. If you already have another status
line, it changes nothing at all, hooks included. You can then add
`--replace-statusline` to replace yours. Or keep yours and add only
trimloop's hooks by hand: `setup --write` (and `setup --check`) prints them,
to add alongside any hooks you already have. `setup --check` changes
nothing. New Claude Code sessions pick up the change. To undo,
copy the backup back, or delete trimloop's `statusLine` and `hooks`
entries. Details:
[ADR 0034](docs/adr/0034-live-nudges.md).

## Files it writes

Everything stays on your machine, readable by you only:

| File | What it holds |
|---|---|
| The report you ask for (`--out`, or `trimloop-report.html`) | Figures, project names and session titles |
| `~/.trimloop/history.json` (or `history-<id>.json`) | Weekly figures only: no names, titles or paths |
| `~/.trimloop/live/<session>.json` | Which nudges a session has been shown |
| `~/.trimloop/nudges.jsonl` | Each nudge shown: time, kind, session size |
| `~/.trimloop/limits.jsonl` (or `limits-<id>.jsonl`) | Pro and Max only: how much of your 5-hour and weekly limits was used, each time it changed |
| `~/.trimloop/backups/settings.json-<id>.trimloop-backup-<time>` | A copy of your Claude Code settings from before each `setup --write` that changed them; it can hold API keys from those settings. Delete old ones when you no longer need them |

## The plan

This section is design, not built yet, apart from the parts the
picture below marks M0.

Teams spend real money on coding agents (Claude Code, Codex, Cursor, Copilot)
and on gateways that route their requests (LiteLLM, Portkey, Helicone,
OpenRouter). The tools we have seen answer *where* the money went; few say
*why it was wasted* or *what to change*. Trimloop is designed to join four
steps:

1. **Capture** requests from any coding agent or gateway.
2. **Detect** waste patterns: the ones above, and more
   ([docs/patterns.md](docs/patterns.md)).
3. **Recommend** fixes ranked by money saved, across a whole team.
4. **Apply** the safe ones automatically, with measurement and rollback.

Principles it is designed around:

- **Local-first.** Prompts contain source code and secrets. Raw content will
  never leave the developer's machine unless explicitly enabled; teams will
  share keyed hashes (fingerprints only the customer's key can make), sizes,
  counts and cost, not code or names
  ([ADR 0011](docs/adr/0011-keyed-fingerprints.md),
  [ADR 0019](docs/adr/0019-fingerprint-key-management.md)).
- **Observe before acting.** Recommendations first; changes Trimloop would
  make to requests itself will ship report-only, then in shadow mode
  (worked out and logged, not applied), then to a few, then to all, each
  measured and reversible.
- **Quality is a constraint.** A saving that makes the agent worse is not a
  saving: Trimloop is to track outcome signals (tests, edits, errors) alongside
  cost.
- **Neutral.** One event model for every coding agent, gateway and model
  provider; their differences live in profiles, not in the detectors.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/architecture-dark.svg">
  <img src="docs/images/architecture.svg" width="840" alt="Architecture: on the developer machine, taps catch a coding agent's requests on their way to the model providers; a normaliser, a local store and the detectors turn them into findings (a report and nudges); actuators apply safe fixes. An optional team collector receives hashes, counts and cost, never source code, including from a gateway sidecar beside LiteLLM. Boxes marked M0 are in today's prototype in part.">
</picture>

Boxes marked **M0** exist today, in part: the prototype reads Claude Code's
logs (not live traffic yet), runs 9 of the 19 waste patterns, and gives a
report and nudges. Everything else in the picture is planned.

The core will be one Go program; a thin Python plugin will connect LiteLLM to
it; a web UI (TypeScript) will be embedded in the program. Detail:
[docs/design.md](docs/design.md), [docs/analyzer.md](docs/analyzer.md),
[ROADMAP.md](ROADMAP.md) for the order, and
[docs/adr/README.md](docs/adr/README.md) for every decision with whether it is
built.

## Development

To run the prototype you need only Go. To run its checks you also need
`golangci-lint` and `govulncheck`. Two steps need more: the end-to-end
test's files need `python3`, and validating the plugin needs `claude`
(Claude Code). Without them the checks skip those steps on your machine,
but CI always runs them. `scripts/bootstrap.sh` installs what is missing
(macOS with Homebrew, or Debian/Ubuntu):

```bash
scripts/bootstrap.sh --check                       # see what is missing; changes nothing
scripts/bootstrap.sh --skip-node --skip-python     # enough for the prototype and most checks
scripts/bootstrap.sh                               # everything the planned stack needs
```

With Python and Node.js skipped, the golden-drift and plugin steps run only
if `python3` and `claude` are already installed. The full install adds a C
toolchain and the DuckDB CLI, `buf` and the protobuf plugins, `uv` with
Python 3.12, and Node.js 22 or later;
`--with-collector` also checks Docker. Re-running is safe.

Before committing Go changes:

```bash
scripts/check.sh             # gofmt, go vet, golangci-lint, race tests, golden drift, plugin validation, govulncheck
scripts/check.sh --offline   # the same without govulncheck, which needs the network
```

It runs every check, lists the failures and exits 1 if there are any.
*Golden drift* checks that the end-to-end test's logs and expected figures
(`testdata/golden`) are what their scripts make today. The linters are set
in [.golangci.yml](.golangci.yml).

The code is laid out as:

| Package | What it does |
|---|---|
| `cmd/trimloop` | The command: options, then runs the report or a live command |
| `internal/adapters/source` | Log readers: `claudecode` (Claude Code logs), `jsonl` (shared line reader) |
| `internal/sessions` | Finds and reads every log and builds the sessions (`sessions.Load`) |
| `internal/safefile` | Replaces a file whole or not at all (reports, history, nudge state, settings) |
| `pkg/event`, `pkg/price`, `pkg/normalize` | The event model, the price list, usage normalisation |
| `pkg/detectors` | The findings, attribution, reconciliation and weekly figures |
| `pkg/cmdshape` | Shell commands reduced to their shape, without values |
| `pkg/fixes` | Recommendations: for each finding, fixes ordered by kind, strongest first (the *fix ladder*, ADR 0030) |
| `internal/report` | Results, rendered as summary, text, web page and JSON |
| `internal/history` | Weekly figures kept between runs |
| `internal/live` | Status line, hooks and `setup` for Claude Code |

### Releases

Pushing a version tag builds a draft GitHub release; how to cut one, and
the steps for making the repository public, are in
[docs/packaging.md](docs/packaging.md#cutting-a-release).

## Documents

| Doc | What's in it |
|---|---|
| [docs/measurements.md](docs/measurements.md) | How each figure the prototype reports is worked out today |
| [docs/examples/example-report.html](docs/examples/example-report.html) | A full web-page report from real logs (download and open in a browser) |
| [docs/patterns.md](docs/patterns.md) | Catalogue of waste patterns: signals, detection, fix |
| [docs/state-machines.md](docs/state-machines.md) | What the code remembers between steps, and the files it keeps |
| [docs/log-signals.md](docs/log-signals.md) | What Claude Code logs contain, and the measured figures (§8) |
| [docs/design.md](docs/design.md) | Planned architecture, components, deployment, security |
| [docs/analyzer.md](docs/analyzer.md) | The analysis engine: command line, gateway sidecar, library; harness and provider profiles |
| [docs/value.md](docs/value.md) | Value case: levers ranked by expected saving, validation plan |
| [docs/data-model.md](docs/data-model.md) | Event model and storage |
| [docs/packaging.md](docs/packaging.md) | Release artefacts and pipeline |
| [docs/adr/](docs/adr/README.md) | ADRs (*architecture decision records*): one per design decision, with an index of what is built |
| [ROADMAP.md](ROADMAP.md) | Milestones from this prototype to automatic fixes |

## Licence and security

Licence: [Apache-2.0](LICENSE). You may use, change and share the code
for any purpose, including in commercial products, as long as you keep the
licence and the [NOTICE](NOTICE) file with it. The licence does not cover
the name "Trimloop"
([ADR 0054](docs/adr/0054-apache-2-licence.md)).

Outside contributions are not accepted yet: they will need a contributor
licence agreement.

Please report security problems privately, not in an issue: on the
repository's **Security** tab, choose **Report a vulnerability**
([GitHub's guide](https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/report-privately)).

Trimloop = **trim** + **loop**: it trims wasted tokens in a loop of detect,
recommend, measure, repeat.
