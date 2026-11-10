package report

import (
	"fmt"
	"io"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
)

// reconcileSection compares the totals with Claude Code's own records;
// labels name the sessions with the largest gaps.
func reconcileSection(w io.Writer, c detectors.Reconciliation, all float64, labels map[string]Label) {
	fmt.Fprintln(w, "\nCHECKED AGAINST CLAUDE CODE'S OWN RECORDS")
	compared := fmt.Sprintf("  For %s with a record, Claude Code recorded %s and the logs show %s:\n",
		count(c.Sessions, "session"), money(c.Harness), money(c.Ours))
	switch {
	case c.Sessions == 0 && c.Unpriced+c.Behind == 0:
		fmt.Fprintln(w, "  No session has a cost record from Claude Code to check against.")
	case c.Sessions == 0:
		fmt.Fprintln(w, "  No session could be checked; the reasons follow.")
	case c.Gap >= 0:
		fmt.Fprint(w, compared)
		fmt.Fprintf(w, "  %s (%.1f%%) was spent on requests the logs do not record. Part of it is\n", money(c.Gap), pct(c.Gap, c.Ours))
		fmt.Fprintln(w, "  background requests: session titles, summaries after a break, compactions and")
		fmt.Fprintln(w, "  web searches.")
		if models := missingCachedWork(c); models != "" {
			fmt.Fprintln(w, "  The record also counts more cached work (cache reads and writes) than the logs")
			fmt.Fprintf(w, "  hold for %s:\n", printable(models))
			fmt.Fprintln(w, "  some of the sessions' own requests are missing from the logs, and why is not known.")
		}
		fmt.Fprintln(w, "  The findings in this report are worked out from the logs, so they leave this spend out.")
	default:
		fmt.Fprint(w, compared)
		fmt.Fprintf(w, "  the logs show %s more than the records; some records may predate the session's end.\n", money(-c.Gap))
	}
	if c.Unpriced > 0 {
		fmt.Fprintf(w, "  %s usage with no known price, in the logs or in the record, and %s left out.\n",
			count(c.Unpriced, "session")+agree(c.Unpriced, " has", " have"), agree(c.Unpriced, "is", "are"))
	}
	if c.WebSearches > 0 {
		fmt.Fprintf(w, "  %s cost about %s on top of their tokens.\n", count(c.WebSearches, "web search"), money(c.WebSearchCost))
	}
	if c.Unrecorded > 0 {
		fmt.Fprintf(w, "  %s no record (%s, %.0f%% of spend): %s cannot be checked.\n",
			count(c.Unrecorded, "session")+agree(c.Unrecorded, " has", " have"), money(c.UnrecordedCost), pct(c.UnrecordedCost, all), agree(c.Unrecorded, "its total", "their totals"))
	}
	if c.Behind > 0 {
		fmt.Fprintf(w, "  %s a record that does not cover all of %s (%s before the record's first run,\n"+
			"  or went on after its last) and %s left out.\n",
			count(c.Behind, "session")+agree(c.Behind, " has", " have"), agree(c.Behind, "it", "them"),
			agree(c.Behind, "it began", "they began"), agree(c.Behind, "is", "are"))
	}
	if c.Sessions == 0 {
		return
	}
	fmt.Fprintf(w, "\n    %-10s %-10s %-10s %s\n", "recorded", "in logs", "gap", "model")
	for _, m := range c.ByModel {
		fmt.Fprintf(w, "    %-10s %-10s %-10s %s\n", money(m.Harness), money(m.Ours), money(m.Gap), printable(modelName(m.Model)))
	}
	fmt.Fprintln(w, "\n  Tokens by model, recorded / in logs:")
	fmt.Fprintf(w, "    %-17s %-17s %-15s %-15s %s\n", "cache reads", "cache writes", "output", "uncached input", "model")
	for _, m := range c.ByModel {
		fmt.Fprintf(w, "    %-17s %-17s %-15s %-15s %s\n", tokenPair(m.HarnessCacheRead, m.OursCacheRead),
			tokenPair(m.HarnessCacheWrite, m.OursCacheWrite), tokenPair(m.HarnessOutput, m.OursOutput),
			tokenPair(m.HarnessUncached, m.OursUncached), printable(modelName(m.Model)))
	}
	if len(c.LargestGaps) == 0 {
		return
	}
	fmt.Fprintf(w, "\n  Sessions with the largest gaps (%s of the %s):\n", money(c.Held()), money(c.Gap))
	fmt.Fprintf(w, "    %-10s %-10s %-10s %14s  %-20s %s\n", "recorded", "in logs", "gap", "sub-agent logs", "project", "session")
	for _, s := range c.LargestGaps {
		l := labels[s.ID]
		name := s.ID
		if l.Title != "" {
			name += "  " + l.Title
		}
		fmt.Fprintf(w, "    %-10s %-10s %-10s %14d  %-20s %s\n", money(s.Harness), money(s.Ours), money(s.Gap),
			s.SubAgentLogs, printable(l.Project), printable(name))
	}
}

