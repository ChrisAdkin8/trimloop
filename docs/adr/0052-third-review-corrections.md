# ADR 0052: Going public from a fresh tag, refused requests, and unused app servers

- Status: Accepted — built: the release workflow, the failure and limit sorting and the P06 card; the steps for going public are for the maintainer
- Date: 2026-11-09
- Amends: [ADR 0051](0051-private-releases-and-going-public.md) (decision 2, the order of going public, is replaced), [ADR 0050](0050-second-gap-analysis-findings.md) (decisions 2 and 3), [ADR 0048](0048-usage-limits-for-subscribers.md) (which messages are limit hits)

## Context

A third review of the repository found:

- **The going-public steps of ADR 0051 would not work.** They re-run the
  release workflow for the `v0.1.0` tag. A re-run builds the commit the tag
  points at, with that commit's settings: v0.1.0 is six commits old, and its
  cask caveat still says the checksums are signed. A re-run also replays the
  push event, which says the repository was private, so it would skip the
  signature again.
- **The git history becomes public with the repository.** Every commit
  carries the author's email address, and an older example report in the
  history shows session and connector identifiers.
- **Requests refused before any work was counted as re-sent.** A rate limit
  (429), an overloaded service (529) or a server that could not be reached
  (`ENOTFOUND`, `ECONNREFUSED`) is not charged, so nothing was paid twice.
  Claude Code marks the last two with a server error, so the error field
  alone sorted them as network failures. ADR 0050 named three kinds of
  failure; the code already had a fourth, "not sent", for an expired login,
  with no record of it.
- **Other wordings of a usage limit** ("5-hour limit reached", "Weekly limit
  reached", "usage limit reached") would have been counted as network
  failures. The founder's logs use only "You've hit your … limit".
- **Unused desktop-app servers were filed under P07**, bloated context. P06,
  unused tool definitions, describes them exactly: MCP servers defined on
  every request but never called.
- **Some pinned tools are not updated by Dependabot**: GoReleaser, cosign,
  golangci-lint, govulncheck, Claude Code in CI, and the tools
  `scripts/bootstrap.sh` installs on Linux.

## Decision

1. **Going public, in this order** (replaces ADR 0051 decision 2):
   1. Decide whether to rewrite or squash the git history first; it is the
      maintainer's call, and it cannot be taken back once public.
   2. Make the trimloop repository public.
   3. Delete the v0.1.0 draft release and its tag, then tag the current
      commit and push the tag. The release is built from that commit.
   4. Check the draft has `checksums.txt.sigstore.json` and provenance, then
      publish it.
   5. Make the tap public. The release has by then rewritten the cask, whose
      old caveat claimed signed checksums.
2. **The release job asks GitHub whether the repository is private when the
   job runs**, not what the push event recorded; if it cannot tell, it skips
   the signature. The workflow runs only for version tags (`v1.2.3`,
   `v1.2.3-rc.1`).
3. **P12 sorts a failure as "not sent" when it was refused before any work
   was done** (amends ADR 0050 decision 2): an expired login, a bad request,
   a billing error, a rate limit, an overloaded service, or a server that
   could not be reached. These are counted apart and cost nothing. "Network"
   keeps connections lost mid-response, timeouts and server errors. The text
   is read before the error field.
4. **A limit hit is any of Claude Code's limit messages** (amends ADR 0048):
   "You've hit your … limit", "… limit reached" (not a rate limit) and
   "usage limit". "Session" covers the 5-hour limit; "weekly" any weekly one.
5. **Unused desktop-app servers are P06**, in reduced fidelity (amends ADR
   0050 decision 3): servers the desktop app adds, defined but never called
   in the period. The card and its figure are unchanged. P06's full form,
   per repository over a rolling window, is still planned.
6. **Versions pinned by hand are listed** in
   [packaging.md](../packaging.md#pinned-by-hand) and checked at each
   release.

## Consequences

- On the founder's logs (2026-11-09) three failures move from network to
  not sent: network 5 → 2, not sent 1 → 4, re-sent at least $4.29 → $4.10.
  No other figure changes.
- v0.1.0 is rebuilt from a fresh tag after going public; the draft built
  while private is deleted, not published.
- The report shows the unused desktop-app servers under P06; the JSON's
  `actions` say `"pattern": "P06"` for that card.
- Fixed in the same change, as corrections that keep their decisions: CI and
  `scripts/check.sh` validate the plugin as well as the marketplace (ADR
  0051 decision 6 checked only the marketplace); `--no-titles` hides
  connector identifiers everywhere on the web page and in the text; a turn
  after a refused first request is still a session's first.
