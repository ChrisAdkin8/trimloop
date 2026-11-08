package fixes

import (
	"fmt"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// ForCacheLife recommends keeping long waits out of sub-agents (P02, ADR
// 0042), or returns false if they would save less than minSaving.
func ForCacheLife(f detectors.CacheLife, minSaving float64) (Recommendation, bool) {
	if f.Saving < minSaving {
		return Recommendation{}, false
	}
	return Recommendation{
		Pattern: "P02",
		Title:   "Keep long waits out of sub-agents",
		Finding: fmt.Sprintf("%d times (%d in sub-agents), a session whose cache lasts 5 minutes waited longer, up to an "+
			"hour, so its next turn paid to save its whole history again. Claude Code's sub-agents use the 5-minute "+
			"cache; main sessions keep theirs for an hour.", f.Waits, f.SubAgent),
		Saving:  f.Saving,
		Overlap: "Separate from the long-break fix, which covers waits over an hour",
		Primary: Fix{Structure,
			"Run long builds, test suites and other slow commands in the main session, not in sub-agents.",
			"The main session keeps its cache for an hour; a sub-agent's lasts 5 minutes, so a sub-agent that waits " +
				"on a slow command, or on its own background task, saves its whole history again when it carries on."},
		Alternatives: []Fix{
			{Advise,
				"Tell Claude not to give sub-agents work that waits on slow commands.",
				`Add to CLAUDE.md: "Run slow builds and test suites yourself, in the background if they take minutes; ` +
					`don't hand them to a sub-agent."`},
		},
	}, true
}

// ForMainModels recommends a newer model in the same line, which costs no
// more for the same work, and, as a trade-off, a tier down for routine main-
// session work (P13, ADR 0042). Either is left out below minSaving.
func ForMainModels(f detectors.MainModels, minSaving float64) []Recommendation {
	var out []Recommendation
	if f.Newer >= minSaving {
		var from, moves []string
		for _, g := range f.ByModel {
			if g.NewerSave > 0 {
				from = append(from, price.DisplayName(g.Model))
				moves = append(moves, "from "+price.DisplayName(g.Model)+" to "+price.DisplayName(g.Newer))
			}
		}
		out = append(out, Recommendation{
			Pattern: "P13",
			Title:   "Move to the newer model",
			Finding: fmt.Sprintf("%d main-session turns ran on %s. Each has a newer version that costs less per "+
				"token and counts tokens the same way.", f.NewerTurns, joinNames(from)),
			Saving:   f.Newer,
			Examples: from,
			Primary: Fix{Advise,
				"Switch main sessions " + joinNames(moves) + ".",
				`Choose the model with /model, or set "model" in .claude/settings.json.`},
			Check: "Prompts tuned for the older model may need adjusting: compare turns per task for a week after switching.",
		})
	}
	if f.LowerTier >= minSaving {
		var from, to []string
		for _, g := range f.ByModel {
			if g.TierSave > 0 {
				from = append(from, price.DisplayName(g.Model))
				to = append(to, price.DisplayName(g.Tier))
			}
		}
		out = append(out, Recommendation{
			Pattern: "P13",
			Title:   fmt.Sprintf("Use %s for routine main-session work", joinNames(unique(to))),
			Finding: fmt.Sprintf("%d main-session turns ran on %s. The same turns on %s would have cost less; whether "+
				"it does the work as well is not known from the logs.", f.TierTurns, joinNames(from), joinNames(unique(to))),
			Saving:   f.LowerTier,
			Examples: from,
			Primary: Fix{Advise,
				fmt.Sprintf("Start sessions on %s and switch to %s for the hardest problems.", joinNames(unique(to)), joinNames(from)),
				`Choose the model with /model at the start of a task, or set "model" in .claude/settings.json.`},
			Check: "Untested for quality: after a week, compare turns per task and tool errors with before, and switch back if they rose.",
		})
	}
	return out
}

// ForEffort recommends high effort as the default (P14, ADR 0042), or
// returns false below minSaving.
func ForEffort(f detectors.Effort, minSaving float64) (Recommendation, bool) {
	if f.Saving < minSaving {
		return Recommendation{}, false
	}
	return Recommendation{
		Pattern: "P14",
		Title:   "Save the highest effort for hard problems",
		Finding: fmt.Sprintf("%d turns ran at the highest effort levels (xhigh or max). Their output beyond a typical "+
			"high-effort turn (%s tokens) cost %s. Harder work gets higher effort on purpose, so this is an upper bound.",
			f.Turns, thousandsInt(f.HighMean), dollars(f.Saving)),
		Saving: f.Saving,
		Primary: Fix{Advise,
			"Use xhigh and max effort only for hard problems; high is enough for most work.",
			"Set the effort level in Claude Code's model settings (/model), and raise it for a task that needs it."},
		Check: "Untested for quality: after a week, compare turns per task and tool errors with before.",
	}, true
}

func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	out := names[0]
	for _, n := range names[1 : len(names)-1] {
		out += ", " + n
	}
	return out + " and " + names[len(names)-1]
}

func unique(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