// missingCachedWork names the models whose record counts clearly more
// cached work than the logs hold (ModelGap.MissingCachedWork), as "Opus 5.5
// and Haiku 5.5"; "" if none.
func missingCachedWork(c detectors.Reconciliation) string {
	var names []string
	for _, m := range c.ByModel {
		if m.MissingCachedWork() {
			names = append(names, modelName(m.Model))
		}
	}
	return joinAnd(names)
}

// tokenPair writes a recorded and a logged token count: "125.0M / 88.0M".
func tokenPair(recorded, logged int) string { return approx(recorded) + " / " + approx(logged) }

// standingSection prints what every request carries (P07).
func standingSection(w io.Writer, s detectors.StandingContext, all float64, hideIDs bool) {
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
		fmt.Fprintf(w, "    %-10s %8s  %s\n", money(p.Cost), thousands(p.Tokens), printable(contextName(p.Name, hideIDs)))
	}
}

// weeksSection prints the figures week by week.
func weeksSection(w io.Writer, weeks []detectors.Week, prices string) {
	if len(weeks) == 0 {
		return
	}
	fmt.Fprintln(w, "\nWEEK BY WEEK")
	fmt.Fprintf(w, "    %-11s %9s %8s %10s %10s %10s %8s %9s %7s %9s %9s %9s %6s\n", "week of", "spend", "turns", "large ctx", "sub-agents",
		"tool out", "re-reads", "breaks", "compact", "short wt", "top eff", "cheaper", "limits")
	for _, k := range weeks {
		kept := ""
		if k.Kept {
			kept = "  (kept from an earlier run"
			if k.KeptWhy != "" {
				kept += ": " + k.KeptWhy
			}
			kept += ")"
		}
		if reason := olderReason(k, prices); reason != "" {
			kept += "  (" + reason + ")"
		}
		fmt.Fprintf(w, "    %-11s %9s %8s %9.0f%% %9.0f%% %10s %7.0f%% %9s %7d %9s %9s %9s %6s%s\n", k.Week, money(k.Spend), thousands(k.Turns),
			pct(k.LargeContext, k.Spend), pct(k.SubAgents, k.Spend), money(k.ToolOutput),
			pct(float64(k.Rereads), float64(k.FileViews)), money(k.PauseRewrite), k.Compactions,
			measured(k, k.ShortWaits), measured(k, k.TopEffort), measured(k, k.OlderModels), limitHits(k), kept)
	}
}

// olderFigures reports whether a week's figures were worked out with another
// price list or an older analysis than this run's, so they may not compare.
func olderFigures(k detectors.Week, prices string) bool { return olderReason(k, prices) != "" }

// olderReason says why a week's figures may not compare with this run's:
// an older analysis, another price list, or both; "" if neither.
func olderReason(k detectors.Week, prices string) string {
	method := k.Analysis < detectors.AnalysisVersion // 0: stored before versions were recorded
	otherPrices := k.Prices != "" && prices != "" && k.Prices != prices
	switch {
	case method && otherPrices:
		return "older method and prices"
	case method && k.Analysis == 0:
		return "older method"
	case method:
		return fmt.Sprintf("older method: analysis %d, now %d", k.Analysis, detectors.AnalysisVersion)
	case otherPrices:
		return "older prices: " + k.Prices
	}
	return ""
}

// measured writes a figure added in analysis version 5 (NewFindingsSince), or "-" for a week an
// earlier version stored, which did not measure it.
func measured(k detectors.Week, v float64) string {
	if !k.HasNewFindings() {
		return "-"
	}
	return money(v)
}

// limitHits writes a week's limit hits, or "-" for a week stored before
// analysis version 7, which did not count them.
func limitHits(k detectors.Week) string {
	if !k.HasLimitHits() {
		return "-"
	}
	return fmt.Sprint(k.LimitHits)
}
