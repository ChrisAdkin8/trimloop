# ADR 0028: Use the signals harnesses record — accounting, injected context, effort, retries, outcomes

- Status: Accepted — partly built (cost records, compaction, injected context, attribution) — amended by [ADR 0033](0033-reconciliation-history-standing-context.md)
- Date: 2026-11-07
- Amends: [analyzer.md §10](../analyzer.md#10-extensibility-harnesses-and-providers) (capabilities), [data-model.md](../data-model.md) (Turn, Session, usage normalisation, price catalogue, labels), [design.md §6.3](../design.md#63-outcome-signals), [patterns.md](../patterns.md) (P07 scope; P10, P12, P14 milestones), [ROADMAP.md](../../ROADMAP.md) M0, [ADR 0027](0027-measured-m0-findings.md) (M0 detector list)

## Context

[log-signals.md](../log-signals.md) surveyed what Claude Code's transcripts
contain beyond what the design used. The design modelled a harness as a
source of requests, responses and usage. Claude Code also records its own
accounting, the context it injects, per-turn settings, retries, compaction
telemetry, timings and productivity — and several detectors planned for M3
or M4 because they needed fingerprints turn out to need none of them.

Three findings drive this decision:

1. **The harness's own cost record disagrees with the transcript.** Summing
   transcript usage per message id matched Claude Code's `cost-state` for
   cache reads and writes in every session, but uncached input disagreed in
   369 of 413 sessions. In 91 % of them the gap is a second model (Haiku)
   making auxiliary calls — title generation and similar — that never
   appear as assistant lines. Without the harness record, Trimloop
   undercounts and cannot be validated.
2. **Detectors were gated on capabilities they do not need.** Compaction
   events carry exact token counts; API errors carry retry attempts and
   delays; every request records its effort level and thinking tokens. P10,
   P12 and P14 can run from these fields alone.
3. **Standing context is not only memory files.** The harness injects
   skill listings, MCP instructions, environment and sandbox text, agent
   listings and reminders, each labelled by kind. P07 could not name them.

## Decision

**Capabilities** (analyzer.md §10.1), owned by the harness profile unless
stated:

| Capability | Meaning |
|---|---|
| `harness.cost_record` | The harness records its own per-session usage and cost, including calls not otherwise logged |
| `harness.injected_context` | Context the harness adds is recorded by kind, with sizes |
| `harness.compaction` | Compaction events with before and after token counts |
| `harness.retries` | Retry attempts, delays and causes are recorded |
| `request.shape_hash` | The harness records an identifier for the system prompt and tool list as sent |
| `request.effort` | Reasoning effort or budget per request |
| `timing.turn_duration` | Wall-clock duration per turn (shape-owned for proxies and gateways) |
| `session.origin` | Who or what started each turn: a person, automation, a notification, a schedule |
| `usage.server_tools` | Server-side tool requests (web search, web fetch) reported in usage (provider-owned) |
| `outcome.lines_changed` | Lines added and removed, recorded by the harness |

The Claude Code profile claims all of these for 2.1.288 onward, confirmed
per field by the conformance kit.

**Auxiliary turns and reconciliation** (data-model.md):

- Where `harness.cost_record` is available, calls the harness billed but
  did not log as turns are added as `auxiliary` turns, with the harness's
  usage and model, so session totals are complete.
- The reader's acceptance test is reconciliation: per session and per token
  class, Trimloop's totals match the harness record within 2 %, auxiliary
  calls included. Reports show **unobserved spend** where they do not.

**Turn and Session fields** (data-model.md): Turn gains `effort`, `origin`,
`requested_model`, `stop_reason`, `shape_hash`, `cache_ttl_requested`,
`history_keep`, `duration_ms`, `server_tool_requests`; Session gains
`entrypoint`, `agent_type`, `spawn_depth`, `harness_cost`, `lines_added`,
`lines_removed`.

**Price catalogue**: per-request prices for server-side tools, so
`server_tool_requests` is costed.

**Outcome signals** (design.md §6.3) gain: lines changed, tool error rate,
tool denials, prompts queued while the agent was busy, edits that collided
with user changes, time spent in retries.

**P07** covers standing *and injected* context: memory files, system
prompt, tool definitions and harness-injected kinds, attributed by kind so
the fix can name the skill, plugin or MCP server.

**P10, P12 and P14 move to M0** in reduced fidelity, as P09 did
(ADR 0027): P10 from compaction records, P12 from the harness's retry
records, P14 from effort and reasoning tokens. Their full-fidelity signals
(fingerprints, turn-complexity classes) stay where they were.

**Labels**: the fingerprint registry gains `skill` and `plugin`; agent
names, titles and skill or plugin names are keyed with those labels or
`alias`, and `privacy.plain_names` accepts `skill` and `plugin`.

## Consequences

- M0's detector list is P01–P07, P09, P10, P12, P14 and P19; the
  prototype's next steps are reconciliation and context composition
  (log-signals.md §5).
- Harnesses without these records simply lack the capabilities; detectors
  that need them report `not_applicable`, as the capability model already
  provides.
- The reader does more than parse turns: it also reads the harness's cost
  and compaction records. The conformance kit checks each claimed field per
  harness version.
- Name-bearing fields the survey found (`agentName`, titles, skill and
  plugin names) are covered by the keyed-name rule before any upload.
