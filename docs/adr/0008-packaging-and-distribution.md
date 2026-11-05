# ADR 0008: One binary, one image, one Python package

- Status: Proposed — build approach amended by [ADR 0014](0014-release-builds-per-os.md) (per-OS builds with open-source GoReleaser)
- Date: 2026-11-05

## Context

Trimloop's Go components — standalone analyzer, gateway sidecar, local proxy,
Envoy external processor, team collector — all run the same engine
([analyzer.md](../analyzer.md)). The LiteLLM plugin is Python (ADR 0005), and
the Kong integration must be its own executable because Kong runs each Go
plugin as a separate process. The product sits in the path of private
traffic, so security reviewers will expect signed, verifiable artefacts. The
DuckDB driver needs cgo, which rules out simple cross-compilation.

## Decision

- Ship **one `trimloop` binary** containing every Go mode as a subcommand,
  with detectors, profiles, prices, templates and web UI embedded. The
  collector and Envoy modes are configuration of this binary.
- Ship **one container image** of that same binary (distroless glibc base,
  multi-arch, non-root), and **one Helm chart** with `sidecar`, `collector`,
  `connector` and `litellm-bundle` modes, plus a Docker Compose example.
- Ship the LiteLLM plugin as the **`trimloop-litellm`** pure-Python package
  on PyPI, versioned independently against the sidecar API version. It,
  the container image and the Helm sidecar mode ship in M1 (ADR 0009).
- Ship **`trimloop-kong`** as the only separate binary (M4).
- Publish `pkg/` as a Go module (unstable until M3) and `proto/` to the Buf
  Schema Registry.
- Build natively per platform in CI with GoReleaser; sign everything, attach
  SBOMs and provenance, notarise macOS binaries.
- Embedded data is overridable from a config directory; network fetching of
  the price catalogue is opt-in and signed.
- DuckDB and the extensions the detectors need are compiled in. Extension
  auto-install and auto-load are disabled and the connection configuration
  is locked, so DuckDB never downloads anything.
- The local DuckDB store carries a schema version; forward-only migrations
  run on open after a backup copy, and an older binary refuses to touch a
  newer file.

Detail: [packaging.md](../packaging.md).

## Consequences

- One version number covers the binary, image and chart; users never match
  component versions by hand. The plugin and wire APIs carry their own
  versions with a compatibility table.
- Every mode carries every driver, making the binary larger than a
  single-purpose build; accepted.
- CI needs native macOS and Linux runners for each architecture.
- Team-only features live in `internal/collector/`, so a later licensing
  decision does not force a restructure.
- Release tooling (GoReleaser, cosign, Helm) is needed only in CI, so
  `scripts/bootstrap.sh` does not install it.
