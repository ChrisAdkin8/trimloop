# Trimloop

> Small adjustments, big savings. Trimloop watches the traffic between AI coding
> agents and the models they call, finds the patterns that waste tokens, and
> fixes them — safely.

**Status:** design phase. No code yet. Start with [docs/design.md](docs/design.md).

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
  leaves the developer's machine unless explicitly enabled. Teams share hashes,
  sizes, counts and cost — not code.
- **Observe before acting.** Every optimisation ships report-only first, then
  shadow, then canary, then on. Each is measured and reversible.
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
| Core: session analyser, normaliser, detectors, local proxy, team collector | Go | One static binary to install; strong HTTP, TLS and streaming support; one codebase for every tap to feed |
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

## Development setup

Install the SDKs and tools needed to build Trimloop (macOS with Homebrew, or
Debian/Ubuntu):

```bash
scripts/bootstrap.sh --check   # see what is missing; changes nothing; exit 1 if any
scripts/bootstrap.sh           # install what is missing (asks first)
```

It installs Go (≥ 1.25) and a C toolchain for the DuckDB driver; `buf`,
`protoc-gen-go`, `protoc-gen-go-grpc`, `golangci-lint` and `govulncheck`;
`uv` with Python 3.12 for the LiteLLM plugin; Node.js (≥ 22) for the web UI;
and the DuckDB CLI. `--with-collector` also checks Docker for the team
collector stack; `--skip-node` and `--skip-python` skip those parts. Re-running
is safe: anything already at the minimum version is left alone.

## Documents

| Doc | What's in it |
|---|---|
| [docs/design.md](docs/design.md) | Architecture, components, deployment, security |
| [docs/analyzer.md](docs/analyzer.md) | Analyzer engine: standalone CLI, gateway sidecar, embeddable library; harness and provider profiles |
| [docs/value.md](docs/value.md) | Value case: levers ranked by expected saving, worked example, value model, limits, validation plan |
| [docs/packaging.md](docs/packaging.md) | Artefacts, channels, versioning, release pipeline, supply-chain signing |
| [docs/patterns.md](docs/patterns.md) | Catalogue of waste patterns: signals, detection, fix |
| [docs/data-model.md](docs/data-model.md) | Normalised event model and storage |
| [docs/adr/](docs/adr/) | Architecture decision records |
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
