package fixes

import (
	"fmt"
	"strings"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// ForSubAgents recommends running sub-agents on a cheaper model (P13 in
// reduced fidelity, ADR 0031), or returns false if the saving is under
// minSaving. The saving is untested for quality, so the recommendation says
// how to check it.
func ForSubAgents(f detectors.SubAgents, minSaving float64) (Recommendation, bool) {
	if f.Saving < minSaving || len(f.ByType) == 0 {
		return Recommendation{}, false
	}
	alt, alias := "", ""
	var examples []string
	for _, g := range f.ByType {
		if g.Alternative != "" && g.Saving > 0 {
			if alt == "" {
				alt, alias = price.DisplayName(g.Alternative), agentAlias(g.Alternative)
			}
			if len(examples) < 3 {
				name := g.Type
				if name == "unknown" || name == "" {
					name = "type not recorded"
				}
				examples = append(examples, name)
			}
		}
	}
	if alt == "" { // saving with no named cheaper model: say so plainly
		alt, alias = "a cheaper model", "sonnet"
	}
	return Recommendation{
		Pattern: "P13",
		Title:   "Run sub-agents on a cheaper model",
		Finding: fmt.Sprintf("Sub-agents (helpers Claude starts for side tasks) cost %s, and %.0f%% of that "+
			"was on a model with a cheaper choice. The saving assumes all of those moved to %s.",
			dollars(f.Cost), 100*f.CostlyShare(), alt),
		Saving:   f.Saving,
		Examples: examples,
		Primary: Fix{Structure,
			fmt.Sprintf("Run sub-agents that search, read and summarise on %s.", alt),
			`Define the agents you use for searching, exploring and web research in .claude/agents/<name>.md ` +
				`with "model: ` + alias + `" in the front matter, and use them instead of general-purpose.`},
		Alternatives: []Fix{
			{Advise,
				"Ask for the cheaper model when starting a sub-agent.",
				`Add to CLAUDE.md: "When starting a sub-agent to search, read or summarise, pass model: ` + alias + `; ` +
					`keep the main model for design work and tricky changes."`},
		},
		Check: "Untested for quality: after a week, compare the agent type's turns per run and tool errors " +
			"with before, and switch back if they rose.",
	}, true
}

// ForCompaction recommends compacting earlier when sessions grew past the
// limit without compacting (P19's fix, with P10's records; ADR 0031), or
// returns false if compacting would save less than minCost. threshold is the context
// size, in tokens, at which to compact; likely and upTo come from
// detectors.LikelyCompactionSaving, and after is the size it assumed.
func ForCompaction(g detectors.Growth, threshold int, upTo, likely float64, after int, minCost float64) (Recommendation, bool) {
	if upTo < minCost {
		return Recommendation{}, false
	}
	return Recommendation{
		Pattern: "P19",
		Title:   "Compact long sessions earlier",
		Finding: fmt.Sprintf("Every turn sends the whole session again, so long sessions get dearer each turn. "+
			"%d sessions passed %s tokens without compacting; Claude Code compacts on its own only near the limit.",
			g.Uncompacted, thousandsInt(g.Limit)),
		Saving:    upTo,
		Likely:    likely,
		Estimated: true,
		LikelyNote: fmt.Sprintf("Replays each main session as if compacted to %s tokens (what the first request "+
			"after a compaction typically sends) whenever it passed %s, "+
			"including the cost of each compaction, and assumes it then went on the same way. "+
			"Re-reading files after a compaction would lower the saving.", thousandsInt(after), thousandsInt(threshold)),
		Overlap: "Shares money with the long-pause and tool-output fixes: do not add them",
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

// ForPauses recommends starting fresh after long breaks (P09), or returns
// false if the cost is under minSaving. likely is the replayed estimate
// (detectors.LikelyPauseSaving) and fresh the new-session size it assumed.
// startCost, if above 0, is the median cost of a new session's first turn,
// which the finding quotes (ADR 0050).
func ForPauses(p detectors.LongPause, likely float64, fresh int, startCost, minSaving float64) (Recommendation, bool) {
	// Only breaks the user took: after the others, no message from the user
	// started the next turn, so there was no moment to start fresh.
	saving := p.Saving
	if saving < minSaving {
		return Recommendation{}, false
	}
	gap := spanText(p.MinGap)
	finding := fmt.Sprintf("%d times, you carried on after more than %s away. The saved copy of the "+
		"session (the cache) had expired, so the next turn paid to store all of it again.", p.Pauses-p.Unprompted, gap)
	if startCost > 0 {
		finding += fmt.Sprintf(" Starting a new session instead costs about %s for its first turn.", cents(startCost))
	}
	switch {
	case p.Unprompted == 1:
		finding += " Another long wait was on a tool, a sub-agent, a timer or another agent, not on you; it is left out of this saving."
	case p.Unprompted > 1:
		finding += fmt.Sprintf(" Another %d long waits were on a tool, a sub-agent, a timer or another agent, not on you; "+
			"they are left out of this saving.", p.Unprompted)
	}
	return Recommendation{
		Pattern:    "P09",
		Title:      "Start fresh after a long break",
		Finding:    finding,
		Saving:     saving,
		Likely:     likely,
		Estimated:  true,
		LikelyNote: fmt.Sprintf("As if each of those turns had started a new session of about %s tokens.", thousandsInt(fresh)),
		Overlap:    "Shares money with the compaction fix: do not add them",
		Primary: Fix{Advise,
			"After a break of over " + gap + ", start a new session with a short note instead of carrying on.",
			"Before the break, ask Claude for a short note of where things stand; afterwards, /clear and paste it in."},
		Alternatives: []Fix{
			{Structure,
				"Compact before a long break, while the cache is still warm.",
				"Run /compact before stepping away; compacting after the break re-sends the whole session."},
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

// dollars formats v as dollars with commas and no cents: "$1,235".
func dollars(v float64) string {
	if v < 0 {
		return "-" + dollars(-v)
	}
	return "$" + thousandsInt(int(v+0.5))
}

// agentAlias is the name Claude Code's agent front matter takes for a model:
// "sonnet" for claude-sonnet-5-5. Unknown families fall back to "sonnet".
func agentAlias(model string) string {
	for _, family := range []string{"haiku", "sonnet", "opus", "fable"} {
		if strings.Contains(model, family) {
			return family
		}
	}
	return "sonnet"
}

// spanText writes the length of a break for a sentence: "an hour",
// "2 hours", "90 minutes". The P09 gap is an hour unless set otherwise.
func spanText(d time.Duration) string {
	switch {
	case d == time.Hour || d <= 0:
		return "an hour"
	case d%time.Hour == 0:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	return fmt.Sprintf("%d minutes", int(d.Minutes()))
}

// cents writes a small amount with its cents: "$0.06", "$1.25"; amounts of
// $10 or more as whole dollars.
func cents(v float64) string {
	if v >= 10 {
		return dollars(v)
	}
	return fmt.Sprintf("$%.2f", v)
}
