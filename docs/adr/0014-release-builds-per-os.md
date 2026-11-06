# ADR 0014: Per-OS release builds with open-source GoReleaser

- Status: Proposed
- Date: 2026-11-05
- Amends: [ADR 0008](0008-packaging-and-distribution.md) (how release builds run)

## Context

The DuckDB driver needs cgo, so one machine cannot simply build every
platform. Merging per-platform builds into one release with GoReleaser's
split/merge or prebuilt builder needs GoReleaser Pro. Cross-compiling cgo for
macOS from Linux needs Apple's SDK on non-Apple hardware, which Apple's
licence does not allow. GitHub is retiring its Intel macOS runners.

## Decision

Release builds run per operating system, each with open-source GoReleaser,
and append to one draft GitHub release:

1. **macOS job** (Apple-silicon runner): builds `darwin/arm64` natively and
   `darwin/amd64` with Apple's clang (`-arch x86_64`); signs with a Developer
   ID and notarises; uploads archives to the draft release.
2. **Linux jobs** (amd64 and arm64 runners): build natively inside a
   glibc 2.34 container (the supported baseline, packaging.md §4); produce
   archives, `.deb` and `.rpm` packages and per-architecture container images;
   upload to the draft release.
3. **Final job:** merges the checksum files, signs them, combines the
   container images into one multi-arch manifest, attaches SBOMs and
   provenance, updates the Homebrew tap from the uploaded archives, and
   publishes the release.

GoReleaser settings for each job: `release.mode: append` with
`release.draft: true` and `release.use_existing_draft: true`, so jobs add to
one draft found by tag; a distinct `checksum.name_template` per job
(`checksums-<os>-<arch>.txt`) so checksum files do not collide. The final job
is hand-written, not GoReleaser: it concatenates and signs the checksum files,
creates the multi-arch manifest with `docker buildx imagetools create`,
generates the Homebrew formula from the uploaded archives, and publishes the
draft. The M0 dry-run also runs `objdump -T` on Linux binaries to confirm no
`GLIBC_`/`GLIBCXX_` symbol is newer than the baseline.

## Consequences

- No GoReleaser Pro licence and no Apple SDK on Linux.
- The release workflow has more jobs and a hand-written final merge step;
  M0 includes a release dry-run to prove it end to end.
- If the merge step proves fragile, GoReleaser Pro's split/merge is the
  fallback.
