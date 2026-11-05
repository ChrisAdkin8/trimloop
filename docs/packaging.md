# Packaging and distribution

| | |
|---|---|
| Status | Draft |
| Last updated | 2026-11-05 |
| Decision | [ADR 0008](adr/0008-packaging-and-distribution.md) |
| Related | [analyzer.md](analyzer.md) (shapes and repo layout), [design.md §9](design.md#9-deployment-modes) (deployment modes) |

## 1. Principles

1. **One binary, many modes.** Every Go component ships in a single
   `trimloop` binary. Modes are subcommands and configuration, not separate
   products.
2. **Works offline, never calls home.** Everything needed to run — detectors,
   profiles, price catalogue, templates, web UI, and the DuckDB extensions it
   uses — is embedded. Anything that fetches data from the network is opt-in
   ([bundled components](#bundled-components)).
3. **Same artefact everywhere.** The binary on a laptop, in the sidecar image
   and in the collector image is the same build.
4. **Verifiable.** Every artefact is signed and ships with a software bill of
   materials and build provenance. Trimloop sits in the path of private
   traffic; security teams will ask.
5. **Release tooling lives in CI.** Developers need only the SDKs from
   `scripts/bootstrap.sh`; packaging and signing tools run in the release
   pipeline.

## 2. Artefacts

| # | Artefact | Contents | Channel | Audience | Milestone |
|---|---|---|---|---|---|
| 1 | **`trimloop` binary** | All modes (§3); embedded web UI, detector SQL, profiles, price catalogue, fix templates | Homebrew tap, GitHub Releases (archives + checksums), `.deb` / `.rpm` | Developers; servers without containers | M0 |
| 2 | **Container image** `ghcr.io/<org>/trimloop` | The same binary, multi-arch | GitHub Container Registry | Gateway sidecar, team collector | M1 |
| 3 | **Helm chart** `trimloop` | Sidecar mode and collector mode (§6) | Helm repository (OCI, on GHCR) | Platform teams on Kubernetes | M1 (sidecar), M2 (collector) |
| 4 | **Docker Compose example** | LiteLLM + plugin + sidecar | `deploy/compose/` in the repo | Evaluators; teams not on Kubernetes | M1 |
| 5 | **`trimloop-litellm`** | Thin Python plugin (§7) | PyPI | Teams running LiteLLM | M1 |
| 6 | **Go module** | Public `pkg/` packages for embedding the engine | Go module proxy | Envoy/Kong integrations, third parties | Unstable until M3 |
| 7 | **Wire schemas** | `proto/trimloop/v1` | Buf Schema Registry | Authors of other plugins | M1 |
| 8 | **Envoy integration** | `trimloop serve --extproc` — a mode of artefacts 1 and 2 | (as 1 and 2) | Envoy-based gateways | M4 |
| 9 | **`trimloop-kong`** | Separate small binary and image | GitHub Releases, GHCR | Kong AI Gateway | M4 |

Milestones follow [ROADMAP.md](../ROADMAP.md): the gateway sidecar ships in M1,
before the local proxy ([ADR 0009](adr/0009-gateway-sidecar-before-local-proxy.md)).

## 3. The `trimloop` binary

### Modes

| Command | Purpose |
|---|---|
| `trimloop analyse` | Standalone analysis of session transcripts (M0) |
| `trimloop serve` | Sidecar or team collector, depending on configuration |
| `trimloop serve --extproc` | Envoy external-processing server |
| `trimloop proxy` | Local forward/reverse proxy |
| `trimloop report`, `explain` | Render reports; explain a finding |
| `trimloop conformance` | Test a provider or harness profile against fixtures |
| `trimloop update-catalogue` | Opt-in: fetch a signed price catalogue (§3.3) |
| `trimloop version` | Version, build provenance, embedded data versions |

### Why one binary

- `analyse`, `serve` and `proxy` run the same engine; separate binaries would
  only add version skew.
- The collector is `serve` with ClickHouse storage and uploads enabled — a
  configuration, not a product.
- "Install `trimloop`" is one instruction to document, support and sell.
- Cost: every mode carries the DuckDB and ClickHouse drivers. A binary in the
  tens of megabytes is acceptable for a developer tool and a server image.

Exception: Kong runs each Go plugin as its own executable, so the Kong
integration is a separate binary (artefact 9).

### Embedded data and overrides

Embedded with `go:embed`: detector SQL, provider and harness profiles, price
catalogue, fix templates, built web UI.

Lookup order for each data kind, first match wins:

1. Path given by a flag or `trimloop.yaml`.
2. Override directory: `~/.trimloop/` (local) or `/etc/trimloop/` (server).
3. Embedded default.

`trimloop version` prints the version of each embedded data set and any
override in use, so a finding can always be traced to the data that produced
it.

### Price catalogue updates

Prices change more often than code. `trimloop update-catalogue` (off by
default, never automatic) downloads a catalogue signed by the project's
release key, verifies it, and writes it to the override directory. Teams that
never run it keep the catalogue that shipped with their release.

### Bundled components

Third-party code compiled into the binary. None is shipped or installed
separately; users never install DuckDB to use Trimloop.

| Component | How it is included | Used by | Notes |
|---|---|---|---|
| **DuckDB** | Static library via DuckDB's official Go driver (cgo) | `analyse`, `serve` with the DuckDB store, `proxy` | MIT licence. Version pinned in `go.mod`, listed in the SBOM, printed by `trimloop version`. The reason builds are native per platform (§4) |
| **DuckDB extensions** | Only those the detectors need (expected: JSON), statically linked | As DuckDB | Never downloaded at run time — see below |
| **ClickHouse client, Postgres driver** | Pure Go | Collector mode | No cgo |
| **Web UI** | Built from `web/`, embedded | `serve`, local UI | |

The DuckDB command-line tool installed by `scripts/bootstrap.sh` is a developer
convenience for writing detector SQL. It is not part of any artefact.

**No extension downloads.** By default DuckDB installs and loads some
extensions from the internet the first time a query needs them. That would
break the "never calls home" principle. Every connection Trimloop opens
therefore:

- sets `autoinstall_known_extensions = false` and
  `autoload_known_extensions = false`;
- refuses community extensions;
- locks the configuration (`lock_configuration = true`) so a query cannot
  change these settings.

CI checks that every extension the detectors use is reported as built in, that
nothing is written to DuckDB's extension directory, and that `trimloop
analyse` makes no network connections.

**Local store upgrades.** The DuckDB file (`~/.trimloop/trimloop.duckdb`) is
created at first run and changes over time in two ways:

| Change | Handling |
|---|---|
| **Trimloop's own schema** (tables, columns) | Schema version kept in a `meta` table. On open, forward-only migrations run in a transaction, after copying the file to `trimloop.duckdb.bak-<old-version>` |
| **DuckDB's storage format** | A newer DuckDB can write files an older one cannot read. Release notes flag every DuckDB version bump. An older `trimloop` that meets a newer file stops with a clear message pointing to the backup, and never modifies the file |

The local store is derived data: anything that came from transcripts can be
rebuilt with `trimloop analyse --rebuild`. Events received live by a sidecar
using the DuckDB store cannot be rebuilt, which is one reason the team
collector holds the durable copy. CI opens store files written by each of the
last few releases to prove upgrades work.

Collector-mode schemas in ClickHouse and Postgres use versioned SQL
migrations, run by `trimloop serve --migrate` and, in the Helm chart, by a
pre-upgrade job.

## 4. Platforms and builds

| OS | Architectures | Notes |
|---|---|---|
| macOS | arm64, amd64 | Signed and notarised |
| Linux (glibc) | amd64, arm64 | `.tar.gz`, `.deb`, `.rpm`; base of the container image |
| Windows | amd64 | Later; no Windows-specific design work until demand exists |

The DuckDB Go driver needs cgo, so binaries are built **natively on each
platform** in CI (macOS and Linux runners for each architecture) rather than
cross-compiled from one machine. A static musl build is not planned; the
container image uses a glibc base instead.

## 5. Container image

- Base: a distroless glibc image (e.g. `gcr.io/distroless/cc`): no shell, no
  package manager.
- Runs as a non-root user; read-only root filesystem supported.
- Multi-arch manifest: `linux/amd64`, `linux/arm64`.
- Entrypoint `trimloop`, default command `serve`.
- Tags: exact version (`1.4.2`), minor (`1.4`), and digest. No floating
  `latest` in documentation.
- Configuration by mounted `trimloop.yaml` and environment variables.

## 6. Helm chart

One chart, two modes:

| Mode | What it deploys |
|---|---|
| `sidecar` | Values to add a `trimloop serve` container to an existing LiteLLM deployment, listening on loopback; plugin configured to point at it |
| `collector` | `trimloop serve` as the team collector; optional ClickHouse and Postgres sub-charts, or connection settings for existing ones |

```yaml
mode: sidecar            # sidecar | collector
image:
  repository: ghcr.io/<org>/trimloop
  tag: ""                # defaults to the chart's appVersion
serve:
  decide: false
  upload:
    url: ""              # team collector, hashes only
collector:
  clickhouse: { enabled: true }
  postgres:   { enabled: true }
```

The chart version and `appVersion` track the binary version (§9).

## 7. Python plugin: `trimloop-litellm`

- Pure-Python wheel; no compiled extensions.
- Dependencies: only what LiteLLM already brings (its HTTP client), so
  installing the plugin adds nothing new to the gateway.
- Declares the sidecar API it speaks (`/v1`) and checks it against
  `GET /v1/health` at start-up; logs a clear error and stays inert on a
  mismatch, never blocking traffic.
- Supported LiteLLM versions are listed in the package metadata and tested in
  CI against each (see [analyzer.md §14](analyzer.md#14-testing)).
- Built and published from `plugins/litellm/` with `uv build`; published to
  PyPI with trusted publishing from CI (no long-lived tokens).

## 8. Go module, schemas and integrations

- **Go module.** `pkg/` is the public embedding API. It is versioned with the
  binary but marked unstable (`v0`) until M3; after that, breaking changes
  follow semantic import versioning. Everything in `internal/` and
  `adapters/` is private.
- **Schemas.** `proto/trimloop/v1` is published to the Buf Schema Registry so
  others can generate clients in any language. A breaking change creates
  `v2`; `v1` stays served until a documented end date.
- **Envoy.** `trimloop serve --extproc` implements Envoy's external-processing
  gRPC API; shipped in the standard binary and image.
- **Kong.** `trimloop-kong` is built from `cmd/trimloop-kong/` against Kong's
  Go plugin development kit; versioned with the binary.

## 9. Versioning and compatibility

| Artefact | Version |
|---|---|
| Binary, container image, Helm chart `appVersion`, `trimloop-kong` | One shared semantic version |
| Helm chart | Its own chart version, bumped on every release |
| `trimloop-litellm` | Its own version; declares the sidecar API version it needs |
| Sidecar HTTP API, wire schemas | `v1`, `v2`, … — independent of release numbers |
| Embedded data (detectors, profiles, prices) | Versioned individually; printed by `trimloop version` |

A compatibility table in the repository lists which plugin versions work with
which sidecar API versions.

## 10. Release pipeline

Run by CI on a version tag:

1. Build and test on each platform runner (§4).
2. GoReleaser assembles archives, checksums, `.deb`/`.rpm`, the Homebrew tap
   update, and multi-arch container images.
3. macOS binaries are signed with a Developer ID and notarised by Apple.
4. Everything is signed and attested (§11).
5. The web UI is built first and embedded; the Python plugin and Helm chart
   are published by their own jobs in the same workflow.
6. Release notes list changed detectors, profiles and price-catalogue
   versions, because those change findings.

## 11. Supply chain

| Measure | Applies to |
|---|---|
| Keyless signatures (cosign) | Binaries, checksums file, container images, Helm chart |
| Software bill of materials (SPDX or CycloneDX) | Every binary and image, including the bundled DuckDB library and extensions |
| Build provenance attestation (SLSA) | Binaries and images |
| Apple notarisation | macOS binaries |
| PyPI trusted publishing + attestations | `trimloop-litellm` |
| Signed price catalogue | `update-catalogue` downloads |

The README's install section will show how to verify a download and an image
signature.

## 12. Repository layout additions

These extend the layout in [analyzer.md §12](analyzer.md#12-repository-layout):

```text
cmd/trimloop/          the binary
cmd/trimloop-kong/     Kong plugin binary (M4)
internal/collector/    team-collector features: recommendations, policy, multi-user
web/                   TypeScript UI, built then embedded into the binary
deploy/helm/trimloop/  Helm chart
deploy/compose/        Docker Compose example
plugins/litellm/       Python package with its own pyproject.toml
.goreleaser.yaml       release definition
```

`internal/collector/` keeps team-only features in one place, so a later
licensing decision (open-core or otherwise) does not force a restructure.

## 13. Open questions

1. **Licence.** Open-source everything, or open-source the binary's
   developer and sidecar modes with a commercial collector? The layout keeps
   both possible.
2. **Homebrew.** Own tap first; apply to homebrew-core once the project
   meets its popularity rules?
3. **Windows.** Wait for demand, or ship `analyse` early because transcript
   analysis needs no proxy?
4. **Air-gapped installs.** Is a bundle (binary + image + chart + catalogue)
   needed for regulated customers?
