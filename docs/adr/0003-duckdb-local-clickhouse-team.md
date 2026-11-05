# ADR 0003: DuckDB locally, ClickHouse for teams

- Status: Proposed
- Date: 2026-11-05

## Context

Detectors are mostly windowed aggregations over turns and segments. Locally we
need an embedded store with no server. For teams we need high-volume event
analytics that organisations can self-host.

## Decision

- Local: DuckDB, one file under `~/.trimloop/`.
- Team: ClickHouse for events and findings; Postgres for recommendations,
  policies and users.

## Rationale

- Both speak SQL, so most detectors are written once as SQL views and run in
  both places with small dialect shims.
- DuckDB is embedded and columnar: fast enough for months of one developer's
  history.
- ClickHouse is widely self-hosted and built for this shape of data.

## Consequences

- Detector SQL must stay inside the common subset, or carry two versions with
  shared fixture tests.
- Sequence detectors (loops, P05) are written in Go and run in both places.
