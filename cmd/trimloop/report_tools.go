package main

import (
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/fixes"
)

// toolOutputSection prints what fills the context (P04) and the fixes for it.
// all is the total spend, for percentages.
func toolOutputSection(w io.Writer, f detectors.ToolOutput, recs []fixes.Recommendation, all float64) {
	fmt.Fprintf(w, "\nP04 WHAT FILLS YOUR CONTEXT: TOOL OUTPUT\n")
	fmt.Fprintf(w, "  %s tool results added about %s tokens (at %d bytes per token).\n",
		thousands(f.Calls), thousands(f.Tokens), event.BytesPerToken)
	fmt.Fprintf(w, "  Each stays in the context and is sent again on every later turn. Carried\n")
	fmt.Fprintf(w, "  until the session ended or was compacted, they cost %s (%.0f%% of all spend).\n",
		dollars(f.Carried), pct(f.Carried, all))
	fmt.Fprintf(w, "  Keeping each result under %s tokens would have saved up to %s (%.0f%%).\n",
		thousands(f.Target), dollars(f.Saving), pct(f.Saving, all))
	fmt.Fprintln(w, "  This is part of P19's cost, not extra to it.")
	if f.Persisted > 0 {
		fmt.Fprintf(w, "  %s results were too large and saved to disk; the model saw a preview.\n", thousands(f.Persisted))
	}
	if f.FileViews > 0 {
		fmt.Fprintf(w, "  %s of %s file views (%.0f%%) re-read a file already viewed, unchanged, in\n",
			thousands(f.Rereads), thousands(f.FileViews), pct(float64(f.Rereads), float64(f.FileViews)))
		fmt.Fprintf(w, "  the session; carrying them cost %s.\n", dollars(f.RereadCarried))
	}

	groupTable(w, "By tool", "tool", f.ByTool, f.Carried, false)
	groupTable(w, "Shell commands by shape", "shape", f.ByShape, f.Carried, true)

	if len(recs) == 0 {
		return
	}
	fmt.Fprintln(w, "\n  Recommended fixes, largest saving first:")
	for i, r := range recs {
		fmt.Fprintf(w, "\n  %d. %s: up to %s", i+1, r.Title, dollars(r.Saving))
		if len(r.Examples) > 0 {
			fmt.Fprintf(w, " (mostly %s)", printable(strings.Join(r.Examples, ", ")))
		}
		fmt.Fprintln(w)
		printFixes(w, r)
	}
	fmt.Fprintln(w, "\n  Check the effect with another run a week after applying a fix.")
}

// groupTable prints groups with their carried cost and share.
func groupTable(w io.Writer, title, nameLabel string, groups []detectors.Group, total float64, filtered bool) {
	if len(groups) == 0 {
		return
	}
	fmt.Fprintf(w, "\n  %s:\n", title)
	fmt.Fprintf(w, "    %-10s %6s %8s %11s %10s", "carried", "share", "calls", "tokens", "saving")
	if filtered {
		fmt.Fprintf(w, " %9s", "filtered")
	}
	fmt.Fprintf(w, "  %s\n", nameLabel)
	for _, g := range groups {
		fmt.Fprintf(w, "    %-10s %5.1f%% %8s %11s %10s", dollars(g.Carried), pct(g.Carried, total),
			thousands(g.Calls), thousands(g.Tokens), dollars(g.Saving))
		if filtered {
			fmt.Fprintf(w, " %8.0f%%", pct(float64(g.Filtered), float64(g.Calls)))
		}
		fmt.Fprintf(w, "  %s\n", printable(g.Name))
	}
}

// fixLine prints one fix: its kind and what it is, then how to apply it.
func fixLine(w io.Writer, label string, f fixes.Fix) {
	fmt.Fprintf(w, "     %s (%s): %s\n", label, f.Kind, f.What)
	for _, line := range wrap(f.How, 70) {
		fmt.Fprintf(w, "       %s\n", line)
	}
}

// wrap splits s into lines of at most width characters, at spaces.
func wrap(s string, width int) []string {
	var lines []string
	var cur strings.Builder
	for _, word := range strings.Fields(s) {
		if cur.Len() > 0 && cur.Len()+1+len(word) > width {
			lines = append(lines, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(word)
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

// printable replaces control characters in s, which comes from logs (shapes
// are built from commands the agent wrote, tool names from the harness), so
// that printing it cannot send escape sequences to the user's terminal.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == utf8.RuneError {
			return '?'
		}
		return r
	}, s)
}
