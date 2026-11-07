package report

import (
	"fmt"
	"io"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
)

// reconcileSection compares the totals with Claude Code's own records.
func reconcileSection(w io.Writer, c detectors.Reconciliation, all float64) {
	fmt.Fprintln(w, "\nCHECKED AGAINST CLAUDE CODE'S OWN RECORDS")
	if c.Sessions == 0 {
		fmt.Fprintln(w, "  No session has a cost record from Claude Code to check against.")
		return
	}
	fmt.Fprintf(w, "  For %d sessions with a record, Claude Code recorded %s and the logs show %s:\n",
		c.Sessions, money(c.Harness), money(c.Ours))
	fmt.Fprintf(w, "  %s (%.1f%%) was spent on background requests the logs do not show,\n", money(c.Gap), pct(c.Gap, c.Ours))
	fmt.Fprintln(w, "  such as session titles, summaries after a break and web searches.")
	if c.WebSearches > 0 {
		fmt.Fprintf(w, "  %d web searches cost about %s on top of their tokens.\n", c.WebSearches, money(c.WebSearchCost))
	}
	if c.Unrecorded > 0 {
		fmt.Fprintf(w, "  %d sessions have no record (%s, %.0f%% of spend): their totals cannot be checked.\n",
			c.Unrecorded, money(c.UnrecordedCost), pct(c.UnrecordedCost, all))
	}
	if c.Behind > 0 {
		fmt.Fprintf(w, "  %d sessions went on after their record was written and are left out.\n", c.Behind)
	}
	fmt.Fprintf(w, "\n    %-10s %-10s %-10s %s\n", "recorded", "in logs", "gap", "model")
	for _, m := range c.ByModel {
		fmt.Fprintf(w, "    %-10s %-10s %-10s %s\n", money(m.Harness), money(m.Ours), money(m.Gap), modelName(m.Model))
	}
}

// standingSection prints what every request carries (P07).
func standingSection(w io.Writer, s detectors.StandingContext, all float64) {
	fmt.Fprintln(w, "\nP07 WHAT EVERY REQUEST CARRIES")
	fmt.Fprintf(w, "  Sending the system prompt and tool definitions on every turn cost %s;\n", money(s.SystemPrompt+s.ToolDefinitions))
	fmt.Fprintf(w, "  text Claude Code added to the conversation cost %s while it stayed there.\n", money(s.Injected))
	fmt.Fprintf(w, "  Together %s (%.0f%% of spend).", money(s.Total()), pct(s.Total(), all))
	if s.Covered < s.Turns {
		fmt.Fprintf(w, " The system prompt and tools were recorded for %.0f%% of turns.", pct(float64(s.Covered), float64(s.Turns)))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "\n    %-10s %8s  %s\n", "cost", "tokens", "source")
	for _, p := range s.Parts[:min(12, len(s.Parts))] {
		fmt.Fprintf(w, "    %-10s %8s  %s\n", money(p.Cost), thousands(p.Tokens), printable(contextName(p.Name)))
	}
}

// weeksSection prints the figures week by week.
func weeksSection(w io.Writer, weeks []detectors.Week) {
	if len(weeks) == 0 {
		return
	}
	fmt.Fprintln(w, "\nWEEK BY WEEK")
	fmt.Fprintf(w, "    %-11s %9s %8s %10s %10s %10s %8s\n", "week of", "spend", "turns", "large ctx", "sub-agents", "tool out", "re-reads")
	for _, k := range weeks {
		kept := ""
		if k.Kept {
			kept = "  (kept from an earlier run)"
		}
		fmt.Fprintf(w, "    %-11s %9s %8s %9.0f%% %9.0f%% %10s %7.0f%%%s\n", k.Week, money(k.Spend), thousands(k.Turns),
			pct(k.LargeContext, k.Spend), pct(k.SubAgents, k.Spend), money(k.ToolOutput),
			pct(float64(k.Rereads), float64(k.FileViews)), kept)
	}
}
