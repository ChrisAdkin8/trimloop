package report

import (
	"fmt"
	"io"
	"strings"
)

// The sections from the second gap analysis (ADR 0050).

// thinkingLine prints P14's whole thinking share of output.
func thinkingLine(w io.Writer, r Results) {
	if e := r.Effort; e.ThinkingCost > 0 && e.OutputCost > 0 {
		fmt.Fprintf(w, "  Thinking was %s of %s output spend (%.0f%%), at every level.\n",
			dollars(e.ThinkingCost), dollars(e.OutputCost), pct(e.ThinkingCost, e.OutputCost))
	}
}

// toolInputLine prints what the text of Claude's own tool calls cost to carry.
func toolInputLine(w io.Writer, r Results) {
	ti := r.ToolInputs
	if ti.Carried <= 0 {
		return
	}
	var top []string
	for _, g := range ti.ByTool[:min(3, len(ti.ByTool))] {
		top = append(top, fmt.Sprintf("%s %s", g.Name, dollars(g.Carried)))
	}
	fmt.Fprintf(w, "  What Claude wrote in its tool calls (commands, scripts, file contents) also stays in the\n")
	fmt.Fprintf(w, "  session: %s to carry (%.0f%% of spend; %s). Compacting clears it too.\n",
		dollars(ti.Carried), pct(ti.Carried, r.Total), strings.Join(top, ", "))
}

// gapSections prints model switches (P01), cut-off replies (P12), unused
// app servers (P07) and when the spend happened.
func gapSections(w io.Writer, r Results) {
	all := r.Total
	if sw := r.Switches; sw.Switches > 0 {
		fmt.Fprintln(w, "\nP01 MODEL SWITCHES MID-SESSION")
		fmt.Fprintf(w, "  %d switches in %s: the first turn on the new model saved the whole conversation\n",
			sw.Switches, count(sw.Sessions, "session"))
		fmt.Fprintf(w, "  to the cache again. Those turns cost %s; reading instead would have saved %s.\n",
			dollars(sw.Cost), dollars(sw.Saving))
		if r.SwitchFix != nil {
			fmt.Fprintf(w, "\n  %s: %s\n", r.SwitchFix.Title, savingText(*r.SwitchFix, all))
			printFixes(w, *r.SwitchFix)
		}
	}
	if f := r.Failures; f.Sleep+f.Network+f.Other > 0 {
		fmt.Fprintln(w, "\nP12 REPLIES CUT OFF")
		fmt.Fprintf(w, "  Requests that failed and were sent again: %d when the computer slept, %d network,\n", f.Sleep, f.Network)
		fmt.Fprintf(w, "  %d other. Sending the conversation again cost at least %s; the cut-off output is not logged.\n",
			f.Other, money(f.Resent))
		if r.FailureFix != nil {
			fmt.Fprintf(w, "\n  %s: %s\n", r.FailureFix.Title, savingText(*r.FailureFix, all))
			printFixes(w, *r.FailureFix)
		}
	}
	if r.AppServerFix != nil {
		fmt.Fprintln(w, "\nP07 DESKTOP-APP TOOLS NEVER USED")
		fmt.Fprintf(w, "  %s: %s\n", r.AppServerFix.Title, savingText(*r.AppServerFix, all))
		printFixes(w, *r.AppServerFix)
	}
	whenSection(w, r)
}

var weekdays = [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// whenSection prints when the spend happened.
func whenSection(w io.Writer, r Results) {
	wh := r.When
	if r.Total <= 0 {
		return
	}
	fmt.Fprintln(w, "\nWHEN YOU WORK (local time)")
	var days []string
	for i, v := range wh.ByWeekday {
		days = append(days, fmt.Sprintf("%s %s", weekdays[i], money(v)))
	}
	fmt.Fprintf(w, "  By day: %s (weekends %.0f%%).\n", strings.Join(days, ", "), 100*wh.WeekendShare)
	peak := 0
	for h, v := range wh.ByHour {
		if v > wh.ByHour[peak] {
			peak = h
		}
	}
	fmt.Fprintf(w, "  Busiest hour: %02d:00–%02d:00 (%s). Most main sessions running at once: %d.\n",
		peak, (peak+1)%24, money(wh.ByHour[peak]), wh.MaxConcurrent)
}

// retentionLine prints a tip when Claude Code keeps logs for under 90 days.
func retentionLine(w io.Writer, r Results) {
	if !r.ShortRetention() {
		return
	}
	how := "by default"
	if r.Retention.Set {
		how = "as cleanupPeriodDays is set"
	}
	fmt.Fprintf(w, "Claude Code deletes logs after %d days %s, so this sees only that far back: add\n", r.Retention.Days, how)
	fmt.Fprintln(w, `"cleanupPeriodDays": 365 to ~/.claude/settings.json to keep a year.`)
}
