# Concepts deck

A 16-slide introduction to trimloop: why tokens get expensive, what the
prototype does today, how its figures are checked, and the plan.

The files here are the deck's source, not a file you can open and present:

- `project/deck.json`: the slide order, the four sections and the fonts.
- `project/slides/<name>.html`: one slide each, laid out on a 1920×1080
  canvas. The `<aside>` at the end of each file holds the speaker notes.

They are in the format of Claude's Slides artifact (a deck viewer on
claude.ai). Icons, arrows and connectors are that viewer's own elements
(`x-icon`, `x-shape`, `x-connector`), so a slide file opened on its own in
a browser shows its text but not those.

## The slides

| # | File | Title |
|---|---|---|
| 1 | `cover.html` | Trimloop |
| 2 | `problem.html` | Every turn re-sends the whole conversation |
| 3 | `cache.html` | Cached history is cheap, until it expires |
| 4 | `pipeline.html` | Trimloop turns your logs into advice |
| 5 | `patterns.html` | It looks for nine kinds of waste |
| 6 | `example.html` | A real run shows what to fix first |
| 7 | `trust.html` | Treat the figures as estimates |
| 8 | `live.html` | Nudges warn you while you work |
| 9 | `measure.html` | Change a habit, then measure again |
| 10 | `privacy.html` | Everything stays on your machine |
| 11 | `code.html` | One program, one path through it |
| 12 | `checks.html` | Two programs must agree on the figures |
| 13 | `plan.html` | Next: it makes the safe changes itself |
| 14 | `safety.html` | Each kind of change has a safety level |
| 15 | `roadmap.html` | From prototype to automatic fixes |
| 16 | `close.html` | Detect. Recommend. Measure. Repeat. |

Slides 2–3 are the background, 4–10 the prototype, 11–12 the code and its
checks, and 13–15 the plan, which is design only.

## Viewing and presenting it

In Claude Code, ask it to publish `docs/deck` as a Slides artifact. The
viewer presents the deck and exports it as a PDF or a PowerPoint file.

## Keeping it in step

The deck restates facts that live elsewhere. When one of these changes,
change the slide too:

| Slide | Fact | Where it comes from |
|---|---|---|
| 5 | The nine findings and their codes | `README.md`, "What it looks for" |
| 6, 7 | The sample run's figures (spend, savings, the 11% gap) | `README.md`, "What you get" |
| 8 | The 200,000 and 500,000 token marks | `README.md`, "Live nudges in Claude Code" |
| 10 | The files trimloop writes | `README.md`, "Files it writes" |
| 11 | Package and function names | `cmd/`, `internal/`, `pkg/` |
| 12 | The number of decision records (53) | `docs/adr/` |
| 13–15 | The plan, the safety levels and the milestones | `README.md`, "The plan"; `docs/design.md`; `ROADMAP.md` |
| 16 | The build and run commands | `README.md`, "Install" and "Use it" |

Rules the slides follow: every title fits on one line, and each term is
explained on the slide where it first appears.
