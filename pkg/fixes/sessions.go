package fixes

import (
	"fmt"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
)

// ForSubAgents recommends running sub-agents on a cheaper model (P13 in
// reduced fidelity, ADR 0031), or returns false if the saving is under
// minSaving. The saving is untested for quality, so the recommendation says
// how to check it.
func ForSubAgents(f detectors.SubAgents, minSaving float64) (Recommendation, bool) {
	if f.Saving < minSaving || len(f.ByType) == 0 {
		return Recommendation{}, false
	}
	alt := ""
	var examples []string
	for _, g := range f.ByType {
		if g.Alternative != "" && g.Saving > 0 {
			if alt == "" {
				alt = g.Alternative
			}
			if len(examples) < 3 {
				examples = append(examples, g.Type)
			}
		}
	}
	return Recommendation{
		Pattern:  "P13",
		Title:    "Sub-agents on the main model",
		Saving:   f.Saving,
		Examples: examples,
		Primary: Fix{Structure,
			fmt.Sprintf("Run sub-agents that search, read and summarise on %s.", alt),
			`Define the agents you use for searching, exploring and web research in .claude/agents/<name>.md ` +
				`with "model: sonnet" in the front matter, and use them instead of general-purpose.`},
		Alternatives: []Fix{
			{Advise,
				"Ask for the cheaper model when starting a sub-agent.",
				`Add to CLAUDE.md: "When starting a sub-agent to search, read or summarise, pass model: sonnet; ` +
					`keep the main model for design work and tricky changes."`},
		},
		Check: "Untested for quality: after a week, compare the agent type's turns per run and tool errors " +
			"with before, and switch back if they rose.",
	}, true
}

// ForCompaction recommends compacting earlier when sessions grew past the
// limit without compacting (P19's fix, with P10's records; ADR 0031), or
// returns false if they cost less than minCost. threshold is the context
// size, in tokens, at which to compact; saving is P19's, which this fix
// shares with starting new sessions.
func ForCompaction(g detectors.Growth, threshold int, saving, minCost float64) (Recommendation, bool) {
	if g.UncompactedCost < minCost {
		return Recommendation{}, false
	}
	return Recommendation{
		Pattern: "P19",
		Title:   "Sessions that grew without compacting",
		Saving:  saving,
		Overlap: "P19's saving: shared with P09 and P04, so not to be added to them",
		Primary: Fix{Advise,
			fmt.Sprintf("Compact when a session passes about %s tokens, or between tasks.", thousandsInt(threshold)),
			"Run /compact yourself (/context shows the current size). Claude Code compacts on its own only " +
				"near the context limit."},
		Alternatives: []Fix{
			{Structure,
				"Start a new session for each task.",
				"Use /clear between tasks, carrying over a short note of where things stand."},
		},
	}, true
}

// thousandsInt formats n with comma separators: 200000 -> "200,000".
func thousandsInt(n int) string {
	s := fmt.Sprint(n)
	out := make([]byte, 0, len(s)+len(s)/3)
	for i := range len(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}
