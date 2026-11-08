# Architecture decision records

Each record states one decision, its context and its consequences. Later
records amend earlier ones; each record's status line names what amends it.
"Built" says whether the M0 prototype implements it; most records from 0005
to 0025 are design for later milestones (M1 gateway, M2 team collector, M3
and M4 automatic fixes).

| ADR | Decision | Area | Built |
|---|---|---|---|
| [0001](0001-capture-proxy-otel-transcripts.md) | Capture through four taps — proxy, OTel, transcripts, gateway connectors | Capture (M0–M2) | Partly: the transcripts tap |
| [0002](0002-go-for-agent-and-proxy.md) | Go for the agent, proxy and CLI | Language | Yes |
| [0003](0003-duckdb-local-clickhouse-team.md) | DuckDB locally, ClickHouse for teams | Storage (M0, M2) | No |
| [0004](0004-local-first-hashes-only.md) | Local-first, hashes-only by default | Privacy | Partly |
| [0005](0005-thin-python-plugin-for-litellm.md) | Go core, thin Python plugin for LiteLLM | Gateway plugin (M1) | No (design) |
| [0006](0006-gateway-plugin-forwards-to-sidecar.md) | Gateway plugins forward raw events to a co-located sidecar | Gateway plugin (M1) | No (design) |
| [0007](0007-recommend-skills-and-doc-retrieval.md) | Recommend skills and documentation retrieval; do not host RAG | Recommendations (M4) | No (design) |
| [0008](0008-packaging-and-distribution.md) | One binary, one image, one Python package | Packaging | No (design) |
| [0009](0009-gateway-sidecar-before-local-proxy.md) | Ship the gateway sidecar before the local proxy | Order (M1) | No (design) |
| [0010](0010-go-detectors-sql-aggregates.md) | Detectors are Go state machines; SQL only for cross-session aggregates | Detectors | Partly |
| [0011](0011-keyed-fingerprints.md) | Keyed fingerprints (HMAC-SHA-256) with a customer-held key | Privacy | No (design) |
| [0012](0012-gateway-sessions-and-parity.md) | Gateway sessions across replicas, M1 parity, and idle windows | Gateway sessions (M1) | No (design) |
| [0013](0013-decide-path-and-transform-safety.md) | Self-contained decide requests, gateway quality signals, transform safety | Automatic fixes (M3) | No (design) |
| [0014](0014-release-builds-per-os.md) | Per-OS release builds with open-source GoReleaser | Packaging | No (design) |
| [0015](0015-defer-local-store-encryption.md) | Defer local store encryption until full-content capture | Storage | No (design) |
| [0016](0016-m1-sidecar-operation.md) | M1 sidecar operation — output, ordering, forwarded body, history edits | Sidecar (M1) | No (design) |
| [0017](0017-content-addressed-segments.md) | Store segments once, content-addressed | Storage | No (design) |
| [0018](0018-rollout-refinements.md) | Rollout refinements — experiment buckets, sub-agents, rejection trigger, tool sorting | Rollout (M3) | No (design) |
| [0019](0019-fingerprint-key-management.md) | Fingerprint key management | Privacy | No (design) |
| [0020](0020-session-identity.md) | Session identity that survives system-prompt churn | Sessions (M1) | No (design) |
| [0021](0021-decide-body-ops-and-policy-signing.md) | Decide body format, the op catalogue, and policy signing | Automatic fixes (M3) | No (design) |
| [0022](0022-sidecar-deployment-and-storage.md) | Sidecar deployment, storage and the local single-writer rule | Sidecar (M1) | No (design) |
| [0023](0023-content-addressed-plugin-transport.md) | Content-addressed transport between plugin and sidecar | Plugin transport (M1) | No (design) |
| [0024](0024-developer-identity-and-key-handling.md) | Developer identity, and keeping the key from the coding agent | Privacy | No (design) |
| [0025](0025-exact-hash-order-preserving-json.md) | Exact hash over order-preserving compact JSON, at every capture point | Fingerprints | No (design) |
| [0026](0026-usage-dedup-across-logs.md) | Count usage once per message id across all logs | Usage (M0) | Yes |
| [0027](0027-measured-m0-findings.md) | Add P19 large context and bring P09 into M0, from measured sessions | Findings (M0) | Yes |
| [0028](0028-harness-recorded-signals.md) | Use the signals harnesses record — accounting, injected context, effort, retries, outcomes | Log signals (M0) | Partly |
| [0029](0029-m0-build-order-by-user-value.md) | M0 build order by value to the user | Build order (M0) | An order: re-ranked by 0031 |
| [0030](0030-how-fixes-are-chosen.md) | How fixes are chosen — the fix ladder, failure-preserving cuts, measure after | Fixes | Partly |
| [0031](0031-m0-order-from-measured-spend.md) | Re-rank M0 from measured spend; sub-agent model fit in M0 | Build order (M0) | Mostly: items 1–7 |
| [0032](0032-readable-reports.md) | Readable reports — session titles, likely savings, plain language | Reports (M0) | Yes |
| [0033](0033-reconciliation-history-standing-context.md) | Reconciliation, weekly history, and pricing standing context | Findings (M0) | Yes |
| [0034](0034-live-nudges.md) | Live nudges in Claude Code | Live nudges (M0) | Yes |
| [0035](0035-measurement-corrections.md) | Measurement corrections and how actions are ranked | Measurements (M0) | Yes |
| [0036](0036-prices-reconciliation-status.md) | Prices checked against the pricing page; reconciliation reports the gap in M0 | Measurements (M0) | Yes |
| [0037](0037-partial-weeks-prompts-compaction-nudges.md) | Partly deleted weeks, breaks the user took, nudges after a compaction | Measurements, live nudges (M0) | Yes |

Statuses: **Proposed** means decided for planning but not yet built;
**Accepted** means the decision is in force, and says how much of it is
built. The [ROADMAP](../../ROADMAP.md) gives the build order.
