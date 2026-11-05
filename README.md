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
- **Harness-neutral.** One event model for every harness and gateway.

## Documents

| Doc | What's in it |
|---|---|
| [docs/design.md](docs/design.md) | Architecture, components, deployment, security |
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
