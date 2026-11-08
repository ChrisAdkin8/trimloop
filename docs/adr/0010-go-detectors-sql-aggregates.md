# ADR 0010: Detectors are Go state machines; SQL only for cross-session aggregates

- Status: Accepted — partly built (detectors are Go, as one-pass functions over finished sessions; the incremental detector interface and SQL aggregates are not built); P07 built as a Go detector by [ADR 0033](0033-reconciliation-history-standing-context.md)
- Date: 2026-11-05
- Amends: [ADR 0003](0003-duckdb-local-clickhouse-team.md) (rationale for SQL detectors)

## Context

Two detector models had crept into the docs. The README, design.md §6.1,
ADR 0003 and packaging.md described detectors as SQL views over DuckDB, with
Go only for sequence logic. [analyzer.md](../analyzer.md) §2 and §7 describe
every detector as a Go incremental state machine (`OnTurn`, `OnSessionEnd`),
with SQL used only for cross-session `Aggregate`s.

The analyzer has to run the same detectors over history (`trimloop analyse`)
and over live gateway traffic (`trimloop serve`), with tested parity. SQL views
over a store fit batch analysis; they do not fit per-turn streaming without
re-querying the whole session on every turn.

## Decision

- **Turn- and session-scope detectors** (P01–P05, P08–P14) are Go
  incremental state machines implementing `detect.Detector`. One
  implementation serves batch replay and live streaming.
- **Cross-session detectors** (repo, developer and org scope: P06, P07, P15,
  P17, P18) are SQL `Aggregate`s run over the store. P16 combines both: a Go
  sequence miner over tool-call sequences the store holds.
- Aggregate SQL stays inside the subset DuckDB and ClickHouse share, or carries
  a dialect variant per engine, both tested against the same fixtures.

## Consequences

- ADR 0003's storage decision (DuckDB locally, ClickHouse and Postgres for
  teams) stands; only its rationale that "most detectors are SQL views" is
  replaced.
- The binary embeds aggregate SQL, not detector SQL.
- Detector state must be serialisable so that incremental re-runs and sidecar
  restarts can resume a session (see analyzer.md §7).
- Writing a turn-scope detector requires Go; writing a cross-session one
  requires SQL. patterns.md says which kind each pattern is.
