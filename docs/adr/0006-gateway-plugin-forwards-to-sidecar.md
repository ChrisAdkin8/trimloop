# ADR 0006: Gateway plugins forward raw events to a co-located sidecar

- Status: Proposed
- Date: 2026-11-05
- Amends: [ADR 0005](0005-thin-python-plugin-for-litellm.md)

## Context

ADR 0005 has the Python LiteLLM plugin redact and fingerprint events itself,
so fingerprinting and redaction exist in both Go and Python, kept in step by
shared test vectors. That is a permanent parity burden, and redaction rules in
particular change often.

The analyzer design ([analyzer.md](../analyzer.md)) runs the same Go engine as
a sidecar (`trimloop serve`) next to the gateway.

## Decision

- **Default (sidecar mode):** the plugin forwards raw events — request, response
  usage, timings, allow-listed headers — to `trimloop serve` on loopback
  (`127.0.0.1`) in the same host or pod. The sidecar does all parsing,
  redaction and fingerprinting. The plugin contains none of it.
- **Fallback (remote mode):** where a co-located sidecar is impossible, the
  plugin fingerprints and redacts locally using the shared spec and test
  vectors from ADR 0005, then sends hashes only.
- Auth headers are never forwarded in either mode.

## Consequences

- Redaction and fingerprinting have one implementation in the default path.
- Raw content crosses only the loopback interface inside infrastructure the
  customer already runs for the gateway; it is never written to disk or sent
  further. This keeps the hashes-only promise of ADR 0004 for everything that
  leaves the gateway host.
- Deployment needs one extra container or process beside the gateway.
  Provided as a Helm chart sidecar and a Docker Compose example.
- Remote mode keeps the Python parity tests from ADR 0005, but only for teams
  that need it.
