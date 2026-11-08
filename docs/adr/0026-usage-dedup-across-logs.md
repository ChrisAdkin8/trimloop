# ADR 0026: Count usage once per message id across all logs

- Status: Accepted — built in the M0 prototype
- Date: 2026-11-06
- Amends: Reader contract in [analyzer.md §4](../analyzer.md#key-interfaces-sketch)

## Context

The Reader contract said message ids already counted are stored **per log**,
so usage repeated across lines (or across runs) is counted once. Running the
M0 prototype on real Claude Code transcripts showed that is not enough: 120
message ids appear in more than one log file, mostly when a resumed session
copies earlier messages into its new transcript. Per-log de-duplication
counts each of those messages once per file, which inflates usage and cost.

The data model already states the intended rule: usage is counted once per
provider response ([data-model.md, usage normalisation](../data-model.md#usage-normalisation)).

## Decision

- Usage is de-duplicated by message id across **every log a run reads**, not
  per log. The oldest log that contains an id (by its earliest message
  time) is credited with it, so a resumed session never takes over turns
  from the session it resumed.
- `Reader.Read` yields every line that carries usage, repeats included. The
  de-duplication set is held by the caller (the engine), not by the Reader,
  because only the caller sees all logs.
- Across runs, the set of counted ids is persisted with the store, alongside
  each log's offset, so incremental re-runs keep the same rule.
- A message with no id is counted every time it appears; there is nothing to
  match repeats on.

## Consequences

- Totals match the data-model rule. On the founder's transcripts this removes
  the double-counting of 120 messages.
- Logs must be ordered by time before counting, which means reading each
  log's first message before crediting any. Per-session totals for resumed
  sessions still need the resume chain (`resumed_from`, P09) to be explained
  correctly; the run's grand total does not.
- Readers stay simple and stateless; a new harness Reader does not need its
  own de-duplication.
- The persisted id set grows with history. It is bounded by transcript
  retention (`cleanupPeriodDays`) and can be pruned with the logs it came
  from.
