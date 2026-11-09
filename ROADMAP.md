# Roadmap

Each milestone is usable on its own. Ship order is chosen so developers get
value before they have to trust anything in their request path: first
transcripts on disk (M0), then a gateway the team already runs (M1); a proxy on
developer machines comes last (M4).

## M0 — Transcript analyser (MVP)

**Goal:** `trimloop analyse` reads existing Claude Code transcripts and prints a
findings report with estimated savings. No proxy, no server.

Each item is tagged **Done**, **Partly** or **Not built** for the prototype.

- **Done.** Transcript importer for Claude Code (`~/.claude/projects/**/*.jsonl`).
- **Not built.** Keyed fingerprints and `trimloop key init` / `import` /
  `status` (per-machine key for solo use;
  [ADR 0011](docs/adr/0011-keyed-fingerprints.md),
  [ADR 0019](docs/adr/0019-fingerprint-key-management.md)).
- **Not built.** Content-addressed segment storage: each segment stored
  once, turns hold references
  ([ADR 0017](docs/adr/0017-content-addressed-segments.md)).
- **Partly.** Normaliser: pricing is done; segmenting, redaction and
  fingerprinting are not.
- **Not built.** DuckDB store. The prototype keeps everything in memory for
  one run, and weekly figures in a history file. The store follows when
  needed (ADR 0029 item 4): P06 needs it. It holds no content before
  redaction is built (build order item 5; ADR 0004).
- **Partly.** Detectors ([ADR 0027](docs/adr/0027-measured-m0-findings.md),
  [ADR 0028](docs/adr/0028-harness-recorded-signals.md),
  [ADR 0031](docs/adr/0031-m0-order-from-measured-spend.md),
  [ADR 0042](docs/adr/0042-p02-p14-and-main-session-models.md)). Done: P04,
  P07, P09 and P19; in reduced fidelity, P01 (model switches), P02,
  P06 (desktop-app servers never used), P10, P12 (replies cut off), P13
  (sub-agents and main sessions) and P14. Not built: P03, P05, and P06 for
  servers the user added.
- **Partly.** Reader also takes the harness's own cost and compaction
  records: reconciliation reports the gap with them (about 13 %). ADR 0028's
  acceptance test, within 2 % with auxiliary turns added, is not yet passed:
  auxiliary turns come after M0
  ([ADR 0045](docs/adr/0045-m0-measurement-method.md) rule 9).
- **Done.** Live nudges in Claude Code: status line, after-reply and
  before-send hooks, and `trimloop setup`
  ([ADR 0034](docs/adr/0034-live-nudges.md)).
- **Done.** CLI report (terminal + static HTML): a summary, the full text, a
  self-contained HTML page and JSON (`--format`, `--out`, `--no-titles`),
  with sessions named by title and likely savings beside the upper bounds
  ([ADR 0032](docs/adr/0032-readable-reports.md)).
- **Not built.** Minimal conformance runner: profile schema, capability
  claims against fixtures, golden events — so the M0 profiles pass it before
  being enabled.
- **Partly.** Fixtures: synthetic sessions in the repository
  (`testdata/sessions`), detector unit tests, and an end-to-end check of
  the report's figures against an independent recount on made-up logs
  (`testdata/golden`, `scripts/recount.py`) are done; a private,
  access-controlled store of real (redacted) sessions, never committed, is
  not.
- **Done.** Local files protected by file permissions (mode 0600);
  encryption deferred until full-content capture
  ([ADR 0015](docs/adr/0015-defer-local-store-encryption.md)).
- **Not built.** Aggregate runner for the cross-session detectors (P06). P07
  is built as a per-session Go detector
  ([ADR 0033](docs/adr/0033-reconciliation-history-standing-context.md)).
- **Partly.** Release: the installable release is built — one GoReleaser
  job, checksums (signed once the repository is public), `.deb` / `.rpm`, a
  Homebrew cask
  ([ADR 0047](docs/adr/0047-first-release-pure-go.md)); nothing is
  published yet. Still to do with cgo:
  the per-OS signed pipeline — signed binary on GitHub Releases, `.deb` / `.rpm`
  packages and a Homebrew tap ([packaging.md](docs/packaging.md)), proved
  by a dry-run of the per-OS build
  ([ADR 0014](docs/adr/0014-release-builds-per-os.md)) — can follow within
  M0 or in M1.

**Build order** ([ADR 0031](docs/adr/0031-m0-order-from-measured-spend.md),
which re-ranked [ADR 0029](docs/adr/0029-m0-build-order-by-user-value.md)
from measured spend). How each figure is measured is in
[ADR 0045](docs/adr/0045-m0-measurement-method.md) and
[measurements.md](docs/measurements.md).

Done:

- reader, usage de-duplication and pricing;
- P19 (size, concentration and compaction point) and P09;
- P04: context by tool and command shape, with fixes from the ADR 0030
  ladder;
