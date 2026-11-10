# Development

How to build trimloop, run its checks and find your way around the code.
What feedback and changes are welcome is in
[CONTRIBUTING.md](../CONTRIBUTING.md).

## Tools and checks

To run the prototype you need only Go. To run its checks you also need
`golangci-lint` and `govulncheck`. Two steps need more: the end-to-end
test's files need `python3`, and validating the plugin needs `claude`
(Claude Code). Without them the checks skip those steps on your machine,
but CI always runs them. `scripts/bootstrap.sh` installs what is missing
(macOS with Homebrew, or Debian/Ubuntu):

```bash
scripts/bootstrap.sh --check                       # see what is missing; changes nothing
scripts/bootstrap.sh --skip-node --skip-python     # enough for the prototype and most checks
scripts/bootstrap.sh                               # everything the planned stack needs
```

With Python and Node.js skipped, the golden-drift and plugin steps run only
if `python3` and `claude` are already installed. The full install adds a C
toolchain and the DuckDB CLI, `buf` and the protobuf plugins, `uv` with
Python 3.12, and Node.js 22 or later;
`--with-collector` also checks Docker. Re-running is safe.

Before committing Go changes:

```bash
scripts/check.sh             # gofmt, go vet, golangci-lint, race tests, golden drift, plugin validation, govulncheck
scripts/check.sh --offline   # the same without govulncheck, which needs the network
```

It runs every check, lists the failures and exits 1 if there are any.
*Golden drift* checks that the end-to-end test's logs and expected figures
(`testdata/golden`) are what their scripts make today. The linters are set
in [.golangci.yml](../.golangci.yml).

## Code layout

The code is laid out as:

| Package | What it does |
|---|---|
| `cmd/trimloop` | The command: options, then runs the report or a live command |
| `internal/adapters/source` | Log readers: `claudecode` (Claude Code logs), `jsonl` (shared line reader) |
| `internal/sessions` | Finds and reads every log and builds the sessions (`sessions.Load`) |
| `internal/safefile` | Replaces a file whole or not at all (reports, history, nudge state, settings) |
| `pkg/event`, `pkg/price`, `pkg/normalize` | The event model, the price list, usage normalisation |
| `pkg/detectors` | The findings, attribution, reconciliation and weekly figures |
| `pkg/cmdshape` | Shell commands reduced to their shape, without values |
| `pkg/fixes` | Recommendations: for each finding, fixes ordered by kind, strongest first (the *fix ladder*, ADR 0030) |
| `internal/report` | Results, rendered as summary, text, web page, JSON and the share format |
| `internal/history` | Weekly figures kept between runs |
| `internal/live` | Status line, hooks and `setup` for Claude Code |
| `internal/demo` | The made-up logs `trimloop demo` reports on (written by `scripts/gen-demo.py`) and its fixed clock |

## Releases

Pushing a version tag builds a draft GitHub release; how to cut one, and
the steps for making the repository public, are in
[docs/packaging.md](packaging.md#cutting-a-release).
