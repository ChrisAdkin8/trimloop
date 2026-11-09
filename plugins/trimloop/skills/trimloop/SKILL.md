---
name: trimloop
description: Explain where the user's Claude Code tokens and money went, and which changes to how they work would save the most, by running the trimloop program on their local Claude Code logs. Use when the user asks why Claude Code is expensive, where their tokens or usage went, why they keep hitting usage limits, how to cut cost, whether a change they made worked, or to set up trimloop's status line and nudges.
when_to_use: >-
  Questions such as: where did my tokens go; why is Claude Code so expensive;
  I keep hitting my usage limit; how do I spend less; did compacting earlier
  help; set up trimloop; check my trimloop setup.
argument-hint: "[since DATE or 7d]"
license: FSL-1.1-ALv2
---

# trimloop

trimloop reads the session logs Claude Code keeps on this machine
(`~/.claude/projects`, or `$CLAUDE_CONFIG_DIR/projects`), works out where
the tokens went, and recommends changes ranked by money saved. It runs
locally and sends nothing anywhere. This skill runs it and explains the
result.

Speak plainly: short sentences, no jargon, and spell out a term the first
time (for example "the cache: a saved copy of the conversation that makes
re-sending it cheaper").

## 1. Make sure trimloop is installed

Run `trimloop --version`. If the command is not found, stop and tell the
user how to install it; do not install it yourself without their yes:

- macOS or Linux with Homebrew: `brew install --cask chrisadkin8/tap/trimloop`
- otherwise: download the archive for their system from
  https://github.com/ChrisAdkin8/trimloop/releases and put `trimloop` on
  their PATH.

## 2. Run the analysis

Write the full figures to a private temporary folder and read them; the
terminal summary alone leaves out the detail you need. Each command runs in
a fresh shell, so note the folder's path and use it literally afterwards:

```bash
mktemp -d                       # prints the folder, e.g. /tmp/tmp.Ab12Cd
trimloop --format json --out /tmp/tmp.Ab12Cd/report.json
```

The second command also prints a short summary. If the user named a date or a
span ("since Monday", "last 2 weeks", or an argument such as `7d` or
`2026-11-01`), add `--since DATE` or `--since 7d`/`2w`: the report then
covers only sessions started from then on, which is how to tell whether a
change worked.

If it prints "No Claude Code session logs found", say so, and mention that
`CLAUDE_CONFIG_DIR` must point at the folder holding `projects` if their
logs are elsewhere.

## 3. Explain the result

Read these fields from the JSON:

| Field | Meaning |
|---|---|
| `total`, `period.days`, `period.per_week` | spend at list prices, over how many days, per week |
| `actions` | recommendations, largest "up to" saving first |
| `actions[].title`, `.finding` | the change, and what was found |
| `actions[].saving` | "up to": the most the change could save |
| `actions[].likely`, `.likely_low`, `.likely_high` | a likelier saving, replayed on their logs (0 if not worked out) |
| `actions[].weekly_limit_share` | for Pro and Max: the saving as a percentage of a week's usage limit, once measured |
| `actions[].primary.what`, `.how` | the fix, and exactly how to make it in Claude Code |
| `actions[].alternatives` | other ways to make the same saving |
| `actions[].check` | how to tell afterwards whether it worked |
| `plan`, `usage_limits` | their plan, and how often a usage limit stopped them |
| `reconciliation` | how far Claude Code's own cost records exceed the logs |

Explain the top three actions: what the habit is, what it cost, the fix in
one or two steps, and the likely saving next to the "up to" figure. Then
offer the rest briefly.

Always say, in plain words:

- The figures are estimates at Anthropic's list prices, not their bill.
- The savings overlap (the same tokens can be saved by more than one
  change), so they must not be added up.
- A change marked untested for quality (a cheaper model, lower effort)
  should be checked after it is made.
- **On a Pro or Max plan** (`plan.subscription` is true, or
  `usage_limits.hits` is above 0): they pay a flat fee, so dollars are a
  measure of how much of their usage limits work takes, not money back.
  Lead with `usage_limits.hits` and `weekly_limit_share` where present.

Offer the full web page with `trimloop --open` (it writes
`trimloop-report.html` in the current folder and opens it).

## 4. Privacy

Reports show session titles, which summarise what the user asked, and
project names. Do not paste a report into anything shared without the
user's yes; `--no-titles` leaves the titles out. Delete the temporary
folder when you are done (`rm -r /tmp/tmp.Ab12Cd`, with its real path).

## 5. Live nudges (optional)

trimloop can also show a status line in Claude Code (session size, cost,
how long the cache has left, and usage-limit readings for Pro and Max)
and nudge when a session gets large. Check with:

```bash
trimloop setup --check
```

If anything is "missing" or "out of date", show the user what
`trimloop setup` prints, and run `trimloop setup --write
~/.claude/settings.json` only if they say yes: it edits Claude Code's
settings (after making a backup). If their status line is "another
program's", tell them that `--replace-statusline` would replace it; never
add that flag on your own. Changes take effect in new Claude Code sessions.

## Commands at a glance

| Command | Does |
|---|---|
| `trimloop` | short summary |
| `trimloop --format json --out FILE` | every figure, for reading |
| `trimloop --open` | the web page, opened in the browser |
| `trimloop --since 7d` | only sessions started in the last 7 days |
| `trimloop setup --check` | is the status line and nudge setup working? |
| `trimloop --version` | which build is installed |