- sub-agent economics and P13 for sub-agents, in reduced fidelity;
- P10 from compaction records;
- spend by project, skill, plugin and MCP server;
- reconciliation with the harness cost record (the gap is reported; the 2 %
  test waits for auxiliary turns);
- P07 priced by carried cost, including the system prompt and tool
  definitions;
- weekly figures kept between runs
  ([ADR 0033](docs/adr/0033-reconciliation-history-standing-context.md),
  [ADR 0043](docs/adr/0043-weekly-trend-for-new-findings.md));
- live nudges in Claude Code and `trimloop setup`
  ([ADR 0034](docs/adr/0034-live-nudges.md));
- P02 cache expiry during short waits, P13 for main sessions and P14 by
  effort level, in reduced fidelity
  ([ADR 0042](docs/adr/0042-p02-p14-and-main-session-models.md));
- usage limits for Pro and Max subscribers: the plan, limit hits, and
  savings as a share of the weekly limit from status-line readings
  ([ADR 0048](docs/adr/0048-usage-limits-for-subscribers.md));
- a Claude Code plugin whose skill runs trimloop and explains the report
  ([ADR 0049](docs/adr/0049-claude-code-plugin.md));
- from a second gap analysis: P01 from model switches, P12 from replies cut
  off, unused desktop-app tools, tool inputs in what fills a session, the
  whole thinking share, the cost to start a session, when you work, and log
  retention ([ADR 0050](docs/adr/0050-second-gap-analysis-findings.md));
  unused desktop-app tools filed as P06, and requests refused before any
  work kept out of P12's re-sent cost
  ([ADR 0052](docs/adr/0052-third-review-corrections.md));
- projects credited by conversation, a card that saves nothing ranked last,
  and re-reads counted by the part of a file viewed
  ([ADR 0053](docs/adr/0053-projects-by-conversation-and-ranking.md)).

To do:

1. Saved offsets, so a re-run reads only new lines.
2. Command-line options: `--repo` (`analyse`, `--format`, `--out`,
   `--open`, `--since` and `--version` are done).
