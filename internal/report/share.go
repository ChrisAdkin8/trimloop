package report

import (
	"bufio"
	"fmt"
	"io"
)

// shareURL is where a reader of a shared summary can find trimloop.
const shareURL = "https://github.com/ChrisAdkin8/trimloop"

// WriteShare prints a few lines that are safe to paste where others can
// read them: figures and trimloop's own wording only. It leaves out
// everything that comes from the user's machine as text (project names,
// session titles and ids, folder paths, the names of skills, plugins, MCP
// servers and agents), and the dates and the plan's name, which say more
// about a person than a reader needs. An action's title is trimloop's own
// sentence; the only names in one are Claude models.
func WriteShare(out io.Writer, r Results, actions int) error {
	w := bufio.NewWriter(out)
	fmt.Fprint(w, "Claude Code usage")
	if d := r.Period.Days; d > 0 {
		fmt.Fprintf(w, " over %s", count(d, "day"))
	}
	fmt.Fprintf(w, ": %s at list prices", money(r.Total))
	if r.Period.PerWeek > 0 {
		fmt.Fprintf(w, ", about %s a week", money(r.Period.PerWeek))
	}
	fmt.Fprintf(w, " (%s, %s).\n", count(r.Read.Sessions, "session"), count(r.Read.Turns, "turn"))
	if r.Subscriber() {
		fmt.Fprintln(w, "On a flat-fee plan, so list prices measure how much of the usage limits the work takes.")
	}
	if s := newSummary(r); s.LargeSpendShare >= 1 {
		fmt.Fprintf(w, "%.0f%% of the spend went on sending turns over %s tokens.\n", s.LargeSpendShare, approx(s.Threshold))
	}
	if len(r.Actions) > 0 {
		fmt.Fprintln(w, "\nWhat would save the most (the savings overlap, so they are not added up):")
		for i, a := range r.Actions[:min(actions, len(r.Actions))] {
			fmt.Fprintf(w, "  %d. %s: %s%s\n", i+1, a.Title, savingText(a, r.Total), limitText(a))
		}
	}
	fmt.Fprint(w, "\nWorked out by trimloop")
	if r.Version != "" {
		fmt.Fprintf(w, " %s", r.Version)
	}
	fmt.Fprintf(w, " from local logs: %s\n", shareURL)
	return w.Flush()
}
