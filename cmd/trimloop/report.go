package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// report prints the cost summary and the findings to out. It writes through a
// bufio.Writer, which keeps the first write error; the error Flush returns is
// the only one that needs checking.
func report(out io.Writer, st stats, sessions []event.Session, f findings) error {
	w := bufio.NewWriter(out)
	ctx, pause := f.context, f.pause
	turns, unpriced := 0, 0
	for _, s := range sessions {
		for _, t := range s.Turns {
			turns++
			if !t.Priced {
				unpriced++
			}
		}
	}
	total := detectors.TotalCost(sessions)
	all := total.Total()

	fmt.Fprintf(w, "Read %d logs: %d sessions, %d turns", st.logs, len(sessions), turns)
	if st.failed > 0 {
		fmt.Fprintf(w, ", %d logs failed", st.failed)
	}
	if st.badLines > 0 {
		fmt.Fprintf(w, ", %d lines could not be parsed", st.badLines)
	}
	fmt.Fprintln(w, ".")
	fmt.Fprintf(w, "Costs are estimates at list prices (%s).", price.Version)
	if unpriced > 0 {
		fmt.Fprintf(w, " %d turns have no price and are left out.", unpriced)
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

	fmt.Fprintf(w, "\nP19 LARGE WORKING CONTEXT\n")
	fmt.Fprintf(w, "  %d turns in %d sessions sent more than %s tokens of context.\n",
		ctx.Turns, ctx.Sessions, thousands(ctx.Threshold))
	fmt.Fprintf(w, "  Their input cost %s (%.0f%% of all spend).\n", dollars(ctx.Cost), pct(ctx.Cost, all))
	fmt.Fprintf(w, "  Keeping context under %s tokens would have saved up to %s (%.0f%%).\n",
		thousands(ctx.Threshold), dollars(ctx.Saving), pct(ctx.Saving, all))
	fmt.Fprintln(w, "  Action: start a new session for each task, or compact earlier.")
	sessionTable(w, ctx.Top, "turns over", "peak context")

	fmt.Fprintf(w, "\nP09 LONG PAUSES (RESUME BLOAT)\n")
	fmt.Fprintf(w, "  %d times in %d sessions, work carried on after a pause of over %s.\n",
		pause.Pauses, pause.Sessions, duration(pause.MinGap))
	fmt.Fprintf(w, "  The cache had expired, so the whole context was written again: %s (%.0f%% of all spend).\n",
		dollars(pause.Cost), pct(pause.Cost, all))
	fmt.Fprintf(w, "  Up to %s of that could be saved.\n", dollars(pause.Saving))
	fmt.Fprintln(w, "  Action: after a long break, compact the session or start a new one before carrying on.")
	sessionTable(w, pause.Top, "pauses", "context at resume")

	toolOutputSection(w, f.tools, f.fixes, all)

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
		fmt.Fprintf(w, "    %-10s %-18s %10d  %s\n", dollars(s.Saving), thousands(s.Peak), s.Turns, printable(s.SessionID))
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

func dollars(v float64) string { return "$" + thousands2(v) }

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
