package report

import (
	"bufio"
	"fmt"
	"io"
)

// WriteSummary prints a few lines for the terminal when the full report went
// to a file: what was read, total spend, the top actions, and where the
// report is.
func WriteSummary(out io.Writer, r Results, path string, actions int) error {
	w := bufio.NewWriter(out)
	readLine(w, r.Read)
	fmt.Fprintf(w, "Spend %s at list prices.\n", dollars(r.Total))
	if len(r.Actions) > 0 {
		fmt.Fprintln(w, "\nTop actions (savings overlap, so do not add them):")
		for i, a := range r.Actions[:min(actions, len(r.Actions))] {
			fmt.Fprintf(w, "  %d. %s: up to %s (%.0f%%)\n", i+1, a.Title, dollars(a.Saving), pct(a.Saving, r.Total))
			fmt.Fprintf(w, "     %s\n", a.Primary.What)
		}
	}
	fmt.Fprintf(w, "\nFull report: %s\n", path)
	return w.Flush()
}
