# ADR 0004: Local-first, hashes-only by default

- Status: Proposed
- Date: 2026-11-05

## Context

Coding-agent traffic contains proprietary source code, internal URLs and,
often, secrets. Security teams will block a tool that ships that content to a
third party — or even to an internal server they did not approve. Yet team
recommendations need data from many machines.

## Decision

- Raw content stays on the developer's machine. Default capture mode is
  `hashes`: segment fingerprints, chunk hashes, sizes, token counts.
- The team collector receives structure, counts, cost and findings — never
  content. File paths are hashed unless a repo opts in.
- Secret redaction runs before anything is written, even locally.
- Full content capture is an explicit, per-repo, local opt-in.

## Consequences

- Most detectors must work on fingerprints alone. The catalogue in
  `patterns.md` is designed around that; P07 (bloated standing context) shows
  section sizes but can only show text locally.
- Recommendation templates that need content (e.g. a proposed CLAUDE.md diff)
  are rendered on the developer's machine, not the collector.
- This is a selling point: "we can prove no code left your network."
