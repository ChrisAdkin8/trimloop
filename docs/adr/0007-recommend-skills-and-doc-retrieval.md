# ADR 0007: Recommend skills and documentation retrieval; do not host RAG

- Status: Proposed
- Date: 2026-11-05

## Context

Two further sources of waste are visible in coding-agent traffic:

- **Repeated work** — the same workflow rediscovered in many sessions (P16)
  and the same guidance retyped by developers (P17). Harnesses increasingly
  support skills (instructions loaded on demand) or equivalents such as slash
  commands, which also let rarely-needed memory-file sections stop being sent
  on every request (P07).
- **Hot documentation** — the same docs fetched or read again and again
  (P18). Options range from a short summary to a local search tool to full
  retrieval-augmented generation (RAG).

Building and hosting RAG indexes would require Trimloop to store document
content, conflicting with the hashes-only model (ADR 0004). Indexes also go
stale, and a stale index gives agents wrong answers. For code, agents' own
search (grep, file reads) often performs as well as or better than vector
retrieval, so RAG is not a safe default.

## Decision

- Trimloop **detects** P16–P18 and **recommends** fixes at level L1: a skill
  (or harness equivalent), a memory-file section, a short doc summary, a local
  documentation search tool, or — only where measured data supports it — RAG.
- Drafts that need real content (skill files, summaries, task labels) are
  **rendered locally** from transcripts on disk or full local capture. Only
  fingerprints, counts and cost reach the team collector.
- Detection that needs semantic similarity (clustering similar prompts) runs
  **locally only**.
- Trimloop **does not build or host** RAG indexes. It exports a hot-doc list
  (source, frequency, size, volatility) that an external RAG builder can
  consume.
- Recommendations are recorded and measured before and after like any other.

## Consequences

- New capabilities: `harness.skills` (harness supports on-demand skills or an
  equivalent) and `content.local` (raw text readable on this machine when a
  recommendation is rendered).
- Harness profiles gain a `skills` entry describing the harness's mechanism
  and where skill files live.
- Savings from P16–P18 appear as fewer turns per task and less developer
  time, which take longer to measure than cache savings; the value case
  ranks them after the token levers.
- An export format for the hot-doc list is needed; it carries URLs or paths
  in plain text only for repos that opt in to plain paths.
