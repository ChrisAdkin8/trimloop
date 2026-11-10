package report

import (
	"bufio"
	"fmt"
	"io"

	"github.com/ChrisAdkin8/trimloop/pkg/fixes"
)

// WriteSummary prints a few lines for the terminal: what was read, total
// spend, the top actions, and where the full report is. path is the file the
// full report went to; if it is empty, the summary says how to get one.
func WriteSummary(out io.Writer, r Results, path string, actions int) error {
	w := bufio.NewWriter(out)
	readLine(w, r.Read)
	periodLine(w, r)
	if c := r.Reconciliation; c.Sessions > 0 && c.Gap > 0.005*c.Ours {
		fmt.Fprintf(w, "Claude Code's own records show %s (%.0f%%) more than its logs: requests the logs do not record.\n",
			money(c.Gap), pct(c.Gap, c.Ours))
		if c.Concentrated() {
			fmt.Fprintf(w, "%s of it is in %s. ", money(c.Held()), count(len(c.LargestGaps), "session"))
		}
		fmt.Fprintln(w, "The findings below come from the logs, so they leave it out.")
	}
	limitLines(w, r)
	if len(r.Actions) > 0 {
		fmt.Fprintln(w, "\nWhat to do first (these savings overlap, so do not add them):")
		for i, a := range r.Actions[:min(actions, len(r.Actions))] {
			fmt.Fprintf(w, "  %d. %s: %s%s\n", i+1, a.Title, savingText(a, r.Total), limitText(a))
			fmt.Fprintf(w, "     %s\n", a.Primary.What)
		}
	}
	if path != "" {
		fmt.Fprintf(w, "\nFull report: %s\n", path)
	} else {
		fmt.Fprintln(w, "\nFor the full report: --format html (a web page) or --format text.")
	}
	return w.Flush()
}

// periodLine prints the spend and the period it covers.
func periodLine(w io.Writer, r Results) {
	fmt.Fprintf(w, "Spent %s at list prices", money(r.Total))
	if p := r.Period; p.Days > 0 {
		fmt.Fprintf(w, " from %s to %s (%s)",
			p.From.Local().Format("2 Jan"), p.To.Local().Format("2 Jan 2006"), count(p.Days, "day"))
		if p.PerWeek > 0 {
			fmt.Fprintf(w, ", about %s a week", money(p.PerWeek))
		}
	}
	fmt.Fprintln(w, ".")
}

// savingText is "likely about $X, up to $Y (Z%)", or "up to $Y (Z%)" when
// there is no likelier estimate.
func savingText(a fixes.Recommendation, total float64) string {
	if a.SavesNothing() {
		return fmt.Sprintf("likely nothing (%s), up to %s (%.0f%%)", a.Nothing, money(a.Saving), pct(a.Saving, total))
	}
	if a.Likely > 0 && a.LikelyHigh > a.LikelyLow {
		return fmt.Sprintf("likely about %s (%s–%s), up to %s (%.0f%%)", money(a.Likely), money(a.LikelyLow), money(a.LikelyHigh),
			money(a.Saving), pct(a.Saving, total))
	}
	if a.Likely > 0 {
		return fmt.Sprintf("likely about %s, up to %s (%.0f%%)", money(a.Likely), money(a.Saving), pct(a.Saving, total))
	}
	return fmt.Sprintf("up to %s (%.0f%%)", money(a.Saving), pct(a.Saving, total))
}
