# Roadmap

Each milestone is usable on its own. Ship order is chosen so developers get
value before they have to trust a proxy in their request path.

## M0 — Transcript analyser (MVP)

**Goal:** `trimloop analyse` reads existing Claude Code transcripts and prints a
findings report with estimated savings. No proxy, no server.

- Transcript importer for Claude Code (`~/.claude/projects/**/*.jsonl`).
- Normaliser: segmenting, redaction, fingerprinting, pricing.
- DuckDB store.
- Detectors P01–P07.
- CLI report (terminal + static HTML).
- Fixture corpus of real (redacted) sessions; detector unit tests.

**Done when:** on a sample of real sessions, findings are reproducible and a
developer agrees at least half of the top-5 are worth fixing.

## M1 — Local proxy, observe-only

- Forward proxy with per-harness CA scoping; reverse-proxy fallback.
- Live capture, including cache-usage fields per turn.
- Local web UI: sessions, turns, prefix-divergence view.
- Codex CLI importer and proxy support.
- Fail-open and latency budget tests.

## M2 — Team collector and recommendations

- Hashes-only upload from agents.
- ClickHouse/Postgres collector, container and Helm chart.
- Root-cause grouping and ranked recommendations.
- L1 actuators: config-patch diffs, optionally opened as PRs.
- Before/after savings measurement.
- Gateway connector: LiteLLM first.

## M3 — Safe in-path transforms

- Outcome signals (git, tests, interrupts).
- Shadow → canary → on rollout, kill switch, signed policies.
- L2 transforms: deterministic tool ordering, volatile-segment reordering,
  cache-breakpoint placement, TTL selection.
- Detectors P08–P12.

## M4 — Broader coverage and routing

- More harnesses (Gemini CLI, others) and gateways (Portkey, Helicone,
  OpenRouter).
- Turn-complexity classifier from outcome data.
- L3 transforms behind opt-in: output truncation with pointers, model routing,
  thinking-budget caps.
- Detectors P13–P15.
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
