# Packaging and distribution

| | |
|---|---|
| Status | Draft |
| Last updated | 2026-11-08 |
| Decision | [ADR 0008](adr/0008-packaging-and-distribution.md) |
| Built | The prototype binary has the `analyse`, `setup`, `statusline` and `hook` commands, is pure Go, and builds with one GoReleaser job (archives, `.deb`/`.rpm`, a Homebrew cask; [ADR 0047](adr/0047-first-release-pure-go.md), [ADR 0051](adr/0051-private-releases-and-going-public.md)); CI runs the checks, a release dry run and the plugin check. The other commands, the DuckDB store, the container image, the Helm chart and the per-OS pipeline below are planned |
| Related | [analyzer.md](analyzer.md) (shapes, `trimloop serve` in [§5.2](analyzer.md#52-gateway-sidecar-trimloop-serve), repo layout in [§12](analyzer.md#12-repository-layout)), [design.md §9](design.md#9-deployment-modes) (deployment modes) |

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
| 1 | **`trimloop` binary** | All modes (§3); aggregate SQL, profiles, price catalogue, fix templates; embedded web UI from M1 | Homebrew tap, GitHub Releases (archives + checksums), `.deb` / `.rpm` | Developers; servers without containers | M0: an installable release; the signed pipeline in M0 or M1 ([ADR 0029](adr/0029-m0-build-order-by-user-value.md) item 10) |
| 2 | **Container image** `ghcr.io/<org>/trimloop` | The same binary, multi-arch | GitHub Container Registry | Gateway sidecar, team collector | M1 |
| 3 | **Helm chart** `trimloop` | Sidecar mode and collector mode (§6) | Helm repository (OCI, on GHCR) | Platform teams on Kubernetes | M1 (sidecar), M2 (collector) |
| 4 | **Docker Compose example** | LiteLLM + plugin + sidecar; the sidecar shares LiteLLM's network namespace (`network_mode: "service:litellm"`) so loopback works | `deploy/compose/` in the repo | Evaluators; teams not on Kubernetes | M1 |
| 5 | **`trimloop-litellm`** | Thin Python plugin (§7) | PyPI | Teams running LiteLLM | M1 |
| 6 | **Go module** | Public `pkg/` packages for embedding the engine | Go module proxy | Envoy/Kong integrations, third parties | Unstable until M3 |
| 7 | **Wire schemas** | `proto/trimloop/v1` | Buf Schema Registry | Authors of other plugins | M1 |
| 8 | **Envoy integration** | `trimloop serve --extproc` — a mode of artefacts 1 and 2 | (as 1 and 2) | Envoy-based gateways | M4 |
| 9 | **`trimloop-kong`** | Separate small binary and image | GitHub Releases, GHCR | Kong AI Gateway | M4 |
| 10 | **Claude Code plugin** `trimloop` | A skill that runs trimloop and explains the report ([ADR 0049](adr/0049-claude-code-plugin.md)) | This repository as a Claude Code marketplace (`.claude-plugin/marketplace.json`) | Claude Code users | M0 |

Milestones follow [ROADMAP.md](../ROADMAP.md): the gateway sidecar ships in M1,
before the local proxy ([ADR 0009](adr/0009-gateway-sidecar-before-local-proxy.md)).

## 3. The `trimloop` binary

### Modes

| Command | Purpose |
|---|---|
| `trimloop analyse` | Standalone analysis of session transcripts (M0) |
| `trimloop statusline`, `trimloop hook stop\|prompt` | Live nudges, run by Claude Code (M0, [ADR 0034](adr/0034-live-nudges.md)) |
| `trimloop setup` | Print or add the Claude Code settings for the live nudges |
| `trimloop serve` | Sidecar, connector or team collector, depending on configuration ([analyzer.md §5.2](analyzer.md#52-gateway-sidecar-trimloop-serve)) |
| `trimloop serve --extproc` | Envoy external-processing server |
| `trimloop proxy` | Local forward/reverse proxy |
| `trimloop report`, `explain` | Render reports (also from one or more sidecars with `--from`); explain a finding |
| `trimloop recommendations` | Fetch open recommendations from the collector and render the exact change locally (M2); authenticates with a per-developer collector token |
| `trimloop identity resolve` | Map keyed developer ids back to people, for key holders (ADR 0024) |
| `trimloop policy key init` | Create the customer-held policy-signing key (M3, ADR 0021) |
| `trimloop conformance` | Test a provider or harness profile against fixtures |
| `trimloop update-catalogue` | Opt-in: fetch a signed price catalogue ([below](#price-catalogue-updates)) |
| `trimloop key init`, `import`, `export`, `rotate`, `status` | Manage the fingerprint key ([ADR 0011](adr/0011-keyed-fingerprints.md), [ADR 0019](adr/0019-fingerprint-key-management.md)) |
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

Embedded with `go:embed`: aggregate SQL, provider and harness profiles, price
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
| **DuckDB extensions** | Only those the detectors need (expected: JSON), statically linked. `httpfs` only once store encryption ships (ADR 0015) | As DuckDB | Never downloaded at run time — see below |
| **ClickHouse client, Postgres driver** | Pure Go | Collector mode | No cgo |
| **Web UI** | Built from `web/`, embedded | `serve`, local UI | |

The DuckDB command-line tool installed by `scripts/bootstrap.sh` is a developer
convenience for writing aggregate SQL. It is not part of any artefact.

**No extension downloads.** By default DuckDB installs and loads some
extensions from the internet the first time a query needs them. That would
break the "never calls home" principle. Every connection Trimloop opens
therefore:

- sets `autoinstall_known_extensions = false` and
  `autoload_known_extensions = false`;
- refuses community extensions;
- disables remote file systems (DuckDB's `disabled_filesystems` setting, to
  verify for DuckDB 2.0) — required if `httpfs` is ever linked in for its
  crypto;
- locks the configuration (`lock_configuration = true`) so a query cannot
  change these settings.

**Encryption and `httpfs`.** In DuckDB 2.0, writing an encrypted database
needs the OpenSSL-based crypto that `httpfs` provides; without it DuckDB
refuses unless an explicitly unsafe mode is forced. Tested 2026-11-05: an
encrypted `ATTACH` made the stock DuckDB CLI **download** `httpfs` from the
internet. Store encryption is therefore deferred until full-content capture,
when `httpfs` must be linked in (or DuckDB built with it) so no download
happens ([ADR 0015](adr/0015-defer-local-store-encryption.md)).

CI checks that every extension the detectors use is reported as built in, that
nothing is written to DuckDB's extension directory, and that `trimloop
analyse` makes no network connections.

**Local store upgrades.** The DuckDB file (`~/.trimloop/trimloop.duckdb`) is
created at first run and changes over time in two ways:

| Change | Handling |
|---|---|
| **Trimloop's own schema** (tables, columns) | Schema version kept in a `meta` table. On open, the owning process runs `CHECKPOINT`, copies the file to `trimloop.duckdb.bak-<old-version>`, then runs forward-only migrations in a transaction. Only one process writes the store (ADR 0022) |
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

**Today** trimloop is pure Go, so one job builds every platform
([ADR 0047](adr/0047-first-release-pure-go.md)); the rest of this section
applies once DuckDB arrives.

The DuckDB Go driver needs cgo, so binaries are built **per operating system**
([ADR 0014](adr/0014-release-builds-per-os.md)): a macOS Apple-silicon runner
builds both macOS architectures (`amd64` with Apple's clang `-arch x86_64`),
and Linux runners build `amd64` and `arm64` natively. A static musl build is
not planned; the container image uses a glibc base instead.

**glibc baseline: 2.34** (RHEL 9 and derivatives, Ubuntu 22.04, Debian 12 and
newer). Linux binaries are built inside a container with glibc 2.34 (for
example an AlmaLinux 9 image) on the Linux runners, so they never require
newer glibc symbols than that; `.rpm` and `.deb` packages declare the
dependency on both glibc and libstdc++. The distroless `cc` runtime image
(glibc 2.36, with libstdc++) is above the baseline. The release dry-run checks
binaries with `objdump -T` for `GLIBC_` and `GLIBCXX_` symbols newer than the
baseline (ADR 0014).

## 5. Container image

- Base: a distroless glibc image (e.g. `gcr.io/distroless/cc`): no shell, no
  package manager.
- Runs as a non-root user; read-only root filesystem supported.
- Multi-arch manifest: `linux/amd64`, `linux/arm64`.
- Entrypoint `trimloop`, default command `serve`.
- Tags: exact version (`0.4.2`), minor (`0.4`), and digest. No floating
  `latest` in documentation.
- Configuration by mounted `trimloop.yaml` and environment variables.

## 6. Helm chart

One chart, four modes. What `trimloop serve` does in each is in
[analyzer.md §5.2](analyzer.md#52-gateway-sidecar-trimloop-serve); this
section covers only how the chart deploys it.

| Mode | What it deploys |
|---|---|
| `sidecar` | A `trimloop serve` container for the LiteLLM pod, listening on loopback, with the plugin pointed at it. A separate Helm release cannot change another release's Deployment, so the container is added **through the LiteLLM chart's own extra-containers values** — this chart renders that values block (`helm template --show-only`) for the user to merge. If the LiteLLM chart version in use has no such option, the `litellm-bundle` mode deploys LiteLLM (as a dependency) and the sidecar together |
| `litellm-bundle` | LiteLLM plus the sidecar in one release; the fallback for the case above |
| `connector` | `trimloop serve` in connector mode: OTLP receiver and log connectors, holding the fingerprint key and forwarding keyed records to the collector (M2) |
| `collector` | `trimloop serve` as the team collector; optional ClickHouse and Postgres sub-charts, or connection settings for existing ones |

```yaml
mode: sidecar            # sidecar | collector | connector | litellm-bundle
image:
  repository: ghcr.io/<org>/trimloop
  tag: ""                # defaults to the chart's appVersion
serve:
  decide: false
  upload:
    url: ""              # team collector, hashes only
persistence:
  type: emptyDir         # emptyDir (per pod, default) | pvc (litellm-bundle mode only, per pod) | none (memory)
  size: 10Gi             # never shared between replicas (ADR 0022)
  storageClass: ""
findings:
  port: 7342             # read-only findings API and page (ADR 0016)
  tokenSecret: ""       # required: the findings listener will not start without it
  tls: { secretName: "" }
webhook:
  url: ""                # e.g. a Slack incoming webhook for findings
fingerprintKey:
  existingSecret: ""     # Kubernetes secret holding the organisation's key (ADR 0011);
                         # required in sidecar and connector modes, never set in collector mode
collector:
  clickhouse: { enabled: true }
  postgres:   { enabled: true }
```

The chart's `appVersion` is the binary version; the chart has its own version, bumped on every release (§9).

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

- **Go module.** One module; `pkg/` is the public embedding API. Releases
  are tagged `v0.x.y` until the `pkg/` API is stable (M3), and the binary
  carries the same version, so `v0` signals "unstable API" for both. From
  `v1.0.0` breaking changes follow semantic import versioning (a `/v2` module
  path). Everything under `internal/` — adapters and database-backed stores
  — is private, which Go enforces.
- **Schemas.** `proto/trimloop/v1` is published to the Buf Schema Registry so
  others can generate clients in any language. A breaking change creates
  `v2`; `v1` stays served until a documented end date.
- **Envoy.** `trimloop serve --extproc` implements Envoy's external-processing
  gRPC API; shipped in the standard binary and image.
- **Kong.** `trimloop-kong` is built from `cmd/trimloop-kong/` against Kong's
  Go plugin development kit; versioned with the binary. Before M4, verify that
  Kong's Go plugins can read response bodies without buffering whole
  streamed responses; if they cannot, Kong support is limited to request-side
  data or replaced by a log connector.

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

**Today:** one GoReleaser job (`.goreleaser.yaml`,
`.github/workflows/release.yml`) builds a draft release with checksums,
SBOMs, packages and a Homebrew cask, after the tests pass in a read-only
job. The repository is public (since v0.2.0), so the checksums are signed
and provenance is attached; a private copy would skip both
([ADR 0051](adr/0051-private-releases-and-going-public.md)). macOS
binaries are not yet notarised ([ADR 0047](adr/0047-first-release-pure-go.md)).
The pipeline below is the target once cgo arrives.

Run by CI on a version tag. The web UI is built first, so every job embeds
it. Then:

1. **macOS job** (Apple-silicon runner): build both macOS architectures, sign
   with a Developer ID, notarise, and upload archives to a draft release.
2. **Linux jobs** (amd64 and arm64 runners): build, then produce archives,
   `.deb`/`.rpm` and per-architecture container images, and upload them to
   the same draft release.
3. **Final job:** merge and sign the checksum files, combine the container
   images into a multi-arch manifest, attach SBOMs and provenance (§11),
   update the Homebrew tap, and publish the release.

Each binary job runs open-source GoReleaser with `release.mode: append`
([ADR 0014](adr/0014-release-builds-per-os.md)); GoReleaser Pro is the
fallback if the merge step proves fragile. The Python plugin and the Helm
chart are published by their own jobs in the same workflow.

Release notes list changed detectors, profiles and price-catalogue versions,
because those change findings.

### Cutting a release

Pushing a version tag (`git tag v0.2.0 && git push origin v0.2.0`) runs
[.github/workflows/release.yml](../.github/workflows/release.yml), which
builds a draft GitHub release with [GoReleaser](../.goreleaser.yaml):
archives for macOS and Linux, each for amd64 (Intel and AMD) and arm64
(Apple silicon and other ARM), `.deb` and `.rpm` packages, signed
checksums, a software bill of materials and build provenance. Check the
draft, write a short "What's new" above its list of commits (what a user
can now do, in a few lines; the list is grouped by how each commit's
subject starts: Add, Fix, README or Docs), then publish it. With a `HOMEBREW_TAP_TOKEN` secret the release also
updates the `ChrisAdkin8/homebrew-tap` cask, for
`brew install --cask chrisadkin8/tap/trimloop`. CI runs the checks and a
release dry run ([ci.yml](../.github/workflows/ci.yml)) on every push to
`main` and every pull request. [ADR 0047](adr/0047-first-release-pure-go.md)
has the detail.

In a private repository, releases skip the signature and provenance
(both need it public, or a paid plan). **Going public** was done with
v0.2.0 (2026-11-10), in this order
([ADR 0051](adr/0051-private-releases-and-going-public.md),
[ADR 0052](adr/0052-third-review-corrections.md)):

1. Decide whether to tidy the git history: it goes public too.
2. Make this repository public, and turn on private vulnerability
   reporting (Settings → Code security), which the README's "Licence and
   security" section points people to.
3. Delete the v0.1.0 draft, then tag the current commit with a new
   version and push the tag (re-running the old workflow would rebuild the
   old commit). Done with v0.2.0: v0.1.0 cannot be reused, because Go's
   module proxy and checksum database recorded it at the old commit once
   the repository was public, so its tag stays.
4. Check that the new draft is signed, and publish it.
5. Make the tap public.
6. Add the README badges that need a public repository: the latest
   release, Go Reference, and the Go version read from `go.mod`. Done with
   v0.2.0; Go Report Card has none, as that service is retired.

### Pinned by hand

Dependabot updates the GitHub Actions and Go modules. It cannot see the
versions below, which are pinned to releases at least two weeks old and
checked by hand before each release
([ADR 0052](adr/0052-third-review-corrections.md)):

| What | Version | Where |
|---|---|---|
| GoReleaser | v2.19.0 | `.github/workflows/release.yml`, `ci.yml` |
| cosign | v3.1.4 | `.github/workflows/release.yml` |
| golangci-lint | v2.15.0 | `.github/workflows/ci.yml`, `scripts/bootstrap.sh` |
| govulncheck | v1.9.0 | `.github/workflows/ci.yml`, `scripts/bootstrap.sh` |
| Claude Code (plugin validation) | 2.1.310 | `.github/workflows/ci.yml` |
| Go, uv, Node.js on Linux, with SHA-256s | 1.27.3, 0.13.5, v24.22.1 | `scripts/bootstrap.sh` |
| buf, protoc-gen-go, protoc-gen-go-grpc, DuckDB CLI | v1.75.0, v1.36.12, v1.6.2, v2.0.0 | `scripts/bootstrap.sh` |

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

## 12. Repository layout

The packaging parts of the layout — `cmd/trimloop-kong/`,
`internal/collector/`, `web/`, `deploy/`, `plugins/litellm/` and
`.goreleaser.yaml` — are in the single target layout in
[analyzer.md §12](analyzer.md#12-repository-layout).

## 13. Open questions

1. **Licence.** Settled by [ADR 0054](adr/0054-apache-2-licence.md),
   which replaces [ADR 0046](adr/0046-licensing.md): everything in the
   repository is Apache-2.0. Whether team features are commercial is
   decided when M2 starts.
2. **Homebrew.** Own tap first; apply to homebrew-core once the project
   meets its popularity rules?
3. **Windows.** Wait for demand, or ship `analyse` early because transcript
   analysis needs no proxy?
4. **Air-gapped installs.** Is a bundle (binary + image + chart + catalogue)
   needed for regulated customers?
5. **LiteLLM chart.** Confirm the name and shape of the LiteLLM Helm chart's
   extra-containers values for the versions supported (§6).
