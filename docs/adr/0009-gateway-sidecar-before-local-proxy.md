# ADR 0009: Ship the gateway sidecar before the local proxy

- Status: Proposed
- Date: 2026-11-05
- Amends: [ADR 0001](0001-capture-proxy-otel-transcripts.md) (order of taps)

## Context

ADR 0001 lists four taps and ships the transcript importer first, followed
by the local proxy. Since then, the design gained a gateway path: a thin
LiteLLM plugin forwarding to a `trimloop serve` sidecar (ADRs 0005, 0006;
[analyzer.md](../analyzer.md)).

The two in-path options compare as follows:

| | Gateway sidecar + plugin | Local proxy |
|---|---|---|
| Installs | Once per team, on infrastructure the company already runs | On every developer machine, with a local CA |
| Request bodies and in-path transforms | Yes | Yes |
| Security review | One server-side component | Software and a certificate on every laptop |
| Reach | Teams with a gateway — larger, budget-holding teams | Anyone, including individuals |
| Local context (repo, git outcomes) | No | Yes |

## Decision

- **M0:** transcript analyser (unchanged).
- **M1:** gateway sidecar and LiteLLM plugin, observe-only.
- **M3:** in-path transforms ship through the plugin's decide path first.
- **M4:** local proxy, for teams without a gateway, including its transforms.

## Consequences

- The first team-wide product needs no software on developer machines.
- Teams without a gateway rely on transcripts (M0) until M4.
- Gateway traffic lacks repo and git context, so findings from the sidecar
  carry no `context.repo` or `outcome.git` capabilities until developer-side
  data is joined in; P05 runs at reduced fidelity and P07 depends on
  `request.full_body` ([analyzer.md §10.4](../analyzer.md#104-capability-gated-detectors)).
- The packaging artefacts for the sidecar — image, Helm sidecar mode, Compose
  example, PyPI plugin, schemas — move to M1.
