# State machines in the prototype

- Status: current for the M0 prototype (2026-11-08)
- Read with: [measurements.md](measurements.md) (what each figure means),
  [ADR 0045](adr/0045-m0-measurement-method.md) (the method in force)

Much of trimloop walks a sequence (log lines, turns, status-line refreshes,
hook calls, runs of the program) and remembers something between steps.
This page lists every such place: what it remembers, the states, what moves
it between them, where the code and tests are, and the edges known and left
alone. Before changing one, read its entry; after changing one, update it.

**Explicit** means a named type with its transitions in one place.
**Implicit** means the state lives in loose variables or file contents;
each such entry says why it was left that way.

## Summary

| # | State machine | Where | Kind | Persisted |
|---|---|---|---|---|
| 1 | What the next turn inherits | `cmd/trimloop/between.go` | Explicit | No |
| 2 | Building a session from a log | `cmd/trimloop/main.go` `buildSession` | Implicit, small | No |
| 3 | Trimloop's entries in Claude Code's settings | `internal/live/setup.go` `EntryState` | Explicit | Claude Code's settings file |
| 4 | Size and pause nudges per session | `internal/live/state.go` `afterReply`, `beforeSend` | Explicit | `~/.trimloop/live/<session>.json` |
| 5 | Usage-limit readings | `internal/history/limits.go`, `pkg/detectors/limits.go` | Explicit rule | `~/.trimloop/limits.jsonl` |
| 6 | Weekly history | `internal/history/history.go` `decide`, `weekOutcome`; `internal/report` `olderReason` | Explicit | `~/.trimloop/history.json` |
| 7 | Status-line phase | `internal/adapters/source/claudecode/tail.go`, `internal/live/run.go` | Implicit, small | No |
| 8 | Log reading offset | `internal/adapters/source/jsonl` | Implicit | Not yet |
| 9 | Detectors that walk a session's turns | `pkg/detectors`; the shared rule `event.Usage.WriteLife` | Implicit, one shared rule | No |
| 10 | Which card text applies | `pkg/fixes` | Decision tables | No |

## 1. What the next turn inherits (explicit)

Between two new turns of a log, Claude Code writes text it adds to the
conversation, the user's messages, and sometimes a compaction marker. The
next turn inherits them. `between` holds them in two phases:

| Phase | Meaning |
|---|---|
| open | no compaction since the last turn; added text and prompts collect for the next turn |
| compacted | a compaction marker was seen; what came before it is held apart from what follows |

| Event | Transition |
|---|---|
| text added, user acted (`inject`, `prompt`) | either phase: collected after the marker (or since the last turn) |
| compaction marker (`compact`) | open → compacted; compacted → compacted, and the earlier compaction is kept as never replied to |
| a turn timed before the pending marker (`take`, late trigger) | compacted → compacted: this reply triggered the compaction and takes only what came before it |
| any other turn (`take`) | either phase → open: the turn takes everything, and the compaction |
| end of log (`end`) | a compaction still pending was never replied to |

Tests: `cmd/trimloop/between_test.go`, and the `TestBuildSession*` tests.
The rule is ADR 0045 rule 5.

## 2. Building a session from a log (implicit, small)

`buildSession` also remembers, across a log's lines: the latest activity
time (for idle time before a turn); message ids counted here (`turnOf`) and
tool calls waiting for results (`callAt`); whether the user named the
session (a generated title then never replaces it); the latest system
prompt and tool list; and the harness's latest cost record per run (records
are running totals, so the largest wins). Each is one variable with one
rule, so they stay as they are. Lines copied into a resumed session are
counted once, in the oldest log, through the shared `seen` set (ADR 0026).

## 3. Trimloop's entries in Claude Code's settings (explicit)

Each entry (the status line, the `Stop` hook, the `UserPromptSubmit` hook)
is in one `EntryState`:

