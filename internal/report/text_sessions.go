package report

import (
	"fmt"
	"io"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/fixes"
)

// attributionSection prints spend by project, skill, plugin and MCP server;
// hideIDs is --no-titles.
func attributionSection(w io.Writer, a detectors.Attribution, hideIDs bool) {
	fmt.Fprintln(w, "\nWHERE THE SPEND WENT")
	spendTable(w, "By project", a.ByProject, a.Cost, true, nil)
	fmt.Fprintln(w, "\n  Turns the harness attributes to a skill, plugin or MCP server:")
	if len(a.BySkill)+len(a.ByPlugin)+len(a.ByMCPServer) == 0 {
		fmt.Fprintln(w, "    none.")
	}
	spendTable(w, "By skill", a.BySkill, a.Cost, false, nil)
	spendTable(w, "By plugin", a.ByPlugin, a.Cost, false, nil)
	spendTable(w, "By MCP server", a.ByMCPServer, a.Cost, false,
		func(n string) string { return serverLabel(n, hideIDs) })
}

// spendTable prints rows of spend; name, if not nil, says how to show a
// row's name.
func spendTable(w io.Writer, title string, rows []detectors.Spend, total float64, sub bool, name func(string) string) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(w, "\n  %s:\n", title)
	fmt.Fprintf(w, "    %-10s %6s %8s %6s", "cost", "share", "turns", "convs")
	if sub {
		fmt.Fprintf(w, " %10s", "sub-agents")
	}
	fmt.Fprintln(w, "  name")
	for _, r := range rows {
		fmt.Fprintf(w, "    %-10s %5.1f%% %8s %6d", dollars(r.Cost), pct(r.Cost, total), thousands(r.Turns), r.Conversations)
		if sub {
			fmt.Fprintf(w, " %9.0f%%", pct(r.SubAgentCost, r.Cost))
		}
		n := r.Name
		if name != nil {
			n = name(n)
		}
		fmt.Fprintf(w, "  %s\n", printable(n))
	}
}

// growthSection prints where spend piles up and how sessions were compacted.
func growthSection(w io.Writer, g detectors.Growth, rec *fixes.Recommendation, all float64) {
	fmt.Fprintln(w, "\nWHERE SPEND PILES UP (P19) AND COMPACTION (P10)")
	if g.Conversations == 0 {
		fmt.Fprintln(w, "  No sessions.")
		return
	}
	fmt.Fprintf(w, "  %s (a main session with its sub-agents); the median one cost %s.\n",
		count(g.Conversations, "conversation"), dollars(g.MedianCost))
	for _, s := range g.TopShares {
		if s.N == 1 {
			fmt.Fprintf(w, "  The costliest one made %.0f%% of all spend.\n", 100*s.Share)
			continue
		}
		fmt.Fprintf(w, "  The costliest %d made %.0f%% of all spend.\n", s.N, 100*s.Share)
	}
	if len(g.Top) > 0 {
		fmt.Fprintf(w, "\n    %-10s %6s %8s %12s  %-20s %s\n", "cost", "share", "turns", "peak context", "project", "conversation")
		for _, c := range g.Top {
			fmt.Fprintf(w, "    %-10s %5.1f%% %8s %12s  %-20s %s\n", dollars(c.Cost), pct(c.Cost, all),
				thousands(c.Turns), thousands(c.Peak), printable(c.Project), printable(c.ID))
		}
	}

	fmt.Fprintln(w, "\n  Cost of a turn by context size (every turn re-sends the whole context):")
	for _, b := range g.Bands {
		label := fmt.Sprintf("%s to %s", short(b.Min), short(b.Max))
		if b.Max == 0 {
			label = "over " + short(b.Min)
		}
		priced := ""
		if b.Priced < b.Turns {
			priced = fmt.Sprintf(" (of the %s with a price)", thousands(b.Priced))
		}
		fmt.Fprintf(w, "    %-16s %8s %s, %s each on average%s\n", label, thousands(b.Turns), agree(b.Turns, "turn", "turns"), dollars(b.MeanCost()), priced)
	}

	fmt.Fprintf(w, "\n  %s (%d asked for)", count(g.Compactions, "compaction"), g.Manual)
	if g.MedianBefore > 0 {
		fmt.Fprintf(w, ", typically from %s to a %s-token summary", thousands(g.MedianBefore), thousands(g.MedianAfter))
		if g.MedianResume > 0 {
			fmt.Fprintf(w, " (the next request sent about %s, with the system prompt and re-attached files)", thousands(g.MedianResume))
		}
	}
	if g.Longest >= time.Second {
		fmt.Fprintf(w, "; the longest took %s", g.Longest.Round(time.Second))
	}
	fmt.Fprintln(w, ".")
	fmt.Fprintf(w, "  %s passed %s tokens and %s never compacted; %s cost %s (%.0f%%).\n",
		count(g.Uncompacted, "session"), thousands(g.Limit), agree(g.Uncompacted, "was", "were"), agree(g.Uncompacted, "it", "they"), dollars(g.UncompactedCost), pct(g.UncompactedCost, all))
	if rec != nil {
		fmt.Fprintf(w, "\n  %s: %s\n", rec.Title, savingText(*rec, all))
		fmt.Fprintln(w, "  This overlaps P19's saving but can be larger: compacting shrinks a session")
		fmt.Fprintln(w, "  well under the limit P19 measures against, to a summary plus the system prompt")
		fmt.Fprintln(w, "  and the files Claude Code attaches again.")
		printFixes(w, *rec)
	}
}

