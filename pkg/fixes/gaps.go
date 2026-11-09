package fixes

import (
	"fmt"
	"strings"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
)

// ForModelSwitches recommends choosing the model at the start of a session
// (P01 from model switches, ADR 0050), or returns false below minSaving.
func ForModelSwitches(f detectors.ModelSwitches, minSaving float64) (Recommendation, bool) {
	if f.Saving < minSaving {
		return Recommendation{}, false
	}
	return Recommendation{
		Pattern: "P01",
		Title:   "Pick the model when a session starts",
		Finding: fmt.Sprintf("%d times, in %s, a session moved to another model part-way through. The saved copy "+
			"of the conversation (the cache) belongs to one model, so the first turn on the new one paid to save "+
			"the whole conversation again.", f.Switches, plural(f.Sessions, "session")),
		Saving: f.Saving,
		Primary: Fix{Advise,
			"Choose the model when you start a session, and keep it.",
			"Use /model at the start. To change model mid-way, run /compact first, so there is less to save again; " +
				"or start a new session with a short note of where things stand."},
		Check: "After a week, the model-switch figure should fall.",
	}, true
}

// ForFailures recommends keeping the computer awake while Claude works,
// when it slept mid-response at least three times (P12 from failed
// requests, ADR 0050), or returns false.
func ForFailures(f detectors.Failures, minSaving float64) (Recommendation, bool) {
	if f.Sleep < 3 || f.SleepResent < minSaving {
		return Recommendation{}, false
	}
	return Recommendation{
		Pattern: "P12",
		Title:   "Keep the computer awake while Claude works",
		Finding: fmt.Sprintf("%d times the computer went to sleep while Claude was replying, in sessions of about "+
			"%s tokens. Each reply was cut off and the request sent again; the cut-off part is not in the logs, so "+
			"the saving counts only the conversation read again. After a long sleep the cache has often expired "+
			"too, and the next turn saves the whole conversation again; the long-break card counts that, so it "+
			"is not added here.", f.Sleep, thousandsInt(f.MedianContext)),
		Saving: f.SleepResent,
		Primary: Fix{Advise,
			"Keep the computer awake during long tasks.",
			"In the Claude desktop app, turn on keeping the computer awake while Claude works. In a terminal on " +
				"macOS, run caffeinate -i alongside, or start Claude Code with caffeinate -i claude."},
		Check: "After a week, the count of replies cut off by sleep should fall.",
	}, true
}

// ForAppServers recommends the terminal app for work that does not need the
// MCP servers the desktop app adds, when some were never called (P06,
// ADRs 0050, 0052), or returns false below minSaving.
func ForAppServers(parts []detectors.ContextPart, minSaving float64) (Recommendation, bool) {
	cost := 0.0
	var names []string
	for _, p := range parts {
		cost += p.Cost
		if len(names) < 3 {
			names = append(names, connectorName(strings.TrimPrefix(p.Name, "mcp__")))
		}
	}
	if cost < minSaving {
		return Recommendation{}, false
	}
	return Recommendation{
		Pattern: "P06",
		Title:   "Use the terminal app when you don't need the desktop app's tools",
		Finding: fmt.Sprintf("The desktop app adds its own MCP servers, whose tool definitions are sent with "+
			"every request. Some were never used in this period (%s), and still cost %s.",
			strings.Join(names, ", "), dollars(cost)),
		Saving:   cost,
		Examples: names,
		Primary: Fix{Source,
			"For work that doesn't need them, run Claude Code in a terminal (claude), which doesn't load them.",
			"These servers come with the desktop app, so /mcp and claude mcp remove do not list them. Sessions " +
				"started from the terminal leave them out."},
		Check: "After a week, the cost of these tool definitions should fall.",
	}, true
}