| State | Meaning | `setup --write` | `setup --check` |
|---|---|---|---|
| `Missing` | not there | adds it | "missing" |
| `Current` | runs this trimloop (however its path is quoted), with today's timeout | leaves it | "up to date" |
| `Stale` | runs a program named `trimloop` from another path, or with an old timeout | updates it | "out of date" |
| `Other` | a status line that is not trimloop's | refuses, unless `--replace-statusline` | "another program's" |

A command is trimloop's only when its program is named exactly `trimloop`;
`trimloop-wrapper` is someone else's. `setup --check` exits 0 when all three
are `Current`, 1 otherwise, and also says whether usage-limit readings are
arriving. Tests: `TestIsTrimloop`, `TestCheckSettingsStates`,
`TestWriteSettings*`, `TestCheckSetup`.

Known edges, left alone: replacing a status line drops its other keys (for
example `padding`); duplicate trimloop hooks are updated, not removed; there
is no `--remove`; each changing write leaves a backup file.

## 4. Size and pause nudges per session (explicit)

Stored per session in `~/.trimloop/live/<session>.json` (ADR 0034). The
state changes only through two methods on `State`, which `Hook` calls:
`afterReply` (hook stop) and `beforeSend` (hook prompt).

- **Size nudges.** `Crossed` is the set of size thresholds already
  announced. After each reply (`hook stop`): thresholds the session is now
  under are re-armed (it was compacted or cleared), and if any were while it
  is still past the large threshold, that one stays announced ("not news");
  then the highest threshold passed and not yet announced is shown.
- **Pause before sending.** `PausedFor` is the reply after which the pause
  was shown, so it shows once per long break (`hook prompt`).

It could be one "highest level announced" value rather than a set, but the
set is stored in users' files; changing it needs a migration for no change
in behaviour, so it stays. Tests: `TestStateAfterReply` (a session growing,
compacted and growing again), `TestStateBeforeSendPausesOncePerBreak`, and
the `TestHook*` tests. Known edges: a stop hook and a prompt hook
running at the same moment can each lose the other's update (a nudge may
repeat); a damaged file starts fresh; files untouched for 30 days are
removed, so a session resumed after a month can see its nudges again.

## 5. Usage-limit readings (explicit rule)

The status line appends a reading to `limits.jsonl` only when it moves a
limit forward from the last line kept (`ahead`): the first reading, a later
window, or a higher use in the same window. A session that has been idle
can repeat an older reading, and every open session refreshes every 30
seconds; anything not ahead is dropped, so the file grows by at most one
line per percentage point per window. Two reset times within a minute of
each other are the same window (`SameWindow`), both when keeping readings
and when measuring the rate (ADR 0048).

Tests: `TestAppendAndLoadLimits`, `TestLoadLimitsSurvivesALongDamagedLine`,
`TestFindLimits*`, `TestStatusLineKeepsLimitReadings`. Known edge: two
status lines writing at the same instant can both append; the measure uses
each window's first and last reading, so a duplicate changes nothing.

## 6. Weekly history (explicit)

Each run merges its weeks into the history file (ADR 0045 rule 10). `decide`
gives each week one `weekOutcome`:

| Outcome | When | Result | Shown as |
|---|---|---|---|
| `fromLogs` | the logs hold the week, and no fuller stored copy wins | this run's figures replace the stored ones | — |
| `keptFuller` | the week is at or before the week of the oldest log (Claude Code may have deleted part of it), and the stored copy counted more turns the same way (analysis ≥ `TurnsCountedSince`) | the stored copy stays | "kept: logs partly deleted, stored copy fuller" |
| `logsGone` | the logs no longer hold the week | the stored copy is shown | "kept: logs deleted" |

Tests: `TestMergeSaysWhyAWeekWasKept` and the other `TestMerge*` tests.

A week worked out by an older analysis or another price list is marked
with the reason (`olderReason`): "older method: analysis 4, now 7", "older
prices: <catalogue>", or both. Figures first measured in a later version
show a dash for older weeks (`Week.HasNewFindings`, `Week.HasLimitHits`).
A `--since` run neither reads nor writes the file.

