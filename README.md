# Trimloop

Trimloop reads the session logs Claude Code keeps on your machine, works out
where your tokens went, and tells you which changes to how you work would save
the most money.

**Status: M0 prototype.** Today it analyses Claude Code logs and gives live
nudges inside Claude Code. It recommends; it does not change anything for
you. The wider product (gateway plugin, team collector, automatic fixes) is
design only: see [The plan](#the-plan).

## What you get

A run on one developer's 44 days of logs:

```text
Read 623 logs: 622 sessions, 18,109 turns.
Spent $2,254 at list prices from 25 Sep to 8 Nov 2026 (44 days), about $359 a week.
Claude Code's own records show $202 (13%) more than its logs: requests the logs do not record.

What to do first (these savings overlap, so do not add them):
  1. Compact long sessions earlier: likely about $555 ($533–$666), up to $851 (38%)
     Compact when a session passes about 200,000 tokens, or between tasks.
  2. Start fresh after a long break: likely about $294, up to $306 (14%)
     After a break of over an hour, start a new session with a short note instead of carrying on.
  3. Run sub-agents on a cheaper model: up to $299 (13%)
     Run sub-agents that search, read and summarise on Sonnet 5.5.
```

The full report (text or a web page) explains each finding, shows the
sessions behind it, and gives the fix step by step.

**How far to trust the figures.** They are estimates:

- Costs are at Anthropic's list prices, not your bill.
- "Up to" is the most a change could save; "likely" replays your logs as if
  you had made it.
- Savings overlap — the same tokens can be saved by several changes — so
  they are never added up.
- Claude Code makes some requests its logs do not record; the report says how
  much (13 % above).
- Quality is not measured: a change marked "untested for quality" (a cheaper
  model, lower effort) needs checking after you make it.

How each figure is worked out is in [docs/measurements.md](docs/measurements.md).

## What it looks for

| Finding | What it means |
|---|---|
| Long sessions (P19) | Every turn re-sends the whole conversation, so big sessions get expensive. When to compact or start fresh. |
| Long breaks (P09) | After an hour away the saved copy (the cache) expires and the whole session is paid for again. |
| Short waits (P02) | Sessions whose cache lasts only 5 minutes (sub-agents, SDK runs) losing it while waiting on a slow command. |
| Tool output (P04) | Large command output and file reads that stay in the conversation and are re-sent on every turn. |
| What every request carries (P07) | The system prompt, tool definitions, skill lists and memory files sent with each request. |
| Model choice (P13) | Sub-agents and sessions on a more expensive model than the work needs, or on an older, dearer version. |
| Effort level (P14) | Turns at the highest effort levels that think far more than usual. |

It also checks its totals against Claude Code's own cost records, shows spend
by project, skill and MCP server, and keeps week-by-week figures so you can
see whether a change worked. The pattern IDs (P19 and so on) refer to
[docs/patterns.md](docs/patterns.md).

## Running it

You need Go 1.27.3 or later. Clone this repository, then from its folder:

```bash
go run ./cmd/trimloop/                          # short summary, as above
go run ./cmd/trimloop/ --format html            # web page in trimloop-report.html
go run ./cmd/trimloop/ --format text            # the full report as text
go run ./cmd/trimloop/ --format json --out ~/trimloop.json   # every figure, for scripts
```

It reads `~/.claude/projects` and sends nothing anywhere.

**Reports name your sessions and projects.** They use the titles Claude Code
gives sessions, which summarise what you asked. Check a report before sharing
it; `--no-titles` removes the titles (project, skill and server names stay).
`.gitignore` keeps `trimloop-report.html` out of git, and `--out` can put a
report anywhere. The web page is one file that loads nothing from the
network.

**No Claude Code logs on this machine?** Try the sample session.
`CLAUDE_CONFIG_DIR` points trimloop at another folder laid out like
`~/.claude`:

```bash
mkdir -p /tmp/trimloop-demo/projects/demo
cp testdata/sessions/tools.jsonl /tmp/trimloop-demo/projects/demo/
CLAUDE_CONFIG_DIR=/tmp/trimloop-demo go run ./cmd/trimloop/ --no-history --format text
```

**Week by week.** Each run keeps weekly figures (numbers only) in
`~/.trimloop/history.json` — one `history-<id>.json` per other
`CLAUDE_CONFIG_DIR` folder — so you can compare before and after a change even
once Claude Code has deleted old logs. `--history FILE` moves the file;
`--no-history` neither reads nor writes it.

### Live nudges in Claude Code

Trimloop can also nudge you while you work:

- a status line with the session's size, its cost and how long its cache has
  left;
- a message when a session passes 200,000 and 500,000 tokens;
- after a long break in a session over 500,000 tokens, a pause before your
  first message, once, showing what sending it will cost.

Build the program, then add its settings to Claude Code:

```bash
go build -o ~/.local/bin/trimloop ./cmd/trimloop
~/.local/bin/trimloop setup                                   # print the settings to add
~/.local/bin/trimloop setup --write ~/.claude/settings.json   # or add them (a backup is made)
```

`setup --write` saves a backup first, follows a link to the real file, keeps
its permissions, updates its own entries if the program moved, and leaves
another status line alone unless you add `--replace-statusline`.
[ADR 0034](docs/adr/0034-live-nudges.md) has the detail.

### Files it writes

Everything stays on your machine, readable by you only (mode 0600):

| File | What it holds |
|---|---|
| The report you ask for (`--out`, or `trimloop-report.html`) | Figures, project names and session titles |
| `~/.trimloop/history.json` (or `history-<id>.json`) | Weekly figures only: no names, titles or paths |
| `~/.trimloop/live/<session>.json` | Which nudges a session has been shown |
| `~/.trimloop/nudges.jsonl` | Each nudge shown: time, kind, session size |

## The plan

Everything in this section is design, not built yet.

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
  make to requests itself will ship report-only, then in shadow, then to a
  few, then to all, each measured and reversible.
- **Quality is a constraint.** A saving that makes the agent worse is not a
  saving: Trimloop is to track outcome signals (tests, edits, errors) alongside
  cost.
- **Neutral.** One event model for every coding agent, gateway and model
  provider; their differences live in profiles, not in the detectors.

![Planned architecture: coding agents send traffic through Trimloop to model providers; analysis runs on the developer machine; an optional team collector receives hashes-only findings, including from a gateway sidecar beside LiteLLM](docs/images/architecture.svg)

The core will be one Go program; a thin Python plugin will connect LiteLLM to
it; a web UI (TypeScript) will be embedded in the program. Detail:
[docs/design.md](docs/design.md), [docs/analyzer.md](docs/analyzer.md),
[ROADMAP.md](ROADMAP.md) for the order, and
[docs/adr/README.md](docs/adr/README.md) for every decision with whether it is
built.

## Development

To run the prototype you need only Go. To run its checks you also need
`golangci-lint` and `govulncheck`. `scripts/bootstrap.sh` installs what is
missing (macOS with Homebrew, or Debian/Ubuntu):

```bash
scripts/bootstrap.sh --check                       # see what is missing; changes nothing
scripts/bootstrap.sh --skip-node --skip-python     # enough for the prototype and its checks
scripts/bootstrap.sh                               # everything the planned stack needs
```

The full install adds a C toolchain and the DuckDB CLI, `buf` and the
protobuf plugins, `uv` with Python 3.12, and Node.js 22 or later;
`--with-collector` also checks Docker. Re-running is safe.

Before committing Go changes:

```bash
scripts/check.sh             # gofmt, go vet, golangci-lint, race tests, govulncheck
scripts/check.sh --offline   # the same without govulncheck, which needs the network
```

It runs every check, lists the failures and exits 1 if there are any. The
linters are set in [.golangci.yml](.golangci.yml). The code is laid out as:

| Package | What it does |
|---|---|
| `cmd/trimloop` | The command: reads logs, builds sessions, runs the report or a live command |
| `internal/adapters/source` | Log readers: `claudecode` (Claude Code logs), `jsonl` (shared line reader) |
| `pkg/event`, `pkg/price`, `pkg/normalize` | The event model, the price list, usage normalisation |
| `pkg/detectors` | The findings, attribution, reconciliation and weekly figures |
| `pkg/cmdshape` | Shell commands reduced to their shape, without values |
| `pkg/fixes` | Recommendations, with the fix ladder of ADR 0030 |
| `internal/report` | Results, rendered as summary, text, web page and JSON |
| `internal/history` | Weekly figures kept between runs |
| `internal/live` | Status line, hooks and `setup` for Claude Code |

## Documents

| Doc | What's in it |
|---|---|
| [docs/measurements.md](docs/measurements.md) | How each figure the prototype reports is worked out today |
| [docs/patterns.md](docs/patterns.md) | Catalogue of waste patterns: signals, detection, fix |
| [docs/log-signals.md](docs/log-signals.md) | What Claude Code logs contain, and the measured figures (§8) |
| [docs/design.md](docs/design.md) | Planned architecture, components, deployment, security |
| [docs/analyzer.md](docs/analyzer.md) | The analysis engine: command line, gateway sidecar, library; harness and provider profiles |
| [docs/value.md](docs/value.md) | Value case: levers ranked by expected saving, validation plan |
| [docs/data-model.md](docs/data-model.md) | Event model and storage |
| [docs/packaging.md](docs/packaging.md) | Release artefacts and pipeline |
| [docs/adr/](docs/adr/README.md) | Decision records, with an index of what is built |
| [ROADMAP.md](ROADMAP.md) | Milestones from this prototype to automatic fixes |

## Licence and security

There is no licence yet: all rights reserved. Please report security problems
to the maintainer privately rather than in an issue.

Trimloop = **trim** + **loop**: it trims wasted tokens in a loop of detect,
recommend, measure, repeat.
