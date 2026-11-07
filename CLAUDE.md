# CLAUDE.md

Trimloop is building M0 (`trimloop analyse`): the design documents are the
spec, and a Go prototype lives in `cmd/`, `internal/` and `pkg/`. Run it with
`go run ./cmd/trimloop/` (the package, not `main.go`) and test with
`go test ./...`.

- Read `docs/design.md` first; `docs/patterns.md` and `docs/data-model.md` hold
  the detail. Decisions live in `docs/adr/` — add a new ADR rather than
  silently changing a decision.
- Pattern IDs (P01–P19) and safety levels (L0–L3) are referenced across all
  docs. Keep them consistent when editing.
- Planned stack: Go (agent, proxy, CLI, detectors), SQL aggregates on DuckDB
  (local) and ClickHouse + Postgres (team collector), Python (thin LiteLLM
  plugin), TypeScript (web UI). See ADR 0010 for the detector model.
- Privacy default is keyed hashes only (ADRs 0004, 0011): HMAC with a
  customer-held key; names are hashed too. Any feature that needs raw content
  must say so explicitly and run locally.
- Dev environment: `scripts/bootstrap.sh` installs the SDKs; run it with
  `--check` first. Keep its tool list in step with the planned stack.
- Packaging and releases: `docs/packaging.md` (ADR 0008). Release tooling
  runs in CI only.
