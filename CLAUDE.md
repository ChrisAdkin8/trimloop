# CLAUDE.md

Trimloop is building M0 (`trimloop analyse`): the design documents are the
spec, and a Go prototype lives in `cmd/`, `internal/` and `pkg/`. Run it with
`go run ./cmd/trimloop/` (the package, not `main.go`; it prints a summary,
and `--format text|html|json|share`, `--out FILE`, `--open`, `--since DATE|7d`
and `--no-titles` change the output; `--since` leaves the history file
alone) and test with `go test ./...`. To try it without real logs, copy a
file from `testdata/sessions` into `<dir>/projects/demo/` and set
`CLAUDE_CONFIG_DIR=<dir>`. The report code is in `internal/report`; all
formats render one `report.Results`. Weekly figures are kept in
`~/.trimloop/history.json`, or one `history-<id>.json` per other
`CLAUDE_CONFIG_DIR` (`internal/history`; pass `--no-history` or
`--history FILE` when testing against real logs). Live nudges (`trimloop
statusline`, `hook`, `setup`) are in `internal/live`; they write under
`~/.trimloop`, so set `HOME` to a scratch folder when trying them by hand.
`setup` writes the path of the running program into Claude Code's settings,
so run it from a built binary (`go build -o ~/.local/bin/trimloop
./cmd/trimloop`), not `go run`. Before a commit, run `scripts/check.sh`
(gofmt, vet, golangci-lint per `.golangci.yml`, race tests, plugin
validation when `claude` is installed, govulncheck; `--offline` skips
govulncheck) and keep it passing.

- Entry points: `README.md` (what the prototype does today),
  `docs/development.md` (the checks and the code layout),
  `docs/measurements.md` (how each reported figure is worked out), the build
  order in `ROADMAP.md`, and `docs/design.md` for the planned product;
  `docs/patterns.md` and `docs/data-model.md` hold the detail.
- Decisions live in `docs/adr/`; `docs/adr/README.md` says which records
  are in force. Add a new ADR rather than silently changing a decision, and
  when you do, update both records' status lines, the index's "Built" and
  "In force" columns, and `ROADMAP.md`. The measurement method in force is
  ADR 0045. A correction that keeps a decision (a bug fix, a test, a
  wording change) goes in the commit message, not a new ADR.
- State machines (what the code remembers between log lines, turns, hook
  calls and runs, and the files it keeps) are listed in
  `docs/state-machines.md`. Read the entry before changing one; update it
  after. Compare reports before and after a refactor on a frozen copy of the
  logs (`CLAUDE_CONFIG_DIR=<copy> trimloop --no-history --format json`).
- `TestGolden` checks the report end to end against `testdata/golden`:
  made-up logs from `scripts/gen-golden.py`, and figures from
  `scripts/recount.py`, a second implementation of `docs/measurements.md`
  in Python. When a method changes, change the recount to match the doc and
  regenerate `expected.json` from it, never from trimloop
  (`testdata/golden/README.md`). On real logs, `python3 -I
  scripts/recount.py <dir> --compare <trimloop.json>` checks a run.
- When a measurement method changes, update `docs/measurements.md` and the
  figures in `docs/log-signals.md` §8; older docs point there rather than
  restating new figures.
- Pattern IDs (P01–P19) and safety levels (L0–L3) are referenced across all
  docs. Keep them consistent when editing.
- Planned stack: Go (agent, proxy, CLI, detectors), SQL aggregates on DuckDB
  (local) and ClickHouse + Postgres (team collector), Python (thin LiteLLM
  plugin), TypeScript (web UI). See ADR 0010 for the detector model.
- Privacy default is keyed hashes only (ADRs 0004, 0011): HMAC with a
  customer-held key; names are hashed too. Any feature that needs raw content
  must say so explicitly and run locally. Exception (ADR 0032): local
  reports show session titles and project names (`--no-titles` hides the
  titles, and the web page's session and connector ids; project names stay);
  stored files (history, nudge state, usage-limit readings) hold numbers
  only. Of Claude Code's account file (`.claude.json`) only the plan fields
  are read (ADR 0048).
- Dev environment: `scripts/bootstrap.sh` installs the SDKs; run it with
  `--check` first. Keep its tool list in step with the planned stack.
- Licence: Apache-2.0 in `LICENSE`, with the copyright line in `NOTICE`
  (ADR 0054, which replaces the FSL of ADR 0046). Do not merge outside
  contributions: they need a contributor agreement.
- Packaging and releases: `docs/packaging.md` (ADR 0008). Releases are
  built by `.goreleaser.yaml` in `.github/workflows/release.yml` on a `v*`
  tag (ADR 0047); `ci.yml` runs `scripts/check.sh` and a snapshot release.
  Release tooling runs in CI only; to dry-run locally, use
  `go run github.com/goreleaser/goreleaser/v2@v2.19.0 release --snapshot
  --clean --skip=sign,sbom`. Pin new tools and actions to releases at least
  two weeks old, actions by commit hash.
- Claude Code plugin (ADR 0049): this repo is also a marketplace
  (`.claude-plugin/marketplace.json`, plugin in `plugins/trimloop/`). On a
  release, set `version` in `plugins/trimloop/.claude-plugin/plugin.json`
  to the new version. If the skill's commands or the JSON fields it names
  change, update `skills/trimloop/SKILL.md`. Run
  `claude plugin validate . --strict` (the marketplace) and `claude plugin
  validate plugins/trimloop --strict` (the plugin) after changing either;
  `scripts/check.sh` runs both.
