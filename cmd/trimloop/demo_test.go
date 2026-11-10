package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// demoRun runs 'trimloop demo' with args, as main does, with the home
// folder, CLAUDE_CONFIG_DIR, the temporary folder and the working folder
// each an empty scratch folder. It fails the test if the run leaves a file
// in any of them: the demo touches none of the user's files, and removes
// the copy of its logs.
func demoRun(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	// Made before TMPDIR moves, so none is inside another.
	dirs := map[string]string{"HOME": t.TempDir(), "CLAUDE_CONFIG_DIR": t.TempDir(), "TMPDIR": t.TempDir(), "working folder": t.TempDir()}
	for _, v := range []string{"HOME", "CLAUDE_CONFIG_DIR", "TMPDIR"} {
		t.Setenv(v, dirs[v])
	}
	t.Chdir(dirs["working folder"])
	stderr = capture(t, &os.Stderr, func() {
		stdout = captureStdout(t, func() { code = runDemo(args) })
	})
	for name, dir := range dirs {
		if left, _ := filepath.Glob(filepath.Join(dir, "*")); len(left) > 0 {
			t.Errorf("%s: files left after the demo: %v", name, left)
		}
	}
	return code, stdout, stderr
}

// The demo's summary is the same on every machine, date and time zone: its
// logs and clock are fixed, and its dates fall mid-morning UTC.
func TestDemoSummary(t *testing.T) {
	const want = `Read 38 logs: 38 sessions, 604 turns.
Spent $149 at list prices from 5 Oct to 6 Nov 2026 (33 days), about $32 a week.

What to do first (these savings overlap, so do not add them):
  1. Compact long sessions earlier: likely about $9.16 ($7.39–$10.00), up to $39 (26%)
     Compact when a session passes about 200,000 tokens, or between tasks.
  2. Start fresh after a long break: likely about $29, up to $31 (21%)
     After a break of over an hour, start a new session with a short note instead of carrying on.
  3. Run sub-agents on a cheaper model: up to $12 (8%)
     Run sub-agents that search, read and summarise on Sonnet 5.5.

For the full report: --format html (a web page) or --format text.
`
	local := time.Local
	t.Cleanup(func() { time.Local = local })
	for _, zone := range []string{"UTC", "America/Los_Angeles", "Pacific/Auckland"} {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatal(err)
		}
		time.Local = loc
		code, stdout, stderr := demoRun(t)
		if code != 0 || stdout != want {
			t.Errorf("%s: exit code %d, summary:\n%s\nwant:\n%s\nIf a measurement changed, the demo should still lead with these "+
				"three actions: update want, or scripts/gen-demo.py if they changed (ADR 0055).", zone, code, stdout, want)
		}
		if stderr != demoBanner+"\n" {
			t.Errorf("%s: standard error %q, want the banner alone", zone, stderr)
		}
	}
}

// With --format json, standard output holds the report and nothing else;
// the banner goes to standard error. The report is dated by the demo's
// clock, and --since counts back from it.
func TestDemoJSON(t *testing.T) {
	for _, c := range []struct {
		args  []string
		since string
	}{
		{[]string{"--format", "json"}, ""},
		{[]string{"--format", "json", "--out", "-", "--since", "7d"}, "2026-10-31"},
	} {
		code, stdout, stderr := demoRun(t, c.args...)
		var r struct {
			Generated time.Time `json:"generated"`
			Read      struct {
				Logs     int    `json:"logs"`
				BadLines int    `json:"bad_lines"`
				Unpriced int    `json:"unpriced"`
				Since    string `json:"since"`
			} `json:"read"`
		}
		if err := json.Unmarshal([]byte(stdout), &r); err != nil || code != 0 {
			t.Fatalf("%v: exit code %d, standard output is not one JSON report (%v): %.80q", c.args, code, err, stdout)
		}
		if !r.Generated.Equal(time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)) || r.Read.Since != c.since {
			t.Errorf("%v: generated %v, since %q; want the demo's clock and %q", c.args, r.Generated, r.Read.Since, c.since)
		}
		if r.Read.BadLines != 0 || r.Read.Unpriced != 0 {
			t.Errorf("%v: %d bad lines, %d unpriced turns; the demo should have none", c.args, r.Read.BadLines, r.Read.Unpriced)
		}
		if stderr != demoBanner+"\n" {
			t.Errorf("%v: standard error %q, want the banner alone", c.args, stderr)
		}
	}
}

// The demo never reads or writes the history file, so its options are
// refused, in one line, before anything is read.
func TestDemoRefusesHistoryOptions(t *testing.T) {
	for _, args := range [][]string{{"--history", "x"}, {"--no-history"}} {
		code, stdout, stderr := demoRun(t, args...)
		first, _, _ := strings.Cut(stderr, "\n")
		if code != 2 || stdout != "" || !strings.Contains(first, "does not go with demo") {
			t.Errorf("%v: exit code %d, standard output %q, standard error %q; want 2 and the reason", args, code, stdout, stderr)
		}
	}
}

// The share format holds figures and trimloop's own words only: nothing
// the report took as text from the logs (project names, session ids and
// titles) may reach it.
func TestDemoShare(t *testing.T) {
	buildVersion = "v9.9.9"
	defer func() { buildVersion = "" }()
	const want = `Claude Code usage over 33 days: $149 at list prices, about $32 a week (38 sessions, 604 turns).
39% of the spend went on sending turns over 200k tokens.

What would save the most (the savings overlap, so they are not added up):
  1. Compact long sessions earlier: likely about $9.16 ($7.39–$10.00), up to $39 (26%)
  2. Start fresh after a long break: likely about $29, up to $31 (21%)
  3. Run sub-agents on a cheaper model: up to $12 (8%)

Worked out by trimloop v9.9.9 from local logs: https://github.com/ChrisAdkin8/trimloop
`
	code, shared, _ := demoRun(t, "--format", "share")
	if code != 0 || shared != want {
		t.Fatalf("exit code %d, share:\n%s\nwant:\n%s", code, shared, want)
	}

	_, full, _ := demoRun(t, "--format", "json", "--out", "-")
	var r struct {
		Attribution struct {
			ByProject []struct {
				Name string `json:"name"`
			} `json:"by_project"`
		} `json:"attribution"`
		Labels map[string]struct {
			Title string `json:"title"`
		} `json:"labels"`
	}
	if err := json.Unmarshal([]byte(full), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Attribution.ByProject) == 0 || len(r.Labels) == 0 {
		t.Fatal("the demo report names no projects or sessions: nothing to check the share format against")
	}
	for _, p := range r.Attribution.ByProject {
		if strings.Contains(shared, p.Name) {
			t.Errorf("the share format names the project %q", p.Name)
		}
	}
	for id, l := range r.Labels {
		if strings.Contains(shared, id) || l.Title != "" && strings.Contains(shared, l.Title) {
			t.Errorf("the share format names the session %s (%q)", id, l.Title)
		}
	}
}
