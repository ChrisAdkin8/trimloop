# ADR 0048: Usage limits for subscribers

- Status: Accepted — built in the M0 prototype; amended by [ADR 0052](0052-third-review-corrections.md) (other wordings of a limit message)
- Date: 2026-11-08
- Amends: [ADR 0032](0032-readable-reports.md) (what reports lead with), [ADR 0033](0033-reconciliation-history-standing-context.md) and [ADR 0045](0045-m0-measurement-method.md) rule 10 (a new weekly figure), [ADR 0034](0034-live-nudges.md) (the status line keeps limit readings)

## Context

Many Claude Code users pay a flat monthly fee (Pro or Max) rather than per
token. For them "save $850" means little: their bill does not change. What
they feel is the usage limit: a 5-hour session limit and a weekly limit,
shared between Claude and Claude Code. Anthropic does not publish the size
of either in tokens.

What can be read on the user's machine (checked 2026-11-08):

- **The plan.** Claude Code's account file (`.claude.json`) holds
  `oauthAccount.organizationType` (for example `claude_max`), `billingType`
  (`stripe_subscription`) and a rate-limit tier
  (`default_claude_max_5x`).
- **Limit hits.** When a limit is reached, the log gets a message Claude
  Code wrote itself: an assistant line with `error: "rate_limit"`,
  `isApiErrorMessage: true`, `apiErrorStatus: 429` and text such as
  "You've hit your session limit · resets …" or "… your weekly limit …".
  The founder's logs hold two in 44 days.
- **How much of each limit is used.** Claude Code passes the status line
  `rate_limits.five_hour` and `rate_limits.seven_day`, each with
  `used_percentage` (0–100) and `resets_at` (Unix seconds), for Pro and Max
  subscribers ([status line docs](https://code.claude.com/docs/en/statusline)).
  Nothing else exposes them, and Claude Code does not keep them.

## Decision

1. **The plan** is read from the account file in Claude Code's folder
   (`$CLAUDE_CONFIG_DIR/.claude.json`, else `~/.claude.json`): only
   `organizationType`, `billingType` and the two rate-limit tiers. It is
   named in local reports ("Max 5×") and never stored.
2. **A user is treated as a subscriber** when the billing type is a
   subscription, or when there are limit readings or limit hits.
3. **Limit hits** are read from the logs, each with its time and which
   limit ("session" for the 5-hour limit, "weekly", or "other"), counted
   once by line id. Each week in the history file keeps `limit_hits`
   (analysis version 7).
4. **Limit readings.** The status line appends a line to
   `~/.trimloop/limits.jsonl` (one `limits-<id>.jsonl` per other
   `CLAUDE_CONFIG_DIR`, named like the history file) when either limit's
   used percentage or reset time changes: the time, and each limit's used
   percentage and reset time. Numbers only.
5. **The exchange rate** from list-price spend to limit: within each weekly
   window (readings with the same reset time), the rise in used percentage
   from its first reading to its last, against the list-price spend the
   logs record between those two times; summed over all windows. It is
   shown only once the windows together cover at least $20 of spend and a
   5-point rise. The 5-hour limit is worked out the same way and shown
   beside it.
6. **How reports change for subscribers.** The summary first says the
   figures are list prices, a measure of how much of the limits work takes,
   not money the user would get back. It says how often a limit was hit.
   With an exchange rate, each action also shows its saving as a share of a
   week's limit: the saving per week times the rate, capped at 100 %. JSON
   carries the plan, the hits, the readings counted and the rates. Dollar
   figures stay everywhere, for comparison and for API users.

## Consequences

- The rate overstates what trimloop's actions save, because the limits are
  shared: use of Claude chat, or Claude Code on another machine, raises the
  percentage with no spend in these logs. Reports say "about" and explain
  why; a later ADR may correct for it.
- The share of the limit is unknown until the status line has run through
  some of a week; until then reports show limit hits only.
- A new stored file, holding numbers only, like the history file
  (ADR 0004, ADR 0032).
- Weeks stored before analysis version 7 have no `limit_hits` and are
  marked as older, as for earlier version changes.
