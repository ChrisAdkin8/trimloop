# Roadmap

Each milestone is usable on its own. Ship order is chosen so developers get
value before they have to trust anything in their request path: first
transcripts on disk (M0), then a gateway the team already runs (M1); a proxy on
developer machines comes last (M4).

## M0 — Transcript analyser (MVP)

**Goal:** `trimloop analyse` reads existing Claude Code transcripts and prints a
findings report with estimated savings. No proxy, no server.

- Transcript importer for Claude Code (`~/.claude/projects/**/*.jsonl`).
  Done in the prototype.
- Keyed fingerprints and `trimloop key init` / `import` / `status`
  (per-machine key for solo use; [ADR 0011](docs/adr/0011-keyed-fingerprints.md),
  [ADR 0019](docs/adr/0019-fingerprint-key-management.md)).
- Content-addressed segment storage: each segment stored once, turns hold
  references ([ADR 0017](docs/adr/0017-content-addressed-segments.md)).
- Normaliser: segmenting, redaction, fingerprinting, pricing.
- DuckDB store. Not built: the prototype keeps everything in memory for one
  run, and weekly figures in a history file. As ADR 0029 item 4 says, the
  store follows when needed: P06 needs it. It holds no content before
  redaction is built (item 7; ADR 0004).
- Detectors P01–P07, P09, P10, P12, P14 and P19 ([ADR 0027](docs/adr/0027-measured-m0-findings.md),
  [ADR 0028](docs/adr/0028-harness-recorded-signals.md)), and P13 for
  sub-agents in reduced fidelity ([ADR 0031](docs/adr/0031-m0-order-from-measured-spend.md)).
- Reader also takes the harness's own cost and compaction records. Done in
  the prototype: reconciliation reports the gap with them (about 13 %).
  ADR 0028's acceptance test, within 2 % with auxiliary turns added, is not
  yet passed: auxiliary turns come after M0 ([ADR 0036](docs/adr/0036-prices-reconciliation-status.md)).
- Live nudges in Claude Code: status line, after-reply and before-send hooks
  ([ADR 0034](docs/adr/0034-live-nudges.md)). Done in the prototype.
- CLI report (terminal + static HTML). Done in the prototype: a summary, the
  full text, a self-contained HTML page and JSON (`--format`, `--out`,
  `--no-titles`), with sessions named by title and likely savings beside the
  upper bounds ([ADR 0032](docs/adr/0032-readable-reports.md)).
- Minimal conformance runner: profile schema, capability claims against
  fixtures, golden events — so the M0 profiles pass it before being enabled.
- Fixtures: synthetic sessions in the repository; real (redacted) sessions
  only in a private, access-controlled store, never committed. Detector unit
  tests.
- Local store protected by file permissions; encryption deferred until
  full-content capture ([ADR 0015](docs/adr/0015-defer-local-store-encryption.md)).
- Aggregate runner for the cross-session detectors (P06). P07 is built as a
  per-session Go detector ([ADR 0033](docs/adr/0033-reconciliation-history-standing-context.md)).
- Release: an installable release first (ADR 0029 item 10); the signed
  pipeline — signed binary on GitHub Releases, `.deb` / `.rpm` packages and a
  Homebrew tap ([packaging.md](docs/packaging.md)), proved by a dry-run of
  the per-OS build ([ADR 0014](docs/adr/0014-release-builds-per-os.md)) —
  can follow within M0 or in M1.

**Build order** ([ADR 0029](docs/adr/0029-m0-build-order-by-user-value.md),
re-ranked from measured spend by [ADR 0031](docs/adr/0031-m0-order-from-measured-spend.md)).
Done: reader, usage de-duplication, pricing, P19 and P09; P04 (context
composition by tool and command shape, with fixes from the ADR 0030
ladder); sub-agent economics and P13 in reduced fidelity; P19 concentration
and compaction point, with P10 from compaction records; spend by project,
skill, plugin and MCP server; reconciliation with the harness cost record
(the gap is reported; the 2 % test waits for auxiliary turns);
P07 priced by carried cost, including the system prompt and tool
definitions; weekly figures kept between runs
([ADR 0033](docs/adr/0033-reconciliation-history-standing-context.md));
live nudges in Claude Code — status line, after-reply and before-send hooks,
and `trimloop setup` ([ADR 0034](docs/adr/0034-live-nudges.md)).

1. Saved offsets, so a re-run reads only new lines.
2. Command-line options: `--since`, `--repo` (`analyse`, `--format` and
   `--out` are done).
3. P14 in reduced fidelity: spend by effort level. Done in the prototype ([ADR 0042](docs/adr/0042-p02-p14-and-main-session-models.md)).
4. P02 cache-lifetime fit. Done in the prototype, with P13 for main sessions ([ADR 0042](docs/adr/0042-p02-p14-and-main-session-models.md)).
5. P03, P06 (with the DuckDB store, numbers only until item 7), P01; then P05, and P12 in reduced fidelity from harness
   records (its full fidelity, and P10's, stays in M3).
6. Distribution: installable release, minimal conformance runner, profiles.
7. Keyed fingerprints, key commands, segmentation, redaction, exact hashes.

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
2. **Licence and business model.** Open-source the agent and detectors
   (distribution, trust) with a paid team collector? Or source-available?
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
