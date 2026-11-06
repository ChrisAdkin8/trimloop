# ADR 0001: Capture through four taps — proxy, OTel, transcripts, gateway connectors

- Status: Proposed — order amended by [ADR 0009](0009-gateway-sidecar-before-local-proxy.md) (gateway sidecar before local proxy); item 4 (connectors) amended by [ADR 0011](0011-keyed-fingerprints.md): connectors run in `serve` connector mode, not inside the collector
- Date: 2026-11-05

## Context

Coding harnesses expose model traffic in different ways. Some write rich local
transcripts, some export OpenTelemetry, some can be proxied, and some (Cursor,
Copilot) send traffic to their own backend where only an admin API is
reachable. A proxy gives the most data but has costs: setup friction, a local
CA, and — as `claude-code-cache-fix` documents — changing a harness's base URL
can disable vendor features.

## Decision

Support four taps feeding one normaliser:

1. Transcript importer (first to ship: zero setup, works on history).
2. Local proxy, forward mode by default, reverse mode as a fallback
   (now shipped after the gateway sidecar; see ADR 0009).
3. OTLP receiver.
4. Gateway connectors at the team collector.

## Consequences

- New users get findings on day one from transcripts, before trusting a proxy.
- The normaliser must reconcile duplicates when two taps see the same turn
  (dedupe on provider request id, else on timing + fingerprint).
- Closed-backend harnesses get spend-level data only; the product says so.
- Each tap needs contract tests against recorded fixtures per harness version.
