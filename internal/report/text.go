package report

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
)

// WriteText prints the full report as plain text to out. It writes through a
// bufio.Writer, which keeps the first write error; the error Flush returns is
// the only one that needs checking.
func WriteText(out io.Writer, r Results) error {
	w := bufio.NewWriter(out)
	ctx, pause := r.Context, r.Pause
	total := r.Cost
	all := r.Total

	readLine(w, r.Read)
	periodLine(w, r)
	fmt.Fprintf(w, "Costs are estimates at list prices (%s).", r.Prices)
	if r.Read.Unpriced > 0 {
		fmt.Fprintf(w, " %d turns have no price and are left out.", r.Read.Unpriced)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "\nCOST BY TOKEN CLASS")
	for _, row := range []struct {
		name string
		cost float64
	}{
		{"Cache reads", total.CacheRead},
		{"Cache writes, 1 hour", total.CacheWrite1h},
		{"Cache writes, 5 min", total.CacheWrite5m},
		{"Output", total.Output},
		{"Uncached input", total.InputUncached},
	} {
		fmt.Fprintf(w, "  %-22s %10s  %5.1f%%\n", row.name, dollars(row.cost), pct(row.cost, all))
	}
	fmt.Fprintf(w, "  %-22s %10s\n", "Total", dollars(all))

	reconcileSection(w, r.Reconciliation, all)
	weeksSection(w, r.Weeks)
	attributionSection(w, r.Attribution)

	fmt.Fprintf(w, "\nP19 LARGE WORKING CONTEXT\n")
	fmt.Fprintf(w, "  %s turns in %d sessions sent more than %s tokens of context.\n",
		thousands(ctx.Turns), ctx.Sessions, thousands(ctx.Threshold))
	fmt.Fprintf(w, "  Their input cost %s (%.0f%% of all spend).\n", dollars(ctx.Cost), pct(ctx.Cost, all))
	fmt.Fprintf(w, "  Keeping context under %s tokens would have saved up to %s (%.0f%%).\n",
		thousands(ctx.Threshold), dollars(ctx.Saving), pct(ctx.Saving, all))
	fmt.Fprintln(w, "  Action: start a new session for each task, or compact earlier.")
	sessionTable(w, ctx.Top, "turns over", "peak context")
	growthSection(w, r.Growth, r.CompactFix, all)

	fmt.Fprintf(w, "\nP09 LONG PAUSES (RESUME BLOAT)\n")
	fmt.Fprintf(w, "  %d times in %d sessions, work carried on after a pause of over %s.\n",
		pause.Pauses, pause.Sessions, duration(pause.MinGap))
	fmt.Fprintf(w, "  The cache had expired, so the whole context was written again: %s (%.0f%% of all spend).\n",
		dollars(pause.Cost), pct(pause.Cost, all))
	fmt.Fprintf(w, "  Up to %s of that could be saved.\n", dollars(pause.Saving))
	fmt.Fprintln(w, "  Action: after a long break, compact the session or start a new one before carrying on.")
	sessionTable(w, pause.Top, "pauses", "context at resume")

	toolOutputSection(w, r.Tools, r.ToolFixes, all)
	subAgentSection(w, r.SubAgents, r.SubAgentFix, all)
	standingSection(w, r.Standing, all)
	if len(r.StandingFixes) > 0 {
		fmt.Fprintln(w, "\n  Recommended fixes:")
		for _, f := range r.StandingFixes {
			fmt.Fprintf(w, "\n  %s: up to %s\n", f.Title, money(f.Saving))
			printFixes(w, f)
		}
	}

	fmt.Fprintln(w, "\nSavings are upper bounds: some context is needed, and a compacted or new")
	fmt.Fprintln(w, "session still pays for what it keeps. The findings overlap, so do not add them.")
	return w.Flush()
}

// sessionTable prints the sessions with the largest savings.
func sessionTable(w io.Writer, top []detectors.SessionStat, turnsLabel, peakLabel string) {
	if len(top) == 0 {
		return
	}
	fmt.Fprintf(w, "  Top sessions:\n")
	fmt.Fprintf(w, "    %-10s %-18s %10s  %s\n", "saving", peakLabel, turnsLabel, "session")
	for _, s := range top {
		fmt.Fprintf(w, "    %-10s %-18s %10s  %s\n", dollars(s.Saving), thousands(s.Peak), thousands(s.Turns), printable(s.SessionID))
	}
}

// duration formats d in words: "1 hour", "90 minutes".
func duration(d time.Duration) string {
	switch {
	case d == time.Hour:
		return "1 hour"
	case d%time.Hour == 0:
		return fmt.Sprintf("%d hours", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%d minutes", d/time.Minute)
	}
	return d.String()
}

func pct(part, whole float64) float64 {
	if whole == 0 {
		return 0
	}
	return 100 * part / whole
}

func dollars(v float64) string {
	if v < 0 {
		return "-" + dollars(-v)
	}
	return "$" + thousands2(v)
}

// money is dollars for reading at a glance: whole dollars from $10 up,
// cents below.
func money(v float64) string {
	if v < 0 {
		return "-" + money(-v)
	}
	if v == 0 {
		return "$0"
	}
	if v >= 10 {
		return "$" + thousands(int(v+0.5))
	}
	return dollars(v)
}

// thousands formats n with comma separators: 1234567 -> "1,234,567".
func thousands(n int) string {
	s := fmt.Sprint(n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 && s[i-1] != '-' {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// thousands2 formats v with comma separators and two decimals.
func thousands2(v float64) string {
	cents := int(v*100 + 0.5)
	return fmt.Sprintf("%s.%02d", thousands(cents/100), cents%100)
}

// readLine prints what was read: "Read 3 logs: 2 sessions, 10 turns."
func readLine(w io.Writer, r Read) {
	fmt.Fprintf(w, "Read %s logs: %s sessions, %s turns", thousands(r.Logs), thousands(r.Sessions), thousands(r.Turns))
	if r.Failed > 0 {
		fmt.Fprintf(w, ", %d logs failed", r.Failed)
	}
	if r.BadLines > 0 {
		fmt.Fprintf(w, ", %d lines could not be parsed", r.BadLines)
	}
	fmt.Fprintln(w, ".")
}
