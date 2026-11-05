# ADR 0002: Go for the agent, proxy and CLI

- Status: Proposed
- Date: 2026-11-05

## Context

The local component runs on every developer's machine, sits in the request
path of their coding agent, and must handle HTTPS interception, HTTP/2 and
server-sent event streams with very low added latency. It has to be easy to
install on macOS, Linux and Windows.

Options considered: Go, Rust, Python, TypeScript (Node/Bun).

## Decision

Go.

## Rationale

- Single static binary, trivial cross-compilation, no runtime to install.
- Mature standard library for HTTP/1.1, HTTP/2, TLS and streaming.
- Good enough latency for a pass-through proxy; the p99 < 5 ms budget is a
  network-path problem, not a language one.
- Official DuckDB and ClickHouse clients exist.
- Faster to build and hire for than Rust; far better distribution story than
  Python or Node for an always-on local daemon.

## Consequences

- Detectors that need ML (turn-complexity classifier for P13) may live in a
  separate Python service at the team collector, not in the agent.
- The web UI is a separate TypeScript build, embedded into the binary.
