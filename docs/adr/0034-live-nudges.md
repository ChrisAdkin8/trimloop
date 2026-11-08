# ADR 0034: Live nudges in Claude Code

- Status: Accepted — built in the M0 prototype; amended by [ADR 0036](0036-prices-reconciliation-status.md) (countdown)
- Date: 2026-11-08
- Amends: [ROADMAP.md](../../ROADMAP.md) (M0 scope), [packaging.md §3](../packaging.md#3-the-trimloop-binary) (commands)

## Context

The two largest savings in the founder's logs are habits: compacting long
sessions earlier (likely about $820 over 44 days; corrected on 2026-11-08 to
about $460, see [log-signals.md §8](../log-signals.md#8-measured-figures-at-a-glance)) and starting fresh after
a long break (about $340). A report read days later does not change a habit
at the moment it matters. Claude Code can run a command for its status line
and commands at hook events, passing each a small JSON document that names
the session's log.

Eleven candidate nudges were measured on the founder's logs (how often each
would fire, money spent after it fires, and who could act on it). Three
findings shaped the design:

- **Timing.** After a break of over an hour in a session over 200k tokens,
  almost all the extra cost is on the first message back ($314, against $45
  over the next four turns). A message shown as the user sends is too late;
  the warning must be visible before, or the message held.
- **Audience.** The money is in the user's habits. Nudging Claude about its
  own tool use is worth little here and costs tokens.
- **Noise.** Raw triggers can fire far too often (392 folder changes, 18 of
  them real project changes; sub-agent starts 25 times a week), so each
  nudge needs a rule that fires rarely.

## Decision

Trimloop gives five nudges (on the status line: size, effort, and the
cache countdown with the next message's cost; after a reply: compact; before
sending: the hold after a break) through three commands Claude Code runs:

1. **`trimloop statusline`**, always visible: the conversation's size in
   colour (green, amber over 200k, red over 500k), the session's cost as
   Claude Code reports it, the effort level when it is `max` or `xhigh`,
   the cache countdown in its last 10 minutes, and once the cache has
   expired, "next message ≈ $X". Claude Code refreshes it every 30 seconds
   so the countdown moves.
2. **`trimloop hook stop`**, after a reply: the first time a session passes
   200k tokens, a suggestion to compact at the next natural break; the first
   time it passes 500k, a stronger one with how many times a fresh session
   it now sends. Shown with the hook output's `systemMessage`.
3. **`trimloop hook prompt`**, when the user sends: if the cache has expired
   and the session is over 500k tokens, the message is held once per break
   (`"decision": "block"` with a reason giving the time away, the size and
   the estimated cost); sending again goes through.

Rules every nudge follows:

- **Fast and harmless.** They read only the end of the current log (at most
  1 MB), finish in milliseconds, and on any problem show nothing; Claude
  Code always carries on.
- **No tokens.** Nothing is printed as plain output on events where Claude
  Code adds it to the conversation. Whether `systemMessage` also reaches the
  model is not documented, so it is to be checked on each Claude Code
  version.
- **Once only.** Each after-reply nudge appears once per session, and again
  only after the session has shrunk below 200k (compacted or cleared) and
  grown past the threshold again; the hold appears once per break. Tracked in `~/.trimloop/live/<session>.json`, which holds
  thresholds and times only; files untouched for 30 days are removed.
- **Measured.** Each shown nudge is appended to `~/.trimloop/nudges.jsonl`
  (time, nudge, context size), so the report can later show what followed.
- **Opt-in setup.** `trimloop setup` prints the settings to add;
  `--write FILE` adds them to a Claude Code settings file after a backup,
  keeps everything else, adds no hook twice, and does not replace another
  status line unless `--replace-statusline` is given. A plugin cannot set
  the status line, so a plugin would still need this step.

Not built: nudges on sub-agent starts (better fixed once in setup with a
cheaper agent), on large tool output and re-reads (Claude's habits, small
sums, and costing tokens), on resuming a large session (not seen in the
logs), cost milestones (the status line shows cost), and project switches
(worth it, after a reliable rule).

## Consequences

- At the founder's rate, the hooks would show about one or two messages a
  day of work; the status line is always there.
- The commands need a built `trimloop` binary at a stable path; `setup`
  refuses to run from `go run`.
- Thresholds are fixed in this version (200k, 500k, 10 minutes); making them
  settings, and reporting nudges' effect from `nudges.jsonl`, are next.
