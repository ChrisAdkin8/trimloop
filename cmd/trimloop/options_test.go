package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 11, 8, 15, 30, 0, 0, time.UTC)
	day := func(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }
	for _, c := range []struct {
		in   string
		want time.Time
	}{
		{"2026-11-01", day(1)},
		{"7d", day(1)},
		{"1w", day(1)},
		{"0d", day(8)}, // today, from midnight
	} {
		got, err := parseSince(c.in, now)
		if err != nil || !got.Equal(c.want) {
			t.Errorf("parseSince(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "d", "7", "7m", "-1d", "1 Nov", "2026-13-01"} {
		if _, err := parseSince(bad, now); err == nil {
			t.Errorf("parseSince(%q): want an error", bad)
		}
	}
}

func TestStartedSince(t *testing.T) {
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	at := func(d int) []event.Turn { return []event.Turn{{Time: time.Date(2026, 11, d, 9, 0, 0, 0, time.UTC)}} }
	sessions := []event.Session{
		{ID: "old", Kind: "main", Turns: at(0)}, // day 0 of November is 31 October
		{ID: "old/a", Kind: "subagent", ParentID: "old", Turns: at(2)},
		{ID: "new", Kind: "main", Turns: at(1)},
		{ID: "new/a", Kind: "subagent", ParentID: "new", Turns: at(3)},
		{ID: "orphan-old/a", Kind: "subagent", ParentID: "gone", Turns: at(0)},
		{ID: "orphan-new/a", Kind: "subagent", ParentID: "gone", Turns: at(5)},
	}
	var got []string
	for _, s := range startedSince(sessions, since) {
		got = append(got, s.ID)
	}
	// A sub-agent goes with its main session, whatever its own start.
	want := []string{"new", "new/a", "orphan-new/a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("startedSince: got %v, want %v", got, want)
	}
}

func TestVersion(t *testing.T) {
	if v := version(); v == "" {
		t.Error("version is empty")
	}
	buildVersion = "v9.9.9"
	defer func() { buildVersion = "" }()
	if v := version(); v != "v9.9.9" {
		t.Errorf("version with buildVersion set: got %q", v)
	}
}

func TestNoLogsMessageNamesTheFolder(t *testing.T) {
	msg := noLogsMessage("/x/.claude/projects")
	if !strings.Contains(msg, "/x/.claude/projects") || !strings.Contains(msg, "CLAUDE_CONFIG_DIR") {
		t.Errorf("message: %q", msg)
	}
}

func TestStartedSinceKeepsHitOnlySessions(t *testing.T) {
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	hit := func(d int) []event.LimitHit {
		return []event.LimitHit{{At: time.Date(2026, 11, d, 9, 0, 0, 0, time.UTC)}}
	}
	sessions := []event.Session{
		{ID: "old", Kind: event.MainSession, LimitHits: hit(0)},
		{ID: "new", Kind: event.MainSession, LimitHits: hit(3)},
	}
	got := startedSince(sessions, since)
	if len(got) != 1 || got[0].ID != "new" {
		t.Errorf("got %+v; want the hit-only session from 3 November", got)
	}
}

func TestPseudoVersion(t *testing.T) {
	for v, want := range map[string]bool{
		"v0.1.1-0.20261109101144-c0daee340df5":      true,
		"v0.0.0-20261108192342-6a1a90719b1d+dirty":  true,
		"v0.2.0-rc.1.0.20261109101144-c0daee340df5": true,
		"v0.1.0":      false,
		"v1.2.3-rc.1": false,
	} {
		if got := pseudoVersion.MatchString(v); got != want {
			t.Errorf("pseudoVersion(%q) = %v, want %v", v, got, want)
		}
	}
	if m := pseudoVersion.FindStringSubmatch("v0.2.0-rc.1.0.20261109101144-c0daee340df5"); m == nil || m[1] != "c0daee340df5" {
		t.Errorf("commit: got %q", m)
	}
}

func TestShellArg(t *testing.T) {
	for in, want := range map[string]string{
		"/Users/a/.claude/settings.json":   "/Users/a/.claude/settings.json",
		"/Users/a/My Claude/settings.json": "'/Users/a/My Claude/settings.json'",
		"/tmp/it's/settings.json":          `'/tmp/it'\''s/settings.json'`,
	} {
		if got := shellArg(in); got != want {
			t.Errorf("shellArg(%q) = %s, want %s", in, got, want)
		}
	}
}

// Mistakes on the command line are named in one line before any log is
// read, with the valid choices.
func TestCheckOptions(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	in := func(name string) string { return filepath.Join(dir, name) }
	for _, c := range []struct {
		name string
		o    cliOptions
		want string // "" for no error
	}{
		{"defaults", cliOptions{format: "summary"}, ""},
		{"unknown command", cliOptions{args: []string{"report"}, first: true, format: "summary"},
			`unknown command "report": run "trimloop" to analyse your logs, or "trimloop setup"`},
		{"word after options", cliOptions{args: []string{"extra"}, format: "text"}, `unexpected "extra"`},
		{"unknown format", cliOptions{format: "txt"}, `unknown format "txt": use summary, text, html or json`},
		{"open needs html", cliOptions{format: "json", open: true}, "--open opens the web page, so it needs --format html"},
		{"missing folder", cliOptions{format: "html", out: "nodir/x.html"}, "the folder nodir does not exist"},
		{"folder is a file", cliOptions{format: "html", out: file + "/x.html"}, "is a file, not a folder"},
		{"out is a folder", cliOptions{format: "html", out: dir}, "is a folder: give a file name"},
		{"open writes html to .json", cliOptions{format: "html", open: true, out: in("x.json")},
			"ends in .json, but the report would be written as html. Use a file name ending in .html, or leave out --open and add --format json"},
		{"json to .html", cliOptions{format: "json", out: in("x.html")},
			"ends in .html, but the report would be written as json. Use a file name ending in .json, or add --format html"},
		{"summary to .json", cliOptions{format: "summary", out: in("x.json")}, "add --format json"},
		{"text to .htm", cliOptions{format: "text", out: in("x.HTM")}, "add --format html"},
		{"html to .html", cliOptions{format: "html", open: true, out: in("x.html")}, ""},
		{"summary to .txt", cliOptions{format: "summary", out: in("x.txt")}, ""},
		{"text to .txt", cliOptions{format: "text", out: in("x.txt")}, ""},
		{"json to .json", cliOptions{format: "json", out: in("x.json")}, ""},
		{"no extension", cliOptions{format: "json", out: in("report")}, ""},
		{"other extension", cliOptions{format: "text", out: in("report.md")}, ""},
	} {
		err := checkOptions(c.o)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: got %v, want no error", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}
