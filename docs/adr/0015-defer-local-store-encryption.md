# ADR 0015: Defer local store encryption until full-content capture

- Status: Proposed
- Date: 2026-11-05

## Context

The design promised an encrypted local DuckDB store from M0. Testing DuckDB
2.0 showed that writing an encrypted database needs the OpenSSL crypto in the
`httpfs` extension; without it DuckDB refuses unless an unsafe mode is forced.
Trimloop must not download extensions (packaging.md), so `httpfs` would have
to be linked into the binary. Whether DuckDB's official Go driver ships
static libraries that include `httpfs` is unconfirmed; if not, M0 needs a
custom DuckDB build on four targets.

The benefit in M0 is small. In the default `hashes` capture mode the store
holds keyed hashes, counts, sizes and costs — no code, prompts or names. The
harness's own transcripts, which do contain code, sit unencrypted beside it in
the user's home directory.

## Decision

- M0 ships an **unencrypted** local store, protected by file permissions
  (`~/.trimloop/` mode 0700, database file 0600).
- Store encryption becomes **required when `capture.content = full`** is
  enabled for a repo, because only then does the store hold raw content. Full
  capture is not offered until encryption works.
- Before full capture is built: confirm whether the Go driver can include
  `httpfs`, or build DuckDB with it, and define where the encryption key
  lives (OS keychain, falling back to a key file with mode 0600 on machines
  without one; environment variable or Kubernetes secret on servers).
- The sidecar store holds keyed hashes only (ADR 0006 forbids raw content on
  the gateway host), so it is not encrypted by Trimloop; the volume can be
  encrypted by the platform.

## Consequences

- M0 needs no custom DuckDB build and links no `httpfs`.
- The "no extension downloads" rule is unchanged: extensions stay disabled
  and the configuration is locked.
- The privacy story for M0 rests on keyed hashing (ADR 0011) and file
  permissions, not encryption at rest.
