# Golden logs: an end-to-end check against an independent recount

`projects/` holds synthetic Claude Code logs, laid out as Claude Code keeps
them (`projects/<encoded-folder>/<session>.jsonl`, sub-agents in
`<session>/subagents/agent-*.jsonl`), so this folder can be used as
`CLAUDE_CONFIG_DIR`. Every name, folder and text in them is made up by
`scripts/gen-golden.py`; no real log is copied.

`expected.json` holds the figures **`scripts/recount.py`** works out from
these logs. The recount is a second implementation of
[`docs/measurements.md`](../../docs/measurements.md), written from the
documents and the log format, not from the Go code, so the two do not share
mistakes. `cmd/trimloop/golden_test.go` runs trimloop's own pipeline on
these logs (in UTC, with no history file) and compares each figure: dollar
figures within half a cent, counts exactly.

## What the logs cover

13 logs (9 main sessions, 4 sub-agents, one log with no counted reply), 68
turns over four calendar weeks (12 Oct to 5 Nov 2026):

- **Models and prices**: Opus 5.5, Opus 5, Opus 4.8, Fable 5.1, Fable 5,
  Sonnet 5.5, Sonnet 5, Haiku 5.5 (main session and sub-agent, with its
  long-prompt rates over 100k), Haiku 4.5 under a Bedrock id
  (`us.anthropic.claude-haiku-4-5-20251001-v1:0`); fast mode on Opus 5.5 and
  Opus 5 (doubled) and on Sonnet 5.5 (no fast price: unpriced); US-only
  inference on Fable 5.1 (surcharge) and Haiku 4.5 (none); a model the
  catalogue does not hold (unpriced).
- **Usage**: 5-minute and 1-hour cache writes; an older record with the
  write total and no split by lifetime; streamed replies whose first line
  repeats the message id with a partial count (the fullest line counts); a
  reply whose top-level usage is zero and whose steps (`iterations`) carry
  it; `<synthetic>` replies, one of them a "went to sleep" error, which are
  not turns.
- **Copied lines**: `alpha-3` resumes `alpha-2`, its log starting with a
  copy of `alpha-2`'s lines (replies and a compaction marker); they count
  once, in `alpha-2`.
- **Context**: turns over 200k on four sessions, one of them after a long
  pause that rebuilt the cache, where the fixed start (the
  `prompt_snapshot`) decides which removed tokens were read and which
  written; compaction markers with `compactMetadata` (automatic and manual,
  one in a sub-agent, one no reply followed).
- **Projects**: a subfolder (`alpha/web`), an unrelated folder and its
  subfolder (`gamma`, `gamma/tools`), a worktree under `.claude/worktrees`
  (counts as `beta`), a git worktree elsewhere (`epsilon-worktrees/feat`,
  its own project), two folders named `alpha` (`dev/alpha`, `work/alpha`),
  a sub-agent started in an unrelated folder that moves into its subfolder
  (`delta`, `delta/sub`), and a sub-agent whose main session's log is gone.
- **Reconciliation** (`cost-state` lines): sessions whose records cover
  them (one with two runs, each with running totals), one whose record was
  written part way, one whose only run began after its first turn, one with
  an unpriced turn, and conversations with no record.
- **Weeks**: a session that crosses from Sunday to Monday in UTC.

## Regenerating

From the repository root:

```sh
python3 -I scripts/gen-golden.py                       # rewrite projects/
python3 -I scripts/recount.py testdata/golden --json testdata/golden/expected.json
go test ./cmd/trimloop/ -run Golden -v
```

To compare the built CLI with the recount, figure by figure:

```sh
go build -o /tmp/trimloop ./cmd/trimloop
CLAUDE_CONFIG_DIR=testdata/golden HOME=$(mktemp -d) TZ=UTC \
  /tmp/trimloop --no-history --format json > /tmp/trimloop.json
python3 -I scripts/recount.py testdata/golden --compare /tmp/trimloop.json
```

The recount keeps its own copy of the price catalogue (`RATES` in
`scripts/recount.py`). When `pkg/price` changes, update that table and its
`RATES_VERSION`, then regenerate `expected.json`; the test stops with a
message until they agree.

When a figure differs, decide from `docs/measurements.md` (and the ADRs it
cites) which side is right. Fix the recount if it is wrong; if trimloop is
wrong, fix trimloop. Never regenerate `expected.json` to match trimloop.
