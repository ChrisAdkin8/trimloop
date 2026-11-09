package main

import (
	"fmt"
	"io"
	"time"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source/claudecode"
	"github.com/ChrisAdkin8/trimloop/internal/history"
	"github.com/ChrisAdkin8/trimloop/internal/live"
	"github.com/ChrisAdkin8/trimloop/internal/report"
)

// checkSetup prints where each part of the live nudges stands, and what to
// do about any that is not working, for 'trimloop setup --check'. It changes
// nothing. It returns 0 when everything is set up, 1 when something needs
// doing, and 2 when the settings file cannot be read.
func checkSetup(w io.Writer, settings, exe, home, cfgDir string) int {
	c, err := live.CheckSettings(settings, exe)
	if err != nil {
		fmt.Fprintln(w, err)
		return 2
	}
	fmt.Fprintf(w, "Claude Code settings: %s\n", settings)
	fmt.Fprintf(w, "  status line: %s%s\n", c.StatusLine, advice(c.StatusLine, "status line"))
	for _, h := range []struct{ event, what string }{
		{"Stop", "hook after each reply"},
		{"UserPromptSubmit", "hook before each message"},
	} {
		s := c.Hooks[h.event]
		fmt.Fprintf(w, "  %s (%s): %s%s\n", h.what, h.event, s, advice(s, "hook"))
	}

	path := history.LimitsPath(home, cfgDir)
	readings, err := history.LoadLimits(path)
	plan := claudecode.Reader{Home: home, ConfigDir: cfgDir}.Plan()
	switch {
	case err != nil:
		fmt.Fprintf(w, "Usage-limit readings: cannot read %s: %v\n", path, err)
	case len(readings) > 0:
		last := readings[len(readings)-1].At
		fmt.Fprintf(w, "Usage-limit readings: %d kept, the latest %s ago (%s)\n",
			len(readings), time.Since(last).Round(time.Minute), path)
	case plan.Subscription || plan.Name != "":
		fmt.Fprintln(w, "Usage-limit readings: none yet. Claude Code sends them to the status line on Pro and Max")
		fmt.Fprintln(w, "  plans, after a session's first reply, wherever the status line is shown (the terminal).")
	default:
		fmt.Fprintln(w, "Usage-limit readings: none (only Pro and Max plans have usage limits).")
	}
	days, set := claudecode.Reader{Home: home, ConfigDir: cfgDir}.Retention()
	if tip := report.RetentionTip(report.Retention{Days: days, Set: set}); tip != "" {
		fmt.Fprintln(w, "Log retention: "+tip)
	}
	if !c.OK() {
		return 1
	}
	return 0
}

// advice says what to do about a settings entry that is not up to date.
func advice(s live.EntryState, what string) string {
	switch s {
	case live.Missing, live.Stale:
		return "; run 'trimloop setup --write <settings file>' to fix it"
	case live.Other:
		return "; trimloop's " + what + " is not shown ('setup --write <file> --replace-statusline' replaces it)"
	}
	return ""
}
