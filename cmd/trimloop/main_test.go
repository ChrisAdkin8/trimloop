package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source"
	"github.com/ChrisAdkin8/trimloop/internal/adapters/source/claudecode"
	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

var t0 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

// line returns one Claude Code transcript line for an assistant message with
// usage, logged at t0 plus at. An empty id leaves the id out.
func line(id string, out int, at time.Duration) string {
	idField := ""
	if id != "" {
		idField = fmt.Sprintf(`"id":%q,`, id)
	}
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{%s"model":"claude-opus-5-5","usage":{"output_tokens":%d}}}`+"\n",
		t0.Add(at).Format(time.RFC3339Nano), idField, out)
}

func writeLog(t *testing.T, name, content string) *source.Log {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return &source.Log{Path: p}
}

var cc = claudecode.Reader{}

// readSession reads one log and builds its session, as main does for each log.
func readSession(r source.Reader, l *source.Log, seen map[string]bool) (event.Session, int, error) {
	d := readLog(r, *l)
	return buildSession(d, seen), d.bad, d.err
}

// TestReadSessionCountsEachIDOnce checks the ADR 0026 rule: each message id
// is counted once across all logs, credited to the first log that has it.
func TestReadSessionCountsEachIDOnce(t *testing.T) {
	first := writeLog(t, "first.jsonl",
		line("a", 1, 0)+
			line("a", 1, 0)+ // repeat inside one log (one line per content block)
			line("b", 10, time.Minute))
	second := writeLog(t, "second.jsonl",
		line("b", 10, time.Minute)+ // repeat across logs (resumed session)
			line("c", 100, 2*time.Minute))

	seen := map[string]bool{}
	for _, tt := range []struct {
		log        *source.Log
		wantTurns  int
		wantOutput int
	}{
		{first, 2, 11},   // a once, b once
		{second, 1, 100}, // b already counted by first; only c
	} {
		s, _, err := readSession(cc, tt.log, seen)
		if err != nil {
			t.Fatal(err)
		}
		out := 0
		for _, turn := range s.Turns {
			out += turn.Usage.Output
		}
		if len(s.Turns) != tt.wantTurns || out != tt.wantOutput {
			t.Errorf("%s: got %d turns, output %d; want %d turns, output %d",
				filepath.Base(tt.log.Path), len(s.Turns), out, tt.wantTurns, tt.wantOutput)
		}
	}
}

// A message copied into a newer log is credited to the oldest log that has it,
// whatever order the logs are found in.
func TestOldestLogGetsTheCredit(t *testing.T) {
	newer := writeLog(t, "a-newer.jsonl", // sorts first by name
		line("m1", 1, 0)+line("m2", 1, 3*time.Hour))
	older := writeLog(t, "z-older.jsonl",
		line("m0", 1, -time.Hour)+line("m1", 1, 0))

	data := []logData{readLog(cc, *newer), readLog(cc, *older)}
	oldestFirst(data)
	if data[0].log.Path != older.Path {
		t.Fatalf("first log: got %s, want the older one", filepath.Base(data[0].log.Path))
	}

	seen := map[string]bool{}
	ids := func(s event.Session) (out []string) {
		for _, turn := range s.Turns {
			out = append(out, turn.ID)
		}
		return out
	}
	if got := ids(buildSession(data[0], seen)); strings.Join(got, ",") != "m0,m1" {
		t.Errorf("older log: got %v, want [m0 m1]", got)
	}
	if got := ids(buildSession(data[1], seen)); strings.Join(got, ",") != "m2" {
		t.Errorf("newer log: got %v, want [m2]", got)
	}
}

func TestOldestFirstOrder(t *testing.T) {
	data := []logData{
		{log: source.Log{Path: "no-time-b"}},
		{log: source.Log{Path: "late"}, start: t0.Add(time.Hour)},
		{log: source.Log{Path: "no-time-a"}},
		{log: source.Log{Path: "tie-b"}, start: t0},
		{log: source.Log{Path: "tie-a"}, start: t0},
	}
	oldestFirst(data)
	var got []string
	for _, d := range data {
		got = append(got, d.log.Path)
	}
	want := "tie-a,tie-b,late,no-time-a,no-time-b"
	if strings.Join(got, ",") != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

func TestReadLogStartIgnoresMissingTimes(t *testing.T) {
	l := writeLog(t, "s.jsonl",
		`{"type":"assistant","message":{"id":"x","usage":{"output_tokens":1}}}`+"\n"+ // no timestamp
			line("a", 1, time.Hour)+line("b", 1, 30*time.Minute))
	d := readLog(cc, *l)
	if want := t0.Add(30 * time.Minute); !d.start.Equal(want) {
		t.Errorf("start: got %v, want %v", d.start, want)
	}
}

func TestReadSessionIdleTime(t *testing.T) {
	l := writeLog(t, "s.jsonl",
		line("a", 1, 0)+
			line("a", 1, 30*time.Second)+ // a's last line: activity until 0:30
			line("b", 1, 2*time.Hour)) // idle 2h minus 30s
	s, _, err := readSession(cc, l, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(s.Turns))
	}
	if !s.Turns[0].First || s.Turns[0].IdleBefore != 0 {
		t.Errorf("turn 0: got first=%v idle=%v, want first, idle 0", s.Turns[0].First, s.Turns[0].IdleBefore)
	}
	if want := 2*time.Hour - 30*time.Second; s.Turns[1].First || s.Turns[1].IdleBefore != want {
		t.Errorf("turn 1: got first=%v idle=%v, want not first, idle %v", s.Turns[1].First, s.Turns[1].IdleBefore, want)
	}
}

// A resumed session copies earlier messages into its new log. They are not
// counted again, but they still mark activity, so the pause before the first
// new turn is measured from them.
func TestReadSessionResumeMeasuresPauseFromCopiedMessages(t *testing.T) {
	seen := map[string]bool{"a": true} // already counted in the original log
	l := writeLog(t, "resumed.jsonl", line("a", 1, 0)+line("b", 1, 3*time.Hour))
	s, _, err := readSession(cc, l, seen)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 || s.Turns[0].First || s.Turns[0].IdleBefore != 3*time.Hour {
		t.Errorf("got %+v, want one turn, not first, idle 3h", s.Turns)
	}
}

func TestReadSessionPricesTurns(t *testing.T) {
	l := writeLog(t, "s.jsonl", line("a", 1_000_000, 0))
	s, _, err := readSession(cc, l, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Turns[0].Priced || s.Turns[0].Cost.Output != 20 { // Opus 5.5 output: $20 per million
		t.Errorf("got priced=%v cost=%+v, want $20 output", s.Turns[0].Priced, s.Turns[0].Cost)
	}
}

func TestReadSessionCountsMessagesWithoutID(t *testing.T) {
	l := writeLog(t, "s.jsonl", line("", 1, 0)+line("", 2, time.Second))
	s, _, err := readSession(cc, l, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 2 {
		t.Errorf("got %d turns, want 2", len(s.Turns))
	}
}

func TestReadSessionCountsBadLines(t *testing.T) {
	l := writeLog(t, "s.jsonl", "not json\n"+line("a", 1, 0))
	s, bad, err := readSession(cc, l, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if bad != 1 || len(s.Turns) != 1 {
		t.Errorf("got %d bad lines, %d turns; want 1 and 1", bad, len(s.Turns))
	}
}

func TestReadSessionMissingFile(t *testing.T) {
	l := &source.Log{Path: filepath.Join(t.TempDir(), "gone.jsonl")}
	if _, _, err := readSession(cc, l, map[string]bool{}); err == nil {
		t.Error("got no error for a missing file")
	}
}

func TestSessionFor(t *testing.T) {
	main := sessionFor("/p/proj/abc.jsonl")
	if main.ID != "abc" || main.Kind != "main" {
		t.Errorf("main: got %+v", main)
	}
	sub := sessionFor("/p/proj/abc/subagents/agent-x.jsonl")
	if sub.ID != "abc/agent-x" || sub.Kind != "subagent" || sub.ParentID != "abc" {
		t.Errorf("subagent: got %+v", sub)
	}
}

func TestThousands(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567", -1234: "-1,234"} {
		if got := thousands(n); got != want {
			t.Errorf("thousands(%d) = %q, want %q", n, got, want)
		}
	}
	if got := dollars(1234.5); got != "$1,234.50" {
		t.Errorf("dollars(1234.5) = %q, want $1,234.50", got)
	}
}

func TestDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		time.Hour: "1 hour", 2 * time.Hour: "2 hours", 90 * time.Minute: "90 minutes", 90 * time.Second: "1m30s",
	} {
		if got := duration(d); got != want {
			t.Errorf("duration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestReportRuns(t *testing.T) {
	var buf bytes.Buffer
	report(&buf, stats{logs: 1}, nil, detectors.ContextSize{}, detectors.LongPause{})
	for _, want := range []string{"COST BY TOKEN CLASS", "P19 ", "P09 "} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report has no %q", want)
		}
	}
}
