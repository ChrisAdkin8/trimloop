# ADR 0046: Licensing — FSL core, Apache-2.0 connectors, commercial team features

- Status: Accepted — partly built: [LICENSE.md](../../LICENSE.md) puts the repository under FSL-1.1-ALv2; the Apache-2.0 connectors and the commercial team features do not exist yet
- Date: 2026-11-08
- Amends: [ADR 0008](0008-packaging-and-distribution.md) (settles the licensing decision it left open); closes ROADMAP open question 2 and packaging.md open question 1

## Context

Trimloop is meant to become a commercial product. Until now the repository
had no licence, so nobody else could legally use it.

What makes it valuable is the detectors and the measurement method
([ADR 0045](0045-m0-measurement-method.md)). The commercial risk is not
developers using it free; it is a gateway or observability vendor building
the detectors into a competing paid product. At the same time the product
asks for trust: it reads developers' session logs, and "nothing leaves your
machine" ([ADR 0004](0004-local-first-hashes-only.md)) is easier to believe
when the code can be read.

Options weighed:

- **Closed source.** Full control, but the privacy claims cannot be checked
  and adoption is slow.
- **Permissive (MIT, Apache-2.0).** Fastest adoption, but a competitor can
  ship the detectors in a paid product at no cost.
- **Copyleft (AGPL).** Many companies forbid it outright, which hurts
  adoption at work; it does little against a competitor that runs the code
  on users' own machines.
- **Source-available (BSL, Elastic License 2.0, FSL).** Code readable and
  free to use, with competing commercial use excluded. The FSL is short,
  fixed (no per-project parameters to get wrong) and turns each version into
  Apache-2.0 after two years.

## Decision

1. **The repository is licensed FSL-1.1-ALv2** (Functional Source License,
   version 1.1, with Apache-2.0 as the future licence), in
   [LICENSE.md](../../LICENSE.md). Anyone may use, change and redistribute
   it for any purpose except a Competing Use: making it available to others
   in a commercial product or service that substitutes for it or offers
   substantially the same functionality. Internal use at any company,
   education, research, and professional services to licensees are allowed.
   Each version becomes available under Apache-2.0 two years after it is
   made available.
2. **Connectors are Apache-2.0.** Code meant to be built into other people's
   tools — the gateway plugins ([ADR 0005](0005-thin-python-plugin-for-litellm.md),
   [ADR 0006](0006-gateway-plugin-forwards-to-sidecar.md)) and a published
   schema for the JSON report — will carry an Apache-2.0 licence file in its
   own folder when it is built. They exist to bring users to trimloop, so
   they should be free to copy.
3. **Team features are expected to be commercial.** The team collector, the
   team web UI and automatic fixes (M2–M4) stay apart in
   `internal/collector/` and its successors ([ADR 0008](0008-packaging-and-distribution.md)).
   Whether they are closed source or FSL with a paid licence is decided by a
   later ADR when M2 starts.
4. **No outside contributions without an agreement.** Code from anyone else
   is accepted only under a contributor licence agreement that lets the
   licensor relicense it and sell commercial licences. Until one exists,
   outside changes are not merged.
5. **The licence does not cover the name.** "Trimloop" needs a trademark
   search and registration before going public (ROADMAP open question 1).

## Consequences

- Trimloop is source-available, not open source by the Open Source
  Initiative's definition. The README says so plainly; some companies'
  policies allow only approved open-source licences.
- Third parties may still embed `pkg/` (ADR 0008), except in a competing
  product, until that version turns Apache-2.0.
- The two-year clock runs from when each version is made available, so
  releases are tagged and dated, and the tags kept.
- The licence text names the licensor as Chris Adkin. If a company is to
  hold the rights, change the notice in LICENSE.md before the first public
  release.
- Before the first public release, a lawyer should review the licence
  choice, the licensor, the trademark, and how far copyright covers code
  written with an AI assistant; the design records and review history
  document the human design and review behind it.
