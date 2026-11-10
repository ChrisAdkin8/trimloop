# ADR 0047: First releases built in one job while trimloop is pure Go

- Status: Accepted — built: `.goreleaser.yaml` and `.github/workflows/release.yml`, proved by a local snapshot release (2026-11-08); first published as v0.2.0 (2026-11-10); releases bump the plugin version: [ADR 0049](0049-claude-code-plugin.md); signing, provenance and the tap while private, the order of going public, and hardening: [ADR 0051](0051-private-releases-and-going-public.md)
- Date: 2026-11-08
- Amends: [ADR 0014](0014-release-builds-per-os.md) (applies once trimloop needs cgo), [packaging.md](../packaging.md) §4 and §10 (until then)

## Context

[ADR 0014](0014-release-builds-per-os.md) splits release builds across
macOS and Linux runners because the DuckDB driver needs cgo, and cgo cannot
cross-compile for every platform on one machine. The M0 prototype has no
DuckDB and no other dependency: it is pure Go. ADR 0031 asks for an
installable release first, and the licence is now settled
([ADR 0046](0046-licensing.md)).

## Decision

1. **While trimloop builds with `CGO_ENABLED=0`, one GoReleaser job on one
   Linux runner builds every release**: macOS and Linux, amd64 and arm64,
   as `.tar.gz` archives, plus `.deb` and `.rpm` packages. Windows waits for
   demand (packaging.md §4).
2. **What each release carries:** a `checksums.txt` signed keylessly with
   cosign by the release workflow's identity; a software bill of materials
   for each archive (syft); GitHub build provenance for the archives and
   packages. Builds are reproducible (`-trimpath`, file times from the
   commit). The version is stamped into the binary (`trimloop --version`).
3. **Homebrew:** a cask in the `ChrisAdkin8/homebrew-tap` repository,
   written only when the `HOMEBREW_TAP_TOKEN` secret is set.
4. **macOS binaries are not yet signed and notarised by Apple.** That needs
   a paid Apple Developer account. Until then, the cask clears macOS's
   quarantine flag after install and says so in its caveats; downloads
   from the release page are checked with the signed checksums instead.
5. **Every release starts as a draft**, published by hand after checking.
6. **Pinned tools.** GoReleaser, cosign, syft and the linters are pinned to
   releases at least two weeks old, and GitHub Actions to commit hashes.
   CI (`ci.yml`) runs `scripts/check.sh` and a snapshot release on every
   push to `main` and every pull request.
7. **The day trimloop first needs cgo**, releases move to ADR 0014's
   per-OS jobs, with Apple notarisation and the glibc baseline check.

## Consequences

- An installable release no longer waits for the full signed pipeline.
- Clearing the quarantine flag skips Apple's check of the binary. It is
  a stop-gap: notarisation replaces it, and the README tells macOS users how
  to verify a download.
- Nothing is published until the repository has a GitHub remote, and the
  tap repository and its token exist.
- Container images, the Helm chart and the Python plugin (M1) are not part
  of this release.