// subAgentSection prints sub-agent spend and the cheaper-model saving.
func subAgentSection(w io.Writer, f detectors.SubAgents, rec *fixes.Recommendation, all float64) {
	fmt.Fprintln(w, "\nSUB-AGENTS ON THE MAIN MODEL (P13)")
	if f.Runs == 0 {
		fmt.Fprintln(w, "  No sub-agent runs.")
		return
	}
	fmt.Fprintf(w, "  %s, %s, cost %s (%.0f%% of all spend).\n",
		count(f.Runs, "sub-agent run"), count(f.Turns, "turn"), dollars(f.Cost), pct(f.Cost, all))
	fmt.Fprintf(w, "  On a cheaper model the same turns would have cost %s less (%.0f%%); untested for quality.\n",
		dollars(f.Saving), pct(f.Saving, all))
	fmt.Fprintf(w, "  Cache writes are %.0f%% of their spend (%.0f%% in main sessions), and each written\n",
		100*f.WriteShare, 100*f.MainWriteShare)
	fmt.Fprintf(w, "  token is read %.0f times (%.0f in main sessions): each run starts with an empty cache.\n",
		f.ReadsPerWrite, f.MainReadsPerWrite)
	if f.Large > 0 {
		fmt.Fprintf(w, "  %s passed %s tokens of context and cost %s.\n", count(f.Large, "run"), thousands(f.LargeAbove), dollars(f.LargeCost))
	}
	fmt.Fprintf(w, "\n    %-10s %6s %6s %8s %10s  %-36s %s\n", "cost", "share", "runs", "turns", "saving", "model -> suggested", "agent type")
	for _, g := range f.ByType {
		models := printable(g.Model)
		if g.Alternative != "" {
			models += " -> " + g.Alternative
		}
		fmt.Fprintf(w, "    %-10s %5.1f%% %6d %8s %10s  %-36s %s\n", dollars(g.Cost), pct(g.Cost, all),
			g.Runs, thousands(g.Turns), dollars(g.Saving), models, printable(g.Type))
	}
	if rec != nil {
		fmt.Fprintf(w, "\n  %s: %s\n", rec.Title, savingText(*rec, all))
		printFixes(w, *rec)
	}
}

// printFixes prints a recommendation's fixes and how to check it.
func printFixes(w io.Writer, r fixes.Recommendation) {
	fixLine(w, "Fix", r.Primary)
	for _, alt := range r.Alternatives {
		fixLine(w, "Or", alt)
	}
	if r.Check != "" {
		for i, line := range wrap(r.Check, 72) {
			lead := "     Check: "
			if i > 0 {
				lead = "            "
			}
			fmt.Fprintf(w, "%s%s\n", lead, line)
		}
	}
}

// short formats a token count briefly: 50000 -> "50k".
func short(n int) string {
	if n%1000 == 0 {
		return fmt.Sprintf("%dk", n/1000)
	}
	return thousands(n)
}
