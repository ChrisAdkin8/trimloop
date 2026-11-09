package report

import (
	"fmt"
	"io"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/fixes"
)

// cacheLifeSection prints P02: caches that expired during waits of 5
// minutes to an hour.
func cacheLifeSection(w io.Writer, c detectors.CacheLife, rec *fixes.Recommendation, all float64) {
	fmt.Fprintln(w, "\nP02 CACHE EXPIRING DURING SHORT WAITS")
	if c.Waits == 0 {
		fmt.Fprintln(w, "  No session on the 5-minute cache waited long enough to lose it.")
		fmt.Fprintf(w, "  In these logs %s and %s used the 5-minute cache; %s and %s the hour's.\n",
			count(c.MainShort, "main session"), count(c.SubShort, "sub-agent"), count(c.MainHour, "main session"), count(c.SubHour, "sub-agent"))
		return
	}
	fmt.Fprintf(w, "  %s on the 5-minute cache (%d in sub-agents) waited 5 minutes to an hour, so the\n",
		count(c.Waits, "turn"), c.SubAgent)
	fmt.Fprintf(w, "  whole history was saved again: those turns cost %s, %s more than with the cache still\n",
		dollars(c.Cost), dollars(c.Saving))
	fmt.Fprintf(w, "  there (%.0f%% of all spend).\n", pct(c.Saving, all))
	fmt.Fprintf(w, "  In these logs %s and %s used the 5-minute cache; %s and %s the hour's.\n",
		count(c.MainShort, "main session"), count(c.SubShort, "sub-agent"), count(c.MainHour, "main session"), count(c.SubHour, "sub-agent"))
	if c.HourNet > 0 {
		fmt.Fprintf(w, "  Putting all of them on the hour's cache would have saved about %s, net of its dearer writes.\n", dollars(c.HourNet))
	} else {
		fmt.Fprintf(w, "  Putting all of them on the hour's cache would have cost %s more: its writes are dearer.\n", dollars(-c.HourNet))
	}
	sessionTable(w, c.Top, "waits", "context")
	if rec != nil {
		fmt.Fprintf(w, "\n  %s: %s\n", rec.Title, savingText(*rec, all))
		printFixes(w, *rec)
	}
}

// mainModelSection prints P13 for main sessions: spend by model and the
// newer or lower-tier model for each.
func mainModelSection(w io.Writer, m detectors.MainModels, recs []fixes.Recommendation, all float64) {
	fmt.Fprintln(w, "\nP13 MAIN-SESSION MODELS")
	if len(m.ByModel) == 0 {
		fmt.Fprintln(w, "  No main-session turns.")
		return
	}
	fmt.Fprintf(w, "    %-10s %6s %8s  %-18s %10s  %-18s %10s\n", "cost", "share", "turns", "model", "newer saves", "tier down", "saves")
	for _, g := range m.ByModel {
		newer, tier := "", ""
		if g.Newer != "" {
			newer = printable(modelName(g.Newer))
		}
		if g.Tier != "" {
			tier = printable(modelName(g.Tier))
		}
		fmt.Fprintf(w, "    %-10s %5.1f%% %8s  %-18s %10s  %-18s %10s\n", dollars(g.Cost), pct(g.Cost, all), thousands(g.Turns),
			printable(modelName(g.Model)), orDash(newer, g.NewerSave), tier, orDash(tier, g.TierSave))
	}
	for _, rec := range recs {
		fmt.Fprintf(w, "\n  %s: %s\n", rec.Title, savingText(rec, all))
		printFixes(w, rec)
	}
}

// effortSection prints P14: spend by effort level.
func effortSection(w io.Writer, e detectors.Effort, rec *fixes.Recommendation, all float64) {
	fmt.Fprintln(w, "\nP14 EFFORT LEVEL")
	if len(e.ByLevel) == 0 {
		fmt.Fprintln(w, "  No turns.")
		return
	}
	perTurn := "thinking a turn"
	if !e.Thinking {
		perTurn = "output a turn"
	}
	fmt.Fprintf(w, "    %-10s %8s %10s %12s %16s\n", "level", "turns", "spend", "output cost", perTurn)
	for _, g := range e.ByLevel {
		level := printable(g.Level)
		if level == "" {
			level = "not logged"
		}
		think := ""
		switch {
		case e.Thinking && g.ThinkN > 0:
			think = thousands(g.Think / g.ThinkN)
		case !e.Thinking && g.Turns > 0:
			think = thousands(g.Output / g.Turns)
		}
		fmt.Fprintf(w, "    %-10s %8s %10s %12s %16s\n", level, thousands(g.Turns), dollars(g.Cost), dollars(g.OutCost), think)
	}
	fmt.Fprintln(w, "  Levels mix models (each thinks a different amount); the saving compares each model")
	fmt.Fprintln(w, "  with its own high-effort turns.")
	if rec != nil {
		fmt.Fprintf(w, "\n  %s: %s\n", rec.Title, savingText(*rec, all))
		printFixes(w, *rec)
	}
}

// orDash writes a saving, or "" when there is no model to move to.
func orDash(model string, saving float64) string {
	if model == "" {
		return ""
	}
	return dollars(saving)
}
