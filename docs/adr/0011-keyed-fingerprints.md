# ADR 0011: Keyed fingerprints (HMAC-SHA-256) with a customer-held key

- Status: Proposed — key management amended by [ADR 0019](0019-fingerprint-key-management.md); developer identity and key storage on developer machines by [ADR 0024](0024-developer-identity-and-key-handling.md)
- Date: 2026-11-05
- Amends: [ADR 0004](0004-local-first-hashes-only.md) (what "hashes-only" protects), [ADR 0001](0001-capture-proxy-otel-transcripts.md) item 4 (connectors run in `serve` connector mode)

## Context

Detectors compare fingerprints rather than text
([data-model.md](../data-model.md#why-fingerprints-are-needed)). The first
specification used plain SHA-256. A plain hash cannot be reversed, but anyone
who can see the codes can **test a guess**: hash a likely file path, URL, short
instruction or suspected secret and look for a match. Paths, repo URLs,
documentation URLs and short user messages are easy to guess. Repo display
names, branch names and MCP server and tool names were also sent in plain
text. The cold review rated this the most underplayed risk: plain hashes give
pseudonymity, not the "no code or names leave your network" promise the docs
made.

## Decision

1. **Algorithm.** Every fingerprint is
   `HMAC-SHA-256(fingerprint_key, label ‖ 0x00 ‖ data)`, with a context label
   per purpose from a fixed registry ([data-model.md](../data-model.md#specification)).
   Labels keep codes made for different purposes from ever being equal.
2. **The key belongs to the customer.** A 256-bit random key per organisation
   (or per machine, for a solo user).
   - `trimloop key init` creates it. Solo: stored in the OS keychain where
     there is one, else `~/.trimloop/fingerprint.key` with mode 0600.
   - Teams: an administrator creates one key and distributes it to developer
     machines (OS credential store only, ADR 0024) and gateway sidecars —
     environment variable `TRIMLOOP_FINGERPRINT_KEY`, a file path in
     `trimloop.yaml` (servers, containers and CI only), or a
     Kubernetes secret referenced by the Helm chart.
   - The team collector never needs the key; detectors there compare codes
     only. A hosted collector never receives it.
3. **Key identity.** Every turn records `fp_key_id`, derived from the key.
   The collector refuses to compare codes with different key ids and flags a
   machine or sidecar using the wrong key.
4. **Names are keyed by default.** File paths, URLs, repo, branch, MCP server
   and MCP tool names are sent as keyed hashes. An organisation may opt in to
   plain text per field (`privacy.plain_names`: any of `repo`, `branch`, `path`,
   `url`, `mcp`, `developer`, `alias`) for
   readable team dashboards. The harness's built-in tool names stay plain.
5. **Evidence is typed.** Finding evidence holds ids, codes and counts only —
   no free text.
6. **Rotation.** `trimloop key rotate` creates a new key (new `fp_key_id`).
   Codes before and after rotation cannot be compared; reports say so rather
   than showing a false drop in repeats. Locally, `trimloop analyse --rebuild`
   re-fingerprints everything still in the harness's logs. Gateway history
   cannot be re-keyed. A lost key is handled as a rotation.

## What it protects and what it does not

| Protected against anyone without the key | Not protected |
|---|---|
| Confirming a guessed file path, URL, repo, branch, instruction or secret | Holders of the key — anyone who can run Trimloop with it can make matching codes |
| Learning names from a database leak, a backup, or a hosted collector | Patterns: counts, sizes, timing, costs, and which sessions share an unnamed item |
| Linking codes across organisations (different keys give different codes) | Plain-text fields an organisation has opted in to |

The documented promise, stated per mode in [ADR 0019](0019-fingerprint-key-management.md):
in hosted mode **Trimloop's servers cannot read your code or your names, and
cannot test guesses against them**; on a self-hosted collector the same holds
for anyone without the key. It does not claim that nothing can be inferred
from usage patterns.

## Consequences

- The fingerprint spec ([data-model.md](../data-model.md#specification))
  uses HMAC throughout; spec version stays `fp1` because nothing was built
  under plain SHA-256.
- Key distribution is a new setup step for teams and a new Helm value; losing
  the key breaks comparison with history but exposes nothing.
- Anything that fingerprints incoming data needs the key, so gateway log
  connectors and the OTLP receiver do not run inside the collector. They run
  as a separate **connector** mode of `trimloop serve`, deployed by the
  customer with the key, which forwards keyed records to the collector. The
  collector process never holds the key, self-hosted or hosted.
- The Python plugin's remote mode (ADR 0006) needs the key too.
- HMAC costs roughly one extra hash per input; negligible next to parsing.
