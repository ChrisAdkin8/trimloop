# ADR 0053: Projects by conversation, a card that saves nothing ranked last, and analysis version 8

- Status: Accepted — built
- Date: 2026-11-09
- Amends: [ADR 0031](0031-m0-order-from-measured-spend.md) (decision 3, attribution by project), [ADR 0045](0045-m0-measurement-method.md) (decision 3, ranking; rule 10, the analysis version)

## Context

Three commits on 2026-11-09 (7866bd6, 07e9341, 7b5606d) fixed the
findings of three reviews. Their messages called every change a correction
that keeps its decision. A fourth review found that two of them change a
decision on record, and that one should have raised the analysis version:

- **Projects.** ADR 0031 decision 3 credits each turn to "the working
  folder's name", and a sub-agent takes its main session's project only
  when it has none. Claude moves between folders (`cd`) during a session:
  on the founder's logs 28 of 368 main sessions changed project part way,
  sub-agents started in a subfolder became projects of their own, and two
  repositories with the same folder name were merged. The commits credit
  turns by conversation instead, and name projects so that same-named
  folders stay apart.
- **Ranking.** ADR 0045 decision 3 ranks actions by their upper bound.
  Once a likely figure of 0 meant "worked out, and it saves nothing"
  (07e9341), such a card could still be ranked first on its upper bound.
  7b5606d ranks it last.
- **Re-reads.** 7866bd6 changed what counts as a re-read (the same part of
  an unchanged file), which changes the weekly `rereads` figure: 1,090
  became 113 on the founder's logs. ADR 0045 rule 10 says any change to how
  weekly figures are worked out raises the analysis version; it stayed at 7.

## Decision

1. **A turn's project is its conversation's** (amends ADR 0031 decision 3):
   the folder the main session started in, a worktree under
   `.claude/worktrees/` counting as its project, when the turn ran there,
   in a subfolder or in a folder above it, whether the main session or a
   sub-agent ran it; otherwise the folder the turn ran in. Other git
   worktrees count as their own folder: the logs mark a folder as a
   worktree but do not name its repository.
2. **Projects are named by their folder's name**, with as many parent
   folders' names as tell two same-named folders apart (`work/api`,
   `home/api`); a folder with no parent left keeps its own name. Reports
   still show names, not paths (ADR 0032), but a parent's name can show
   (for example the home folder's), and a name depends on which folders the
   run includes, so a `--since` run can name a project differently.
3. **A card whose likely saving was worked out and is nothing is ranked
   after every other**, and says why in its own words (amends ADR 0045
   decision 3); the rest are still ranked by their upper bound.
4. **Analysis version 8** (amends ADR 0045 rule 10): re-reads count only
   views of the same part of a file not changed since. Weeks stored by an
   earlier version are shown as worked out by an older method.

## Consequences

- On the founder's logs: `slop-or-not` $242 → $490 and `github.com` $76 →
  $122, matching an independent recount of the logs; re-reads 1,090 → 113.
- A stored history shows past weeks as "older method" until a run covers
  them again; partly deleted weeks keep their stored copy, and its note.
- How each figure is worked out is in
  [measurements.md](../measurements.md), the figures in
  [log-signals.md §8](../log-signals.md#8-measured-figures-at-a-glance).
- A change like these needs its own record when it is made, not after a
  review finds it.
