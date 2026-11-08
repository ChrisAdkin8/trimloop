# Trimloop

> Small adjustments, big savings. Trimloop watches the traffic between AI coding
> agents and the models they call, finds the patterns that waste tokens, and
> fixes them — safely.

**Status:** M0 prototype. It analyses the Claude Code session logs on your
machine and gives live nudges inside Claude Code; everything else (gateway
plugin, team collector, automatic fixes) is design. To try it, see
[Running the prototype](#running-the-prototype); for what is built, see
[Current state](#current-state).

## The problem

Teams now spend real money on coding agents (Claude Code, Codex, Cursor, Copilot)
and on LLM gateways (LiteLLM, Portkey, Helicone, OpenRouter). Existing tools
answer *where* the money went. Almost none answer *why it was wasted* or *what
to change*:

| What exists today | What it does | What it misses |
|---|---|---|
| LLM gateways | Log requests, cost dashboards, caching, routing rules | You write the rules; no pattern analysis |
| LLM observability (Langfuse, LangSmith, Braintrust, Arize) | Traces and evals for apps you build | Not built around coding harnesses |
| Coding-agent spend trackers (PointFive TokenShift, agentsview, vendor dashboards) | Spend by developer, team, tool | Reports *where*, not *why* |
| Local proxies (claude-code-cache-fix) | Rewrite one harness's traffic to improve caching | Single harness, single fix |
| Trace analysers (Clew, adk-eval, pi-deadloop) | Flag redundant steps in finished sessions | Diagnose only; per-session; no team view |

Nobody joins the four steps together:

1. **Capture** traffic from *any* harness or gateway.
2. **Detect** waste patterns (cache-prefix churn, repeated file reads, loops,
   oversized tool output, over-provisioned models, unused tool definitions…).
3. **Recommend** fixes ranked by money saved, across a whole team.
4. **Apply** the safe ones automatically, with measurement and rollback.

Trimloop is designed to do all four.

## Principles

- **Local-first.** Prompts contain source code and secrets. Raw content never
  leaves the developer's machine unless explicitly enabled. Teams share keyed
  hashes, sizes, counts and cost — not code or names. The key stays with the
  customer: a hosted Trimloop server cannot read your code or names or test
  guesses against them; on a self-hosted collector the same holds for anyone
  without the key ([ADR 0011](docs/adr/0011-keyed-fingerprints.md),
  [ADR 0019](docs/adr/0019-fingerprint-key-management.md)).
- **Observe before acting.** Recommendations come first; changes Trimloop
  would make to requests itself (planned, levels 2 and 3) ship report-only,
  then shadow, then canary, then on. Each is measured and reversible.
- **Quality is a constraint, not an afterthought.** A saving that makes the
  agent worse is not a saving. Trimloop tracks outcome signals alongside cost.
- **Harness- and provider-neutral.** One event model for every harness,
  gateway and model provider; their differences live in profiles, not in
  detectors.

## Architecture

![Trimloop high-level architecture: coding agents send traffic through Trimloop taps to model providers; a normaliser, local store, detectors and actuators run on the developer machine; an optional team collector receives hashes-only findings, including from a gateway sidecar beside LiteLLM, and ranks recommendations](docs/images/architecture.svg)

Everything in the top box runs on the developer's machine; raw prompts and code
never leave it. The dashed arrow is the fix loop: actuators feed safe
transforms back into the proxy. The gateway sidecar (M1) runs the same engine
beside LiteLLM, so teams get findings without installing anything on developer
machines. The team collector is optional and receives only hashes, counts,
cost and findings. Detail in
[docs/design.md §3](docs/design.md#3-architecture).

## Technology

**Go for the core, plus a thin Python plugin for LiteLLM.**

| Part | Language | Why |
|---|---|---|
| Core: session analyser, normaliser, detectors, local proxy, team collector | Go | One binary to install (glibc-linked, built per OS); strong HTTP, TLS and streaming support; one codebase for every tap to feed |
| Detectors | Go per-turn state machines; SQL only for cross-session aggregates | One implementation serves batch analysis and live traffic ([ADR 0010](docs/adr/0010-go-detectors-sql-aggregates.md)) |
| LiteLLM gateway plugin | Python, kept thin | LiteLLM plugins must be Python. The plugin only captures events and forwards them to a `trimloop serve` sidecar on loopback, which does all parsing, redaction and fingerprinting; it applies transforms the core has decided on but contains no detection logic |
| Envoy and Kong gateways (later) | Go | Envoy's external processing and Kong's Go plugin server let the core plug in directly — no third language |
| Web UI | TypeScript | Embedded in the Go binary |

By default all fingerprinting happens in Go, in the sidecar. Where a sidecar
cannot sit beside the gateway, the plugin fingerprints locally; the rules are
specified once and both implementations are tested against shared test vectors.

Decisions: [ADR 0002](docs/adr/0002-go-for-agent-and-proxy.md) (Go),
[ADR 0005](docs/adr/0005-thin-python-plugin-for-litellm.md) (thin Python
plugin), [ADR 0006](docs/adr/0006-gateway-plugin-forwards-to-sidecar.md)
(plugin forwards to a sidecar). Engine design: [docs/analyzer.md](docs/analyzer.md).

## Running the prototype

You need only Go 1.27 or later. The prototype reads the Claude Code session logs on this machine
(`~/.claude/projects`) and reports where tokens went and what to change:

```bash
go run ./cmd/trimloop/                          # short summary: spend, period, top three actions
go run ./cmd/trimloop/ --format html            # web page in trimloop-report.html
go run ./cmd/trimloop/ --format text            # the full report as text
go run ./cmd/trimloop/ --format json --out r.json   # every figure, for scripts
```

The HTML page is one self-contained file: styles, charts and script are
inline and it loads nothing from the network, so it works offline and the
data stays on the machine. It names sessions by the titles Claude Code
gives them, which summarise what you asked, and shows project and skill
names: check it before sharing, or add `--no-titles`
([ADR 0032](docs/adr/0032-readable-reports.md)).

Each run also keeps week-by-week figures (numbers only) in
`~/.trimloop/history.json`, so you can compare before and after a change
even once Claude Code has deleted the old logs. `--history FILE` moves the
file; `--no-history` neither reads nor writes it
([ADR 0033](docs/adr/0033-reconciliation-history-standing-context.md)).

### Live nudges in Claude Code

Trimloop can also nudge you while you work: a status line showing the
conversation's size, its cost and the cache countdown, a message when a
session passes 200k and 500k tokens, and a one-time pause before an
expensive message after a long break
([ADR 0034](docs/adr/0034-live-nudges.md)). Build the binary, then add its
settings to Claude Code:

```bash
go build -o ~/.local/bin/trimloop ./cmd/trimloop
~/.local/bin/trimloop setup                                   # print the settings to add
~/.local/bin/trimloop setup --write ~/.claude/settings.json   # or add them (a backup is made)
```

### Files it writes

Everything stays on your machine, readable by you only (mode 0600):

| File | What it holds |
|---|---|
| The report you ask for (`--out`, or `trimloop-report.html`) | Figures, project names and session titles |
| `~/.trimloop/history.json` | Weekly figures only: no names, titles or paths |
| `~/.trimloop/live/<session>.json` | Which nudges a session has been shown |
| `~/.trimloop/nudges.jsonl` | Each nudge shown: time, kind, session size |

`trimloop setup --write` also edits the Claude Code settings file you name,
after saving a backup next to it.

## Current state

What exists today, and where:

| Package | What it does |
|---|---|
| `cmd/trimloop` | The command: reads logs, builds sessions, runs the report or a live command |
| `internal/adapters/source` | Log readers: `claudecode` (Claude Code transcripts), `jsonl` (shared line reader) |
| `pkg/event`, `pkg/price`, `pkg/normalize` | The event model, the price catalogue, usage normalisation |
| `pkg/detectors` | Findings: P19, P09, P04, P07, P13 (sub-agents), growth and compaction, attribution, reconciliation, weekly figures |
| `pkg/cmdshape` | Shell commands reduced to shapes, without values |
| `pkg/fixes` | Recommendations, with the fix ladder of ADR 0030 |
| `internal/report` | Results, rendered as summary, text, HTML and JSON |
| `internal/history` | Weekly figures kept between runs |
| `internal/live` | Status line, hooks and `setup` for Claude Code |

Not built yet: the store (DuckDB), other harnesses, the gateway plugin and
sidecar (M1), the team collector (M2) and automatic fixes (M3 onwards). The
[ROADMAP](ROADMAP.md) has the order; [docs/adr/README.md](docs/adr/README.md)
lists every decision with whether it is built.

There is no licence yet: all rights reserved. Please report security
problems to the maintainer privately rather than in an issue.

## Development setup

The prototype needs only Go. The script below installs everything the
planned stack needs:

Install the SDKs and tools needed to build Trimloop (macOS with Homebrew, or
Debian/Ubuntu):

```bash
scripts/bootstrap.sh --check   # see what is missing; changes nothing; exit 1 if any
scripts/bootstrap.sh           # install what is missing (asks first)
```

It installs Go (≥ 1.27) and a C toolchain for the DuckDB driver; `buf`,
`protoc-gen-go`, `protoc-gen-go-grpc`, `golangci-lint` and `govulncheck`;
`uv` with Python 3.12 for the LiteLLM plugin; Node.js (≥ 22) for the web UI;
and the DuckDB CLI. `--with-collector` also checks Docker for the team
collector stack; `--skip-node` and `--skip-python` skip those parts. Re-running
is safe: anything already at the minimum version is left alone.

Before committing Go changes, run the code checks:

```bash
scripts/check.sh             # gofmt, go vet, golangci-lint, race tests, govulncheck
scripts/check.sh --offline   # the same without govulncheck, which needs network
```

It runs every check even when one fails, lists the failures at the end and
exits 1 if there are any. The linters are set in [.golangci.yml](.golangci.yml).

## Documents

| Doc | What's in it |
|---|---|
| [docs/design.md](docs/design.md) | Architecture, components, deployment, security |
| [docs/analyzer.md](docs/analyzer.md) | Analyzer engine: standalone CLI, gateway sidecar, embeddable library; harness and provider profiles |
| [docs/value.md](docs/value.md) | Value case: levers ranked by expected saving, worked example, value model, limits, validation plan |
| [docs/log-signals.md](docs/log-signals.md) | What Claude Code logs contain: unused signals, measured opportunities, and gaps in the design they expose |
| [docs/packaging.md](docs/packaging.md) | Artefacts, channels, versioning, release pipeline, supply-chain signing |
| [docs/patterns.md](docs/patterns.md) | Catalogue of waste patterns: signals, detection, fix |
| [docs/data-model.md](docs/data-model.md) | Normalised event model and storage |
| [docs/adr/](docs/adr/README.md) | Architecture decision records, with an index of what is built |
| [ROADMAP.md](ROADMAP.md) | Milestones from offline analyser to auto-fix |

## Name

Trimloop = **trim** + **loop**. It trims wasted tokens, and it works as a
closed loop: detect a pattern, recommend or apply a fix, measure the saving,
repeat ([design §6.2](docs/design.md#62-recommendation-engine-team-collector)).

The working name was "Trimtab". It was dropped on 2026-11-05 because
`trimtab.ai` is owned by another company and an npm package called `trimtab`
already targets AI-agent behaviour.

Availability checked 2026-11-05: `trimloop.ai` and the npm, PyPI and GitHub
names were unregistered; `trimloop.com` is taken; `.dev` unconfirmed.
Trademarks not yet checked — see [ROADMAP.md](ROADMAP.md#open-questions).
