# Architecture decision records

Each record states one decision, its context and its consequences. Later
records amend or supersede earlier ones; each record's status line names
what changed it.

## Start here

- **How a figure is worked out today:** [measurements.md](../measurements.md).
- **The measured figures:** [log-signals.md §8](../log-signals.md#8-measured-figures-at-a-glance).
- **The measurement method in force, in one record:** [ADR 0045](0045-m0-measurement-method.md).
  It replaces eight records written one per review round (0035–0041, 0044),
  which stay as history.
- **The build order:** [ROADMAP](../../ROADMAP.md).

Which record governs what in the M0 prototype:

| Topic | Governing ADRs |
|---|---|
| What the prototype measures, and how | [0045](0045-m0-measurement-method.md), [0050](0050-second-gap-analysis-findings.md) |
| Which findings are in M0, and in what order | [0027](0027-measured-m0-findings.md), [0031](0031-m0-order-from-measured-spend.md), [0042](0042-p02-p14-and-main-session-models.md) |
| How fixes are chosen and worded | [0030](0030-how-fixes-are-chosen.md) |
| What reports show | [0032](0032-readable-reports.md), [0048](0048-usage-limits-for-subscribers.md) (subscribers) |
| Reconciliation and weekly history | [0033](0033-reconciliation-history-standing-context.md), [0043](0043-weekly-trend-for-new-findings.md) |
| Live nudges | [0034](0034-live-nudges.md) |
| Licensing | [0046](0046-licensing.md) |
| Releases | [0047](0047-first-release-pure-go.md), then [0014](0014-release-builds-per-os.md) once cgo arrives; [0049](0049-claude-code-plugin.md) (the plugin); [0051](0051-private-releases-and-going-public.md) (private releases, going public) |
| Privacy | [0004](0004-local-first-hashes-only.md), [0011](0011-keyed-fingerprints.md), [0032](0032-readable-reports.md) (the local-report exception) |

## All records

"Built" says whether the M0 prototype implements a record; most records from
0005 to 0025 are design for later milestones (M1 gateway, M2 team collector,
M3 and M4 automatic fixes). "In force" says whether a record still governs:
**Yes**, **Amended** (in force, with later records changing parts of it), or
**Superseded** (kept as history; the record named replaces it).

| ADR | Decision | Area | Built | In force |
|---|---|---|---|---|
| [0001](0001-capture-proxy-otel-transcripts.md) | Capture through four taps — proxy, OTel, transcripts, gateway connectors | Capture | Partly: the transcripts tap | Amended by 0009, 0011 |
| [0002](0002-go-for-agent-and-proxy.md) | Go for the agent, proxy and CLI | Language | Yes | Amended by 0008 |
| [0003](0003-duckdb-local-clickhouse-team.md) | DuckDB locally, ClickHouse for teams | Storage | No | Amended by 0010 |
| [0004](0004-local-first-hashes-only.md) | Local-first, hashes-only by default | Privacy | Partly | Amended by 0011, 0032 |
| [0005](0005-thin-python-plugin-for-litellm.md) | Go core, thin Python plugin for LiteLLM | Gateway plugin (M1) | No (design) | Amended by 0006 |
| [0006](0006-gateway-plugin-forwards-to-sidecar.md) | Gateway plugins forward raw events to a co-located sidecar | Gateway plugin (M1) | No (design) | Yes |
| [0007](0007-recommend-skills-and-doc-retrieval.md) | Recommend skills and documentation retrieval; do not host RAG | Recommendations (M4) | No (design) | Yes |
| [0008](0008-packaging-and-distribution.md) | One binary, one image, one Python package | Packaging | No (design) | Amended by 0014, 0046, 0049 |
| [0009](0009-gateway-sidecar-before-local-proxy.md) | Ship the gateway sidecar before the local proxy | Build order (M1) | No (design) | Yes |
| [0010](0010-go-detectors-sql-aggregates.md) | Detectors are Go state machines; SQL only for cross-session aggregates | Detectors | Partly: Go detectors, as one-pass functions; no incremental interface or SQL | Yes |
| [0011](0011-keyed-fingerprints.md) | Keyed fingerprints (HMAC-SHA-256) with a customer-held key | Privacy | No (design) | Amended by 0019, 0024 |
| [0012](0012-gateway-sessions-and-parity.md) | Gateway sessions across replicas, M1 parity, and idle windows | Gateway sessions (M1) | No (design) | Amended by 0016, 0020 |
| [0013](0013-decide-path-and-transform-safety.md) | Self-contained decide requests, gateway quality signals, transform safety | Automatic fixes (M3) | No (design) | Amended by 0018, 0020, 0021 |
| [0014](0014-release-builds-per-os.md) | Per-OS release builds with open-source GoReleaser | Packaging | No (design) | Amended by 0047 |
| [0015](0015-defer-local-store-encryption.md) | Defer local store encryption until full-content capture | Storage | No (design) | Yes |
| [0016](0016-m1-sidecar-operation.md) | M1 sidecar operation — output, ordering, forwarded body, history edits | Sidecar (M1) | No (design) | Amended by 0020, 0022, 0023, 0025 |
| [0017](0017-content-addressed-segments.md) | Store segments once, content-addressed | Storage | No (design) | Yes |
| [0018](0018-rollout-refinements.md) | Rollout refinements — experiment buckets, sub-agents, rejection trigger, tool sorting | Rollout (M3) | No (design) | Amended by 0020, 0021 |
| [0019](0019-fingerprint-key-management.md) | Fingerprint key management | Privacy | No (design) | Yes |
| [0020](0020-session-identity.md) | Session identity that survives system-prompt churn | Sessions (M1) | No (design) | Yes |
| [0021](0021-decide-body-ops-and-policy-signing.md) | Decide body format, the op catalogue, and policy signing | Automatic fixes (M3) | No (design) | Yes |
| [0022](0022-sidecar-deployment-and-storage.md) | Sidecar deployment, storage and the local single-writer rule | Sidecar (M1) | No (design) | Yes |
| [0023](0023-content-addressed-plugin-transport.md) | Content-addressed transport between plugin and sidecar | Plugin transport (M1) | No (design) | Yes |
| [0024](0024-developer-identity-and-key-handling.md) | Developer identity, and keeping the key from the coding agent | Privacy | No (design) | Yes |
| [0025](0025-exact-hash-order-preserving-json.md) | Exact hash over order-preserving compact JSON, at every capture point | Fingerprints | No (design) | Yes |
| [0026](0026-usage-dedup-across-logs.md) | Count usage once per message id across all logs | Measurements (M0) | Partly: within one run; ids and offsets are not kept between runs | Amended by 0045 |
| [0027](0027-measured-m0-findings.md) | Add P19 large context and bring P09 into M0, from measured sessions | Findings (M0) | Yes | Amended by 0028, 0045 |
| [0028](0028-harness-recorded-signals.md) | Use the signals harnesses record — accounting, injected context, effort, retries, outcomes | Log signals (M0) | Partly | Amended by 0033, 0042, 0045 |
| [0029](0029-m0-build-order-by-user-value.md) | M0 build order by value to the user | Build order (M0) | An order | Superseded by 0031 |
| [0030](0030-how-fixes-are-chosen.md) | How fixes are chosen — the fix ladder, failure-preserving cuts, measure after | Fixes | Partly | Amended by 0032, 0045 |
| [0031](0031-m0-order-from-measured-spend.md) | Re-rank M0 from measured spend; sub-agent model fit in M0 | Build order (M0) | Mostly: items 1–4, 6, 9, 10; 5, 7, 8 partly; 11 not built | Amended by 0042, 0045 |
| [0032](0032-readable-reports.md) | Readable reports — session titles, likely savings, plain language | Reports (M0) | Yes | Amended by 0045, 0048 |
| [0033](0033-reconciliation-history-standing-context.md) | Reconciliation, weekly history, and pricing standing context | Findings (M0) | Yes | Amended by 0043, 0045, 0048, 0050 |
| [0034](0034-live-nudges.md) | Live nudges in Claude Code | Live nudges (M0) | Yes | Amended by 0045, 0048 |
| [0035](0035-measurement-corrections.md) | Measurement corrections and how actions are ranked | Measurements (M0) | Yes | Superseded by 0045 |
| [0036](0036-prices-reconciliation-status.md) | Prices checked against the pricing page, and what "reconciliation done" means in M0 | Measurements (M0) | Yes | Superseded by 0045 |
| [0037](0037-partial-weeks-prompts-compaction-nudges.md) | Partly deleted weeks, breaks the user took, and nudges after a compaction | Measurements (M0) | Yes | Superseded by 0045 |
| [0038](0038-real-size-after-compaction.md) | The real size after a compaction, and who ended a wait | Measurements (M0) | Yes | Superseded by 0045 |
| [0039](0039-rebuilt-cache-and-likely-ranges.md) | Removed history on a rebuilt cache, and likely savings as a range | Measurements (M0) | Yes | Superseded by 0045 |
| [0040](0040-position-pricing-and-one-method-page.md) | Removed history priced by position, and one page for the current method | Measurements (M0) | Yes | Superseded by 0045 |
| [0041](0041-compaction-edges-and-quartile-rule.md) | Compaction edge cases and the quartile rule | Measurements (M0) | Yes | Superseded by 0045 |
| [0042](0042-p02-p14-and-main-session-models.md) | P02, P14 and main-session models in M0 | Findings (M0) | Yes | Amended by 0043, 0045, 0050 |
| [0043](0043-weekly-trend-for-new-findings.md) | Weekly figures for P02, P14 and main-session models | Reports (M0) | Yes | Amended by 0045 |
| [0044](0044-effort-thinking-and-cache-figures.md) | Effort by thinking tokens, and fairer cache figures | Measurements (M0) | Yes | Superseded by 0045 |
| [0045](0045-m0-measurement-method.md) | The M0 measurement method | Measurements (M0) | Yes | Amended by 0048, 0050 |
| [0046](0046-licensing.md) | Licensing — FSL core, Apache-2.0 connectors, commercial team features | Licensing | Partly: LICENSE.md | Yes |
| [0047](0047-first-release-pure-go.md) | First releases built in one job while trimloop is pure Go | Packaging | Yes: not yet published | Amended by 0049, 0051 |
| [0048](0048-usage-limits-for-subscribers.md) | Usage limits for subscribers | Reports (M0) | Yes | Yes |
| [0049](0049-claude-code-plugin.md) | A Claude Code plugin with a trimloop skill | Packaging | Yes: not yet tried by users | Yes |
| [0050](0050-second-gap-analysis-findings.md) | Findings from the second gap analysis | Findings (M0) | Yes | Yes |
| [0051](0051-private-releases-and-going-public.md) | Releases while private, the order of going public, and release hardening | Packaging | Yes | Yes |

Statuses: **Proposed** means decided for planning but not yet built;
**Accepted** means the decision is in force, and says how much of it is
built.

## Adding a record

A new decision, or a change to one, gets a new record that names what it
amends or supersedes; update both records' status lines, this index (its
"Built" and "In force" columns) and the [ROADMAP](../../ROADMAP.md). A
correction that keeps a decision (a bug fix, a test, a wording change) goes
in the commit message and, for a measurement, in
[measurements.md](../measurements.md): it does not get a record.
