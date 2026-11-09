# ADR 0051: Releases while private, the order of going public, and release hardening

- Status: Accepted — built: workflows, GoReleaser settings, Dependabot and bootstrap pins; decision 2 (the order of going public) replaced by [ADR 0052](0052-third-review-corrections.md)
- Date: 2026-11-09
- Amends: [ADR 0047](0047-first-release-pure-go.md) (signing, provenance and the tap while the repository is private; reproducible archives; hardening)

## Context

The first release, v0.1.0, was built while both repositories are private.
Keyless signing would publish the private repository's name in Sigstore's
public log, and GitHub's build provenance for private repositories needs a
paid plan, so the release workflow skips both while private. The draft
release therefore has no signature, yet the cask's caveat, the release
notes and the README spoke of signed checksums. The tap was also updated
when the draft was built, so its download links point at a release not
yet published. A review also found the release job running the tests with
write permissions, no updates for pinned versions, no CI check of the
plugin, archives whose bundled files took the checkout's time, and a
bootstrap script that installed the newest Go, uv and Node.js on Linux.

## Decision

1. **While private, releases are drafts without signature or provenance**,
   and none is published. Texts that ship with a release (the cask's
   caveat, the release notes) do not claim a signature; the README says
   which releases have one.
2. **Going public, in this order**: make the trimloop repository public;
   re-run the release workflow for the version tag (it reuses the draft,
   replaces its files, and now signs them and adds provenance); check the
   draft and publish it; then make the tap public. Until then the tap stays
   private, so no public cask points at an unpublished release.
3. **The tests run in their own job with read-only permissions**; the
   release job, which can write and sign, runs after them.
4. **The tap token moves into a protected `release` environment, limited to
   `v*` tags, when the repository goes public.** Environments in private
   repositories need a paid plan, so it stays a repository secret until
   then; only the release workflow uses it.
5. **Pinned versions are kept up to date** by Dependabot (GitHub Actions and
   Go modules), taking only releases at least two weeks old; on Linux,
   `scripts/bootstrap.sh` pins Go, uv and Node.js with their checksums
   recorded in the script.
6. **CI and `scripts/check.sh` validate the plugin** (`claude plugin
   validate . --strict`, with a pinned Claude Code in CI).
7. **Archives and packages are reproducible**: files bundled with the
   program take the commit's time, so the same commit builds the same files
   (checked: two builds gave identical checksums).

## Consequences

- v0.1.0 must not be published as it stands; it is rebuilt after going
  public.
- Going public takes a few manual steps, listed above and in docs/packaging.md (§10, "Cutting a release").
- A Dependabot pull request needs a review like any other.