3. P03, P06 (with the DuckDB store, numbers only until item 5), P01's other
   causes (beyond model switches); then P05, and P12 from harness retry
   records (its full fidelity, and P10's, stays in M3).
4. Distribution: publish the first release after going public
   from a fresh tag
   ([ADR 0052](docs/adr/0052-third-review-corrections.md)), minimal
   conformance runner, profiles.
5. Keyed fingerprints, key commands, segmentation, redaction, exact hashes.

**Done when:** on a sample of real sessions, findings are reproducible and a
developer agrees at least half of the top-5 are worth fixing.

## M1 — Gateway sidecar and LiteLLM plugin, observe-only

**Goal:** a team running LiteLLM installs one plugin and one sidecar and gets
the same findings as `trimloop analyse`, live, for every developer — with no
software on developer machines ([ADR 0009](docs/adr/0009-gateway-sidecar-before-local-proxy.md)).

- Organisation fingerprint key distributed to sidecars (environment variable
  or Kubernetes secret); `fp_key_id` mismatch detection.
- **First task:** benchmark the plugin on LiteLLM under realistic load —
  observe path against the ≤ 1 ms p99 budget, and a decide round trip — using
  the content-addressed transport ([ADR 0023](docs/adr/0023-content-addressed-plugin-transport.md));
  also confirm whether the pre-call hook runs on pass-through routes
  ([ADR 0021](docs/adr/0021-decide-body-ops-and-policy-signing.md)).
- Paired fixture capture: run Claude Code through a test LiteLLM and keep
  each session both as transcript and as gateway capture, in the private
  fixture store, for the parity test.
- Findings output without a collector: read-only findings API and page on
  `trimloop serve`, webhook sink, Helm persistence values.
- Plugin forwards the provider-bound request; `normalize/litellm` envelope;
  gateway fingerprints use `fidelity = gateway`.
- Reorder window, session merging and history-edit handling in the
  sessioniser.
- Per-pod sidecar storage, liveness-only probes, native sidecar where
  available; fail-open test on a real cluster; output per replica, merged by
  `trimloop report --from` ([ADR 0022](docs/adr/0022-sidecar-deployment-and-storage.md)).
- Session anchors and joining on the latest response
  ([ADR 0020](docs/adr/0020-session-identity.md)).
- Supported topologies: one gateway instance, or replicas with sticky routing
  by API key or user ([ADR 0012](docs/adr/0012-gateway-sessions-and-parity.md)).
  Session kinds (`main`, `subagent`, `auxiliary`); idle window of at least
  2 hours, extended to the longest observed cache TTL (capped at 24 hours);
  plugin buffer capped by bytes.
- `trimloop serve`: ingest API (`/v1/events`), sessioniser, streaming detector
  driver, memory and DuckDB stores ([analyzer.md](docs/analyzer.md) §5.2, §6).
- `trimloop-litellm` plugin, observe-only: success/failure callbacks, batched
  asynchronous forwarding, fail-open ([ADR 0005](docs/adr/0005-thin-python-plugin-for-litellm.md),
  [ADR 0006](docs/adr/0006-gateway-plugin-forwards-to-sidecar.md)).
- Batch-versus-streaming parity tests; plugin tested against each supported
  LiteLLM version.
- Packaging: container image, Helm chart sidecar mode, Docker Compose
  example, plugin on PyPI, schemas on the Buf registry
  ([packaging.md](docs/packaging.md)).
- Codex CLI transcript reader.
- Full conformance kit (`trimloop conformance`) for provider and harness
  profiles (a minimal runner ships in M0).
- Local web UI: sessions, turns, prefix-divergence view.

**Done when:** on the same fixture sessions, for every detector both sources
can run, finding counts match exactly and estimated savings match within 2 %;
the plugin adds at most 1 ms at p99 to model calls with decide off; killing
the sidecar causes zero failed model calls.

## M2 — Team collector and recommendations

- Hashes-only upload from sidecars and developer machines.
- Collector-side sessionising across gateway replicas, and session-scope
  detectors at the collector when one is configured (ADR 0012).
- ClickHouse/Postgres collector: container image and Helm chart collector
  mode ([packaging.md](docs/packaging.md)).
- Root-cause grouping and ranked recommendations.
- L1 actuators: config-patch diffs, optionally opened as PRs.
- Before/after savings measurement.
- `trimloop serve` connector mode (holds the fingerprint key, forwards keyed
  records): OTLP receiver for harnesses that export OpenTelemetry, and log
  connectors for gateways without a plugin (hosted gateways' log exports and
  usage APIs).

## M3 — Safe in-path transforms

- Outcome signals (git, tests, interrupts).
- Decide path through the LiteLLM plugin first (`/v1/decide`, 25 ms budget,
  fail-open): self-contained decide requests, canary buckets from the
  conversation root, op preconditions, and resend-untransformed on rejection
  ([ADR 0013](docs/adr/0013-decide-path-and-transform-safety.md)).
- Gateway quality signals (errors, retries, turns per session, corrections)
  gate gateway rollouts.
- Shadow → canary → on rollout, kill switch, signed policies.
- L2 transforms: deterministic tool ordering, cache-breakpoint placement
  within provider limits, TTL selection, cache keys.
- Detectors P08, P11; full-fidelity P10 and P12.

## M4 — Local proxy and broader coverage

- Local proxy for teams without a gateway: forward proxy with per-harness CA
  scoping, reverse-proxy fallback, live capture including cache-usage fields,
  fail-open and latency-budget tests; L2 transforms through the proxy.
- More harnesses (Gemini CLI, others) and gateways (Portkey, Helicone,
  OpenRouter).
- Envoy integration (`trimloop serve --extproc`) and the `trimloop-kong`
  binary, after verifying that Kong's Go plugins can read streamed response
  bodies without buffering them.
- Turn-complexity classifier from outcome data.
- L3 transforms behind opt-in: moving volatile segments, output truncation
  with pointers, model routing, thinking-budget caps. At a gateway only once
  developer-side outcome signals can be joined to gateway sessions.
- Detectors P13 (per-turn routing), P15–P18 and full-fidelity P14 (P16–P18: skill and documentation-retrieval
  recommendations, ADR 0007).
- Optional hosted collector.

## Open questions

1. **Name.** Register `trimloop.ai` and claim the GitHub, npm, PyPI and
   Homebrew names (all free on 2026-11-05). Run a trademark search (UK IPO,
   EUIPO, USPTO) before going public. `trimloop.com` belongs to someone else.
   (The working name "Trimtab" was dropped on 2026-11-05: `trimtab.ai` is
   owned by another company and an npm package `trimtab` already targets
   AI-agent behaviour. `.dev` availability was not confirmed.)
2. **Licence and business model.** Settled by
   [ADR 0046](docs/adr/0046-licensing.md): the repository is source-available
   (FSL-1.1-ALv2), connectors will be Apache-2.0, and team features are
   expected to be commercial.
3. **Provider terms.** Confirm that structural request changes (reordering,
   cache markers, TTL choice) are allowed under each provider's terms, and
   that forward-proxying a vendor's harness is acceptable to that vendor.
4. **Certificate pinning.** Which harnesses pin certificates and so cannot be
   forward-proxied? Fallback is transcripts + OTel.
5. **Token counts without a tokenizer.** Where the provider's tokenizer is not
   available locally, how accurate are estimates, and how is that shown?
6. **Outcome signals.** Are git and test signals enough to catch quality
   regressions from L3 transforms, or is a small eval suite per repo needed?
7. **Duplicates across taps.** Best key for de-duplicating the same turn seen
   by the proxy and the transcript importer.
8. **Competitive response.** If harness vendors ship strong built-in caching
   and analytics, does the cross-harness team view still justify a product?
