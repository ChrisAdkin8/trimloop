# Roadmap

Each milestone is usable on its own. Ship order is chosen so developers get
value before they have to trust anything in their request path: first
transcripts on disk (M0), then a gateway the team already runs (M1); a proxy on
developer machines comes last (M4).

## M0 — Transcript analyser (MVP)

**Goal:** `trimloop analyse` reads existing Claude Code transcripts and prints a
findings report with estimated savings. No proxy, no server.

- Transcript importer for Claude Code (`~/.claude/projects/**/*.jsonl`).
- Keyed fingerprints and `trimloop key init` (per-machine key for solo use;
  [ADR 0011](docs/adr/0011-keyed-fingerprints.md)).
- Normaliser: segmenting, redaction, fingerprinting, pricing.
- DuckDB store.
- Detectors P01–P07.
- CLI report (terminal + static HTML).
- Fixture corpus of real (redacted) sessions; detector unit tests.
- Aggregate runner for the cross-session detectors (P06, P07).
- Release pipeline: signed binary on GitHub Releases, `.deb` / `.rpm`
  packages and a Homebrew tap ([packaging.md](docs/packaging.md)).

**Done when:** on a sample of real sessions, findings are reproducible and a
developer agrees at least half of the top-5 are worth fixing.

## M1 — Gateway sidecar and LiteLLM plugin, observe-only

**Goal:** a team running LiteLLM installs one plugin and one sidecar and gets
the same findings as `trimloop analyse`, live, for every developer — with no
software on developer machines ([ADR 0009](docs/adr/0009-gateway-sidecar-before-local-proxy.md)).

- Organisation fingerprint key distributed to sidecars (environment variable
  or Kubernetes secret); `fp_key_id` mismatch detection.
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
- Conformance kit (`trimloop conformance`) for provider and harness profiles.
- Local web UI: sessions, turns, prefix-divergence view.

**Done when:** on the same sessions, the sidecar's findings match
`trimloop analyse`; the plugin adds no latency to model calls; killing the
sidecar causes zero failed model calls.

## M2 — Team collector and recommendations

- Hashes-only upload from sidecars and developer machines.
- ClickHouse/Postgres collector: container image and Helm chart collector
  mode ([packaging.md](docs/packaging.md)).
- Root-cause grouping and ranked recommendations.
- L1 actuators: config-patch diffs, optionally opened as PRs.
- Before/after savings measurement.
- OTLP receiver for harnesses that export OpenTelemetry.
- Gateway log connectors for gateways without a plugin (hosted gateways'
  log exports and usage APIs).

## M3 — Safe in-path transforms

- Outcome signals (git, tests, interrupts).
- Decide path through the LiteLLM plugin first (`/v1/decide`, 25 ms budget,
  fail-open).
- Shadow → canary → on rollout, kill switch, signed policies.
- L2 transforms: deterministic tool ordering, volatile-segment reordering,
  cache-breakpoint placement, TTL selection.
- Detectors P08–P12.

## M4 — Local proxy and broader coverage

- Local proxy for teams without a gateway: forward proxy with per-harness CA
  scoping, reverse-proxy fallback, live capture including cache-usage fields,
  fail-open and latency-budget tests; L2 transforms through the proxy.
- More harnesses (Gemini CLI, others) and gateways (Portkey, Helicone,
  OpenRouter).
- Envoy integration (`trimloop serve --extproc`) and the `trimloop-kong`
  binary.
- Turn-complexity classifier from outcome data.
- L3 transforms behind opt-in: output truncation with pointers, model routing,
  thinking-budget caps.
- Detectors P13–P18 (P16–P18: skill and documentation-retrieval
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
