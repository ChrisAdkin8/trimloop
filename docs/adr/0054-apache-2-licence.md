# ADR 0054: The repository is open source under Apache-2.0

- Status: Accepted — built: [LICENSE](../../LICENSE) holds the Apache-2.0 text and [NOTICE](../../NOTICE) the copyright line
- Date: 2026-11-10
- Supersedes: [ADR 0046](0046-licensing.md) (kept as history; its decisions 4 and 5 carry over)
- Amends: [ADR 0049](0049-claude-code-plugin.md) (decision 5: the plugin's licence is now Apache-2.0)

## Context

[ADR 0046](0046-licensing.md) put the repository under FSL-1.1-ALv2, a
source-available licence: anyone could read and use the code, but not
offer it in a competing commercial product. It named the risk it guarded
against: a gateway or observability vendor building the detectors into a
paid product.

The maintainer has chosen to make trimloop open source instead. An
open-source licence is easier for companies to approve (many allow only
licences the Open Source Initiative approves), and the README can then say
"open source" plainly.

Among open-source licences, Apache-2.0 fits best:

- It is permissive (few conditions), so it is widely approved for use at
  work.
- It grants a patent licence from each contributor, which MIT does not.
- It is the licence ADR 0046 already chose for connectors, and the one FSL
  versions turned into after two years.

AGPL was not chosen: many companies ban it, and it does little against a
competitor when the code runs on users' own machines. MPL-2.0 was not
chosen: its file-level copyleft adds conditions without stopping a closed
product built around the files.

## Decision

1. **The whole repository is licensed Apache-2.0**: the standard text in
   `LICENSE`, and a `NOTICE` file with the copyright line ("Copyright 2026
   Chris Adkin"), which redistributions must keep. This includes the
   prototype, `pkg/`, the Claude Code plugin and future connectors; there is
   no separate connector licence any more.
2. **Release files say so.** Archives and Linux packages include `LICENSE`
   and `NOTICE`; the package metadata, the Homebrew cask and the plugin
   manifest name `Apache-2.0`.
3. **Team features stay apart.** `internal/collector/` and its successors
   stay in one place, as [ADR 0008](0008-packaging-and-distribution.md)
   laid out. Whether they are commercial, and under what licence, is decided
   by the record M2 starts with.
4. **Carried over from ADR 0046:** outside contributions are not merged
   without a contributor licence agreement (decision 4), and the licence
   does not cover the name "Trimloop" (decision 5; Apache-2.0 section 6
   also excludes trademarks).

## Consequences

- Anyone may use, change, redistribute and sell trimloop, including in a
  competing product, as long as they keep the licence and notice. This
  accepts the risk ADR 0046 guarded against, in exchange for easier
  adoption and trust.
- The README, the release notes and the package metadata say Apache-2.0.
  The draft `v0.1.0` release was built under FSL-1.1-ALv2 and never
  published; the first public release is cut from a fresh tag
  ([ADR 0052](0052-third-review-corrections.md)), so it ships with this
  licence.
- With a permissive licence, the contributor agreement matters less for
  relicensing, but it is kept until the maintainer decides otherwise (a
  lighter Developer Certificate of Origin is the usual alternative).
- As with ADR 0046, a lawyer should review the licence, the copyright
  holder, the trademark and how far copyright covers code written with an
  AI assistant before the first public release.
