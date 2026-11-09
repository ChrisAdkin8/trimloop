package main

import (
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
		"v0.1.1-0.20261109101144-c0daee340df5":     true,
		"v0.0.0-20261108192342-6a1a90719b1d+dirty": true,
		"v0.1.0":      false,
		"v1.2.3-rc.1": false,
	} {
		if got := pseudoVersion.MatchString(v); got != want {
			t.Errorf("pseudoVersion(%q) = %v, want %v", v, got, want)
		}
	}
}
