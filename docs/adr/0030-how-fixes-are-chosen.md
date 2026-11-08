# ADR 0030: How fixes are chosen — the fix ladder, failure-preserving cuts, measure after

- Status: Accepted — partly built (fix kinds and alternatives; measuring after a fix is not built yet) — amended by [ADR 0032](0032-readable-reports.md)
- Date: 2026-11-07
- Amends: [design.md §6.2](../design.md#62-recommendation-engine-team-collector) (recommendation ranking), [data-model.md](../data-model.md) (Recommendation)

## Context

Patterns list fixes, but nothing said how Trimloop chooses among them or how
it presents them. For noisy tool output (P04) there are at least six kinds of
fix — steering the agent through a memory file, enforcing with a hook,
capping output in the harness's settings, fixing the noisy tool itself,
keeping noise out of the main session, and trimming output in the request
path — with different strengths, costs and risks. Two risks recur across
patterns:

- **Over-cutting.** A cut that hides the line that mattered (an error, a
  failing test) makes the agent re-run commands or read more files, which
  costs more than the noise did.
- **Unproven fixes.** The core promise is recommend, apply, measure
  (design §6.2). A fix that is never measured cannot be shown to work, or
  to have stopped working.

## Decision

**1. Fix kinds, from most to least lasting.** Every recommendation's fix is
one of these kinds:

| Kind | What it changes | Example (P04) |
|---|---|---|
| `source` | The tool or command produces less | Quiet flags, a failures-only test wrapper, an MCP server returning summaries or pages, an unused tool disabled |
| `structure` | Where the work happens, so noise is not carried | Exploration delegated to a sub-agent; a new session or compaction after exploring; tuning when old tool output is cleared |
| `enforce` | The harness stops or rewrites the noisy action | A pre-tool hook that warns, blocks with a reason, or adds a filter |
| `cap` | A hard limit in the harness's settings | A lower output limit for shell or MCP results |
| `advise` | Instructions the agent may follow | A memory-file or skill line |
| `trim` | Output cut in the request path (L3) | `truncate_tool_result` (M4) |

**2. Ranking.** Candidate fixes for a finding are ranked by expected saving ×
confidence ÷ effort to apply (as design §6.2 ranks recommendations). Ties go
to the more lasting kind, in the order above. A recommendation shows **one
primary fix and at most two alternatives of different kinds**, so the user
sees a choice of strength — for example `advise` now, `enforce` if the habit
persists, `source` where the tool can be changed.

**3. Failure-preserving cuts.** Any fix that removes output (`cap`, `trim`,
an `enforce` that adds a filter, a `source` fix that changes what a tool
prints) must keep failures visible: exit status, error output, and the
summary or failing items of tests and builds. Fix templates are written in a
failure-preserving form (for example, keeping the end of a test run, where
failures are summarised, rather than the start), and a template that cannot
guarantee this is not offered for commands whose failures matter.

**4. Measure after, then escalate or roll back.** When a recommendation is
applied, Trimloop measures the same group — same repo, developer cohort and,
for P04, command shape — over a comparison window (default 7 days of
activity):

- **Saving**: the group's carried cost before and after.
- **Side effects**: re-runs of the same command shape, turns per session,
  tool error rate and extra reads in the group — signs that a cut removed
  something the agent needed.

If the saving is not seen, the recommendation is re-offered at the next
stronger kind (`advise` → `enforce` → `cap`). If side effects rise beyond a
threshold, Trimloop recommends rolling the fix back and records why.

**5. Harness-specific rendering.** Each fix kind has templates per harness
(`templates/<harness>/`); a kind the harness cannot express (for example,
hooks in a harness without them) is not offered there.

## Consequences

- Recommendations carry a fix kind and alternatives, and every applied one
  has a measured outcome, including "did not help" and "rolled back".
- Fix templates need review for failure preservation before they ship.
- Measurement needs results kept between runs (ADR 0029, item 4) and, for
  side effects, the outcome signals of ADR 0028.
- The ladder applies to every pattern, not only P04; patterns state their
  fixes by kind as they are revised.
