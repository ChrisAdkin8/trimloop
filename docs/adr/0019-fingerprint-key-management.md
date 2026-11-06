# ADR 0019: Fingerprint key management

- Status: Proposed — key storage on developer machines by [ADR 0024](0024-developer-identity-and-key-handling.md)
- Date: 2026-11-05
- Amends: [ADR 0011](0011-keyed-fingerprints.md)

## Context

ADR 0011 keyed every fingerprint with a customer-held key. The second cold
review noted what it left out:

- One organisation key on every laptop means every key holder who can read
  the collector can test guesses across the whole organisation. In
  self-hosted mode the collector's administrator usually created the key, so
  "the servers cannot test guesses" holds fully only for hosted mode.
- A solo user's per-machine key (M0) has no path to an organisation key, and
  history older than the harness's transcript retention cannot be re-keyed,
  breaking before/after baselines.
- No import command, rotation schedule or compromise procedure.

## Decision

- **Commands.** `trimloop key init`, `key import` (from a file, environment
  variable or secret manager reference), `key export` (to a file, for an
  administrator distributing the key), `key rotate`, and `key status` (shows
  `fp_key_id` and where the key is held — never the key).
- **Solo to team.** When a developer joins a team, `key import` installs the
  organisation key alongside the old one. The local store keeps both key ids;
  `trimloop analyse --rebuild` re-fingerprints what the transcripts still
  hold under the new key, and older data stays under the old key id, reported
  separately rather than merged.
- **Distribution.** Administrators distribute the key through an existing
  secret manager (1Password, Vault, cloud secret stores, Kubernetes secrets),
  never through chat or the collector.
- **Rotation.** No routine rotation: rotation breaks comparability and gains
  little, since the key protects only against outsiders. Rotate on suspected
  compromise or when someone with key access leaves and the organisation's
  policy requires it.
- **Compromise procedure.** Rotate immediately; mark the old `fp_key_id` as
  compromised at the collector; restrict collector access to the old data
  until reviewed.
- **Honest promise.** The README and ADR 0011 state the promise by mode:
  - hosted: Trimloop's servers cannot read your code or names and cannot test
    guesses;
  - self-hosted: the same holds for anyone without the key; key holders with
    collector access can test guesses, so collector access should be limited
    to people who may see the organisation's code anyway.

## Consequences

- Team onboarding has one more step (`key import`).
- Mixed-key history is reported per key id; trends across a key change are
  shown as a break, not a drop.
