# ADR 0024: Developer identity, and keeping the key from the coding agent

- Status: Proposed
- Date: 2026-11-05
- Amends: [ADR 0011](0011-keyed-fingerprints.md), [ADR 0019](0019-fingerprint-key-management.md)

## Context

The third cold review found two gaps in the privacy model:

1. **Identities.** `developer_id` was "pseudonymous; mapping to a person lives
   only in the team collector". In hosted mode that puts the
   developer-to-person mapping on the vendor's server, contradicting "cannot
   read your names". Gateway key aliases and team names had no label and no
   opt-in rule either.
2. **The agent can read the key.** On developer machines the key could live
   in an environment variable or a file. The coding agent runs shell commands
   as the same user and inherits the environment: one `printenv` or `cat`
   puts the organisation's key into a prompt sent to the provider and into
   transcripts. With one key per organisation, one leak exposes everyone.

## Decision

**Identities are keyed.**

- `developer_id = fp("developer", stable identity)`. The identity is the
  user's git email where set, else the OS user name and machine id.
- Gateway key aliases and team names are keyed with `fp("alias", …)`.
- The mapping from codes to people is held on the customer side only: a
  developer's machine knows its own identity; administrators can resolve
  codes with the key using `trimloop identity resolve`. The collector stores
  codes.
- Organisations may opt in to plain text with `privacy.plain_names` entries
  `developer` and `alias`, like other names.

**The key on developer machines.**

- On developer machines the key is held **only in the OS credential store**
  (macOS Keychain; Secret Service via libsecret on Linux desktops), with an
  access rule limited to the `trimloop` binary. Environment variable and file
  options are for servers, containers and CI only, and `trimloop` warns if it
  finds them on an interactive machine.
- Where no credential store exists (headless Linux without Secret Service),
  the key file is allowed with mode 0600 and a warning that the coding agent,
  running as the same user, can read it.
- Keys have a recognisable format (`tlfk1_` followed by base64url), and the
  redactor treats that pattern as a secret, so a key that does reach a prompt
  or transcript is redacted before Trimloop stores anything.
- ADR 0019's compromise procedure applies if a key is found in a transcript.

## Consequences

- Hosted mode stores no developer names or emails unless the organisation
  opts in.
- Administrators need the key to see who a code belongs to, which is the
  intended trade-off.
- A credential-store prompt may appear the first time `trimloop` runs.
- The fingerprint label registry gains `developer` and `alias`.
