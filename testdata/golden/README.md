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
these logs (with no history file) and compares each figure: dollar figures
within $0.000001 (the two sides agree to about $0.000000001), counts and
names exactly. It compares what trimloop reports (the report's results,
such as P13's tables of main-session spend by model and sub-agent spend by
agent type with the model that cost each type most), not figures the test
works out itself; only token totals and spend per session, which the report
does not hold, are added up from the sessions it was given.

## Time zones

Weeks are calendar weeks in the report's time zone, and the CLI uses local
time, so the test runs the report in four zones (`UTC`,
`America/Los_Angeles`, `Asia/Kolkata`, `Pacific/Auckland`), one subtest
each, setting `report.Settings.Location` as the CLI does. `expected.json`
holds the figures that do not depend on the zone at the top level and each
zone's `weeks` and `period` under `zones` (the period counts whole 24-hour
spans, so it is the same in each zone). The recount works the zones out
together and stops if any other figure differs between them.

## Prices

The recount keeps its own copy of the price catalogue (`RATES`, `LONG`,
`FAST` and `NO_US_SURCHARGE` in `scripts/recount.py`) and writes the table
it used into `expected.json` (`rates`: each model's five rates, its
long-prompt tier, fast-mode and US-only multipliers). The test's `prices`
subtest compares it, model by model, with `pkg/price`'s catalogue
(`price.Catalogue`): a model in one and not the other, or any rate that
differs, fails with a message to update the recount and regenerate. The
other subtests do not run until the prices agree.

## What the logs cover

19 logs (13 main sessions, 6 sub-agents, one log with no counted reply), 90
turns over four calendar weeks (replies from 12 Oct to 5 Nov 2026, and a
prompt on 7 Nov):

- **Models and prices**: Opus 5.5, Opus 5, Opus 4.8, Fable 5.1, Fable 5,
  Sonnet 5.5, Sonnet 5, Haiku 5.5 (main session and sub-agent, with its
  long-prompt rates over 100k), Haiku 4.5 under a Bedrock id
  (`us.anthropic.claude-haiku-4-5-20251001-v1:0`) and as plain
  `claude-haiku-4-5`, in a main session (`eta-1`) and in sub-agents of one
  type (`Plan`), grouped as one model: counted id by id, Sonnet 5.5 would
  be the model that cost the Plan type most; fast mode on Opus 5.5 and
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
- **Context**: a turn of exactly 200,000 tokens, which is not over 200k;
  turns over 200k on four sessions, one of them after a long
  pause that rebuilt the cache, where the fixed start (the
  `prompt_snapshot`) decides which removed tokens were read and which
  written; compaction markers with `compactMetadata` (automatic and manual,
  one in a sub-agent, some no reply followed).
- **Projects**: a subfolder (`alpha/web`), an unrelated folder and its
  subfolder (`gamma`, `gamma/tools`), a worktree under `.claude/worktrees`
  (counts as `beta`), a git worktree elsewhere (`epsilon-worktrees/feat`,
  its own project), two folders named `alpha` (`dev/alpha`, `work/alpha`),
  a sub-agent started in an unrelated folder that moves into its subfolder
  (`delta`, `delta/sub`), a sub-agent whose main session's log is gone
  (`zeta`), and another (`theta`) that moves from its start folder to an
  unrelated one (`iota`) and then into its start folder's subfolder
  (`theta/sub`): with no main log, its own first folder stands in for the
  conversation's, so the subfolder's turn counts as `theta`.
- **Reconciliation** (`cost-state` lines): sessions whose records cover
  them (one with two runs, each with running totals), one whose record was
  written part way, one whose only run began after its first turn, one with
  an unpriced turn, and conversations with no record. Three compared
  sessions have a record over their logged cost, so the largest gaps list
  them (one with two sub-agent logs), and the records' tokens by model
  differ from the logs' in every class.
- **Weeks**: a session that crosses from Sunday to Monday in UTC, and
  sessions that fall in another week in Los Angeles, Kolkata or Auckland.
- **Compactions by week** (`kappa-*`): a compaction counts in the week of
  the reply it goes with, and one no reply followed in its own week. In
  Kolkata, a compaction just after Monday 00:00 whose triggering reply is
  logged after the marker but timed before it, on Sunday (it stays before
  the compaction), and whose next reply is just after it; in Auckland, a
  manual compaction just before Monday 00:00 whose next reply is just after
  (it counts in the new week), with an earlier compaction and no reply
  between the two (that one counts in its own week); in every zone, a
  compaction no reply followed eleven days after its session's last reply
  (its own week, not the last reply's).
- **Bad lines**: a line cut short in the middle of `kappa-1` and a JSON list
  in `kappa-2` (two bad lines, counted and skipped); a blank line, and a
  reply cut short at the end of `kappa-3` with no line ending (an
  unfinished last line), both skipped and not counted.
- **Period**: a prompt two days after the last reply, the last line of all
  the logs; the period runs from the first counted reply to the last.

## Regenerating

From the repository root:

```sh
python3 -I scripts/gen-golden.py                       # rewrite projects/
python3 -I scripts/recount.py testdata/golden --json testdata/golden/expected.json
go test ./cmd/trimloop/ -run Golden -v
```

`scripts/check.sh` checks that the committed files are what the scripts make
now: it writes the logs (`gen-golden.py --out`) and the answers into a
scratch folder and fails if any file here but this README differs, is
missing or is extra, or if either script fails. Without `python3` it skips
the step with a note, except in CI (`CI` set), where it fails. Edit the
scripts, not the files.

To compare the built CLI with the recount, figure by figure:

```sh
go build -o /tmp/trimloop ./cmd/trimloop
CLAUDE_CONFIG_DIR=testdata/golden HOME=$(mktemp -d) TZ=UTC \
  /tmp/trimloop --no-history --format json > /tmp/trimloop.json
python3 -I scripts/recount.py testdata/golden --tz UTC --compare /tmp/trimloop.json
```

`--compare` uses the first zone in `--tz`; run trimloop with the same `TZ`.

When a figure differs, decide from `docs/measurements.md` (and the ADRs it
cites) which side is right. Fix the recount if it is wrong; if trimloop is
wrong, fix trimloop. Never regenerate `expected.json` to match trimloop.