Known edges: deleting logs by hand makes a later week look complete, so a
fuller stored copy is replaced; two runs at once can overwrite each other's
merge. Neither happens in normal use.

## 7. Status-line phase (implicit)

`ReadTail` scans back from the end of a log: to the latest reply; if that
reply wrote nothing to the cache, on to the latest one that did (its
lifetime sets the countdown); and it notes a compaction logged after the
latest reply. The status line is then in one of three phases: no reply yet
(cost only), replied (size, cost, countdown), or compacted (size unknown
until the next reply: "compacted"). A compaction logged before the latest
reply counts as after it only when both have times and the compaction's is
later (the triggering reply can be logged late). The scan's two flags are
local to one function and read clearly, so they stay. Known edge: a final
line over 1 MB shows the cost only.

## 8. Log reading offset (implicit; not saved yet)

`jsonl.Lines` moves `Log.Offset` past each complete line, stops before a
final line with no newline, and starts again from 0 if the file shrank.
Offsets are not saved between runs today. Before they are (ROADMAP), make
the offset a cursor that also records the file's identity and size, so a
replaced or rotated file is noticed.

## 9. Detectors that walk a session's turns (implicit)

| Detector | What it remembers across turns | Note |
|---|---|---|
| P02 short waits (`cache_life.go`) | the session's cache lifetime from its latest write; whether it ever wrote for 5 minutes or an hour; from its first hour write on (for the hour-against-5-minutes range) | Three questions on purpose, all from one rule, `Usage.WriteLife` (the hour if any write was for the hour, else 5 minutes, else none), which the status line's countdown also uses. "Wholly on the 5-minute cache" means it never wrote for an hour, by design (ADR 0045 rule 7) |
| P09 long breaks, weekly `userBreak` (`long_pause.go`) | nothing but the turn's idle time and who ended it | P02 counts waits of 5 minutes to an hour; P09 waits longer than its gap setting, an hour by default, and the card names the gap it used |
| Compaction replay (`likely.go`) | how much a simulated compaction has removed so far | Reset by a real compaction, and by a fall of more than half with no marker (`TestLikelyCompactionResetsOnAnUnmarkedDrop`) |
| Carried tool output (`tool_output.go` `carry`) and carried text (`standing.go`) | what is still in the session, cleared by a compaction; files viewed since their last change | Two implementations of one idea (ADR 0045 rule 4). On a compaction turn, tool output from before is dropped, while text arriving on that turn is new and priced as such. Merging them is possible, but each is covered by its own tests and the merge would be all risk for no change in figures |
| Weekly figures (`weekly.go`) | the week of each turn | |
| Reconciliation (`reconcile.go`) | per main session: recorded, unpriced, behind, or compared | Decided by the order of its checks |
| Usage limits (`limits.go`) | per limit window: its first and last reading | See §5 |
| Model switches (`switches.go`) | the lifetime of the session's latest cache write, as P02 does | A switch counts only while that cache was alive |
| Tool inputs (`tool_output.go` `FindToolInputs`) | the tool-call inputs still in the session, cleared by a compaction | Priced like carried tool output |

Sums over maps are taken in a fixed order (sorted keys), so a report is
the same on every run.

## 10. Which card text applies (decision tables)

Several cards in `pkg/fixes` choose their text from counts:

- **P02** (`ForCacheLife`): main sessions keep the hour (move slow waits
  there); only sub-agents keep it (do not let a main session wait); nobody
  does (run slow commands in the background).
- **P13 sub-agents** (`ForSubAgents`): names the cheaper model and the
  front-matter name for it (`model: haiku`, `model: sonnet`); "a cheaper
  model" when none is named.
- **P14** (`ForEffort`): back to the usual level, or to high.
- **P09** (`ForPauses`): by how many breaks were the user's.

Each case has a test in `pkg/fixes`.

## Session kinds

A session is `event.MainSession` or `event.SubAgentSession`
(`pkg/event/turn.go`); every session read from the logs is one of the two.
