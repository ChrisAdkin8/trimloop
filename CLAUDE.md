# CLAUDE.md

Trimloop is in the design phase: the repo holds design documents only.

- Read `docs/design.md` first; `docs/patterns.md` and `docs/data-model.md` hold
  the detail. Decisions live in `docs/adr/` — add a new ADR rather than
  silently changing a decision.
- Pattern IDs (P01–P18) and safety levels (L0–L3) are referenced across all
  docs. Keep them consistent when editing.
- Planned stack: Go (agent, proxy, CLI), DuckDB (local), ClickHouse + Postgres
  (team collector), TypeScript (web UI).
- Privacy default is hashes-only (ADR 0004). Any feature that needs raw
  content must say so explicitly and run locally.
- Dev environment: `scripts/bootstrap.sh` installs the SDKs; run it with
  `--check` first. Keep its tool list in step with the planned stack.
- Packaging and releases: `docs/packaging.md` (ADR 0008). Release tooling
  runs in CI only.
