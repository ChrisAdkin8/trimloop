# ADR 0012: Gateway sessions across replicas, M1 parity, and idle windows

- Status: Proposed
- Date: 2026-11-05

## Context

The cold review found three problems with the M1 gateway sidecar:

1. **Replicas.** Production LiteLLM usually runs several replicas behind a load
   balancer, each with its own sidecar. One developer's turns land on
   different replicas, so each sidecar sees fragments of a session and prefix
   continuity (analyzer §6), P02 gaps and P05 loops break. Harnesses also send
   sub-agent requests and small auxiliary calls (titles, summaries) that look
   like separate one-turn sessions and skew per-session statistics.
2. **Parity.** M1's "findings match `trimloop analyse`" cannot hold exactly:
   gateway traffic and transcripts carry different capabilities
   (analyzer §10.1) and sessions are rebuilt differently. "Adds no latency" is
   unprovable, and the plugin buffer was capped by event count, not bytes.
3. **Idle window.** Sessions closed after 60 idle minutes, so P02 never saw
   the pauses that defeat a 1-hour cache.

## Decision

**Sessions across replicas.**

- **M1** supports two topologies: a single gateway instance, or several
  replicas with **sticky routing** by the client's API key or user header (a
  load-balancer setting). The Helm chart documents the setting; the sidecar
  warns when it sees a session's prefix continue from a turn it never
  received.
- **M2 onwards**, session-scope detection can run at the **team collector**:
  sidecars upload normalised, keyed turn records (no content), and the
  collector sessionises across all replicas and runs session-scope detectors.
  Sidecars then do normalisation, fingerprinting and turn-scope detectors
  only. This is the default whenever a collector is configured.
- **Sub-agents and auxiliary calls** are labelled, not merged. Each request
  gets a session `kind`: `main`, `subagent` or `auxiliary`. Harness profiles
  give the rules (for example: a system-prompt fingerprint known to belong to
  title generation, a cheaper model with no shared history, or a sub-agent
  system prompt). Sub-agent sessions link to their parent by
  `parent_session_id` where the harness provides it, else by key, time window
  and harness. Per-session statistics count `main` sessions only; findings
  still cover all kinds.

**M1 parity and overhead.**

- Parity is defined **per detector, on capabilities both sources have**:
  on the same fixture sessions, finding counts match exactly and estimated
  savings match within 2 %. Detectors that run `reduced` or `not_applicable`
  on one side are excluded and listed.
- Overhead is a **measured budget**: with decide off, the plugin adds at most
  1 ms at p99 to the request path (callbacks are asynchronous); CPU and memory
  overhead on the gateway host are reported in the M1 benchmark.
- The plugin buffer is capped by **bytes** (default 50 MB) as well as events;
  on overflow it drops the oldest events and counts them, never blocking.

**Idle window.** A session's prefix index and state are kept for **2 hours**
of inactivity by default — longer than the longest cache lifetime (1 hour) —
so P02 sees expiries. Configurable; never shorter than the longest TTL in any
enabled provider profile.

## Consequences

- M1's "done when" becomes measurable (ROADMAP).
- Large deployments without sticky routing get correct sessions only from
  M2, when the collector sessionises.
- The collector does more work in M2 (sessionising and session-scope
  detectors), and gateway uploads become per-turn records rather than
  findings only. All uploads remain keyed hashes (ADR 0011).
- Session records gain `kind` and `parent_session_id`.
- Memory for open sessions grows with the longer idle window; the LRU bound
  still applies.
