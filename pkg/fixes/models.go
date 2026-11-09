package fixes

import (
	"fmt"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// ForCacheLife recommends keeping long waits in sessions whose cache lasts
// an hour (P02, ADRs 0042, 0044), or returns false below minSaving. Which
// kind of session keeps the hour is read from the logs, not assumed: in
// Claude Code main sessions usually do, but SDK runs may not.
func ForCacheLife(f detectors.CacheLife, minSaving float64) (Recommendation, bool) {
	if f.Saving < minSaving {
		return Recommendation{}, false
	}
	where := fmt.Sprintf("In these logs %s and %s used the 5-minute cache; %s and %s kept theirs for an hour.",
		plural(f.MainShort, "main session"), plural(f.SubShort, "sub-agent"),
		plural(f.MainHour, "main session"), plural(f.SubHour, "sub-agent"))
	r := Recommendation{
		Pattern: "P02",
		Title:   "Keep long waits out of sessions with a 5-minute cache",
		Finding: fmt.Sprintf("%d times (%d in sub-agents), a session whose cache lasts 5 minutes waited longer, up to an "+
			"hour, so its next turn paid to save its whole history again. %s", f.Waits, f.SubAgent, where),
		Saving:  f.Saving,
		Overlap: "Separate from the long-break fix (waits over an hour); shares sub-agent turns with the cheaper-model fix: do not add them",
		Primary: Fix{Structure,
			"Run long builds, test suites and other slow commands in a session that keeps its cache for an hour.",
			"A session on the 5-minute cache that waits on a slow command, or on its own background task, saves its " +
				"whole history again when it carries on. In Claude Code that is usually the main session rather " +
				"than a sub-agent; sessions run through the SDK may use the 5-minute cache too."},
		Alternatives: []Fix{
			{Advise,
				"Tell Claude not to give sub-agents work that waits on slow commands.",
				`Add to CLAUDE.md: "Run slow builds and test suites yourself, in the background if they take minutes; ` +
					`don't hand them to a sub-agent."`},
		},
		Check: "After a week, the weekly \"short waits\" figure should fall.",
	}
	switch {
	case f.MainHour == 0 && f.SubHour > 0:
		// Only sub-agents kept the hour (possible in SDK runs): the main
		// session is where waits cost most, and handing work to sub-agents
		// is not the advice.
		r.Primary = Fix{Structure,
			"Don't let a main session wait on slow commands: start them in the background and check back.",
			"In these logs only sub-agents kept their cache for an hour, so a main session that waits longer " +
				"than 5 minutes saves its whole history again when it carries on."}
		r.Alternatives = nil
	case f.MainHour == 0 && f.SubHour == 0:
		// No session here kept the hour: there is nowhere better to move the wait.
		r.Primary = Fix{Structure,
			"Don't let a session wait on slow commands: start them in the background and check back.",
			"Every session in these logs used the 5-minute cache, so any session that waits longer than 5 minutes " +
				"saves its whole history again when it carries on."}
		r.Alternatives = nil
	}
	return r, true
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
				"token and counts tokens the same way.", f.NewerTurns, joinNames(unique(from))),
			Saving:   f.Newer,
			Examples: unique(from),
			Primary: Fix{Advise,
				"Switch main sessions " + joinNames(unique(moves)) + ".",
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
				"it does the work as well is not known from the logs.", f.TierTurns, joinNames(unique(from)), joinNames(unique(to))),
			Saving:   f.LowerTier,
			Examples: unique(from),
			Overlap:  "May share turns at the highest effort with the effort fix: do not add them",
			Primary: Fix{Advise,
				fmt.Sprintf("Start sessions on %s and switch to %s for the hardest problems.", joinNames(unique(to)), joinNames(unique(from))),
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
	// Come back to the usual level, or to high if the usual level is itself
	// one of the highest.
	back := f.Usual
	if back == "" || back == "xhigh" || back == "max" {
		back = "high"
	}
	finding := fmt.Sprintf("%d turns ran at the highest effort levels (xhigh or max); most turns ran at %s. Had "+
		"%d of them %s as much per turn, on average, as the same model's high-effort turns, they would have "+
		"cost %s less.", f.Turns, f.Usual, f.Turns-f.Unmatched,
		map[bool]string{true: "thought", false: "written"}[f.Thinking], dollars(f.Saving))
	if f.Unmatched > 0 {
		finding += fmt.Sprintf(" The other %d ran on a model with too few high-effort turns to compare with.", f.Unmatched)
	}
	finding += " Harder work gets higher effort on purpose, so a lower level would save less than this."
	how := fmt.Sprintf(`Effort can be set per model ("modelSettings" in ~/.claude/settings.json): if it is, lower it `+
		`there. Otherwise keep the default at %s with /effort %s (or "effortLevel" in .claude/settings.json), and `+
		`raise it with /effort for a task that needs it.`, back, back)
	if len(f.TopModels) > 0 {
		names := make([]string, 0, len(f.TopModels))
		for _, m := range f.TopModels {
			names = append(names, price.DisplayName(m))
		}
		how = fmt.Sprintf(`The xhigh and max turns ran on %s. Effort can be set per model ("modelSettings" in `+
			`~/.claude/settings.json): if it is, lower it there. Otherwise keep the default at %s with /effort %s `+
			`(or "effortLevel" in .claude/settings.json), and raise it with /effort for a task that needs it.`,
			joinNames(unique(names)), back, back)
	}
	return Recommendation{
		Pattern: "P14",
		Title:   "Save the highest effort for hard problems",
		Finding: finding,
		Saving:  f.Saving,
		Overlap: "May share turns with the cheaper-model fixes: do not add them",
		Primary: Fix{Advise,
			fmt.Sprintf("Use xhigh and max effort only for hard problems, and come back to %s after.", back),
			how},
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

// plural writes n with its noun: "1 sub-agent", "304 main sessions".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return thousandsInt(n) + " " + noun + "s"
}
