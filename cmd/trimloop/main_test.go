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
	"github.com/ChrisAdkin8/trimloop/internal/report"
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

// TestToolFixture reads the made-up session in testdata, with tool calls
// logged one content block per line, a re-read, an edit, an output saved to
// disk and a compaction, and checks what fills its context.
func TestToolFixture(t *testing.T) {
	l := source.Log{Path: filepath.Join("..", "..", "testdata", "sessions", "tools.jsonl")}
	s, bad, err := readSession(cc, &l, map[string]bool{})
	if err != nil || bad != 0 {
		t.Fatalf("got err %v, %d bad lines", err, bad)
	}
	if len(s.Turns) != 6 {
		t.Fatalf("got %d turns, want 6 (one per message id)", len(s.Turns))
	}
	if n := len(s.Turns[0].ToolCalls); n != 1 {
		t.Errorf("turn m1: got %d tool calls, want 1 from its second line", n)
	}
	if c := s.Turns[2].ToolCalls[1]; c.Shape != "go test -v" || !c.HasResult || !c.Result.Error || !c.Result.Persisted {
		t.Errorf("go test call: got %+v", c)
	}
	if c := s.Turns[5].Compaction; !s.Turns[5].CompactedBefore || !c.Manual || c.Before != 12_000 || c.After != 2_000 {
		t.Errorf("the last turn should follow a manual compaction from 12k to 2k tokens; got %+v", c)
	}
	if s.Project != "repo" || s.Turns[0].Project != "repo" {
		t.Errorf("project: got %q, want repo (from the cwd)", s.Project)
	}

	f := detectors.FindToolOutput([]event.Session{s}, 500, 10)
	// t1 to t5 are each sent once and carried; the compaction drops them all.
	if f.Calls != 5 || f.FileViews != 3 || f.Rereads != 1 || f.Persisted != 1 {
		t.Errorf("got %d calls, %d views, %d re-reads, %d persisted; want 5, 3, 1, 1",
			f.Calls, f.FileViews, f.Rereads, f.Persisted)
	}
	if f.Carried <= 0 || f.Saving <= 0 || f.Saving >= f.Carried {
		t.Errorf("got carried %v, saving %v; want 0 < saving < carried", f.Carried, f.Saving)
	}
	if len(f.ByShape) != 2 || f.ByShape[0].Name != "sed -n" {
		t.Errorf("by shape: got %+v", f.ByShape)
	}

	var buf bytes.Buffer
	settings := report.DefaultSettings()
	settings.ResultTarget, settings.MinFixSaving = 500, 0
	r := report.Analyse([]event.Session{s}, report.Read{Logs: 1}, settings, t0)
	if err := report.WriteText(&buf, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sed -n", "1 of 3 file views", "Fix (advise)", "CLAUDE.md"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report has no %q", want)
		}
	}
}

// A message copied from an earlier log (a resumed session) brings no tool
// calls: its output was credited to the log it first appeared in.
func TestBuildSessionSkipsCallsOfCopiedMessages(t *testing.T) {
	d := logData{log: source.Log{Path: "/p/s.jsonl"}, msgs: []source.Message{
		{ID: "old", ToolCalls: []event.ToolCall{{ID: "c1", Tool: "Bash"}}},
		{Kind: source.KindToolResults, Results: []source.Result{{CallID: "c1"}}},
		{ID: "new", ToolCalls: []event.ToolCall{{ID: "c2", Tool: "Read"}, {ID: "c2", Tool: "Read"}}},
	}}
	s := buildSession(d, map[string]bool{"old": true})
	if len(s.Turns) != 1 || len(s.Turns[0].ToolCalls) != 1 || s.Turns[0].ToolCalls[0].ID != "c2" {
		t.Errorf("got %+v, want one turn with call c2 once", s.Turns)
	}
}

func TestInheritProjects(t *testing.T) {
	sessions := []event.Session{
		{ID: "m", Kind: "main", Project: "web"},
		{ID: "m/a", Kind: "subagent", ParentID: "m", Turns: []event.Turn{{}, {Project: "own"}}},
		{ID: "m/b", Kind: "subagent", ParentID: "m", Project: "kept"},
		{ID: "x/c", Kind: "subagent", ParentID: "x"}, // parent not read
	}
	inheritProjects(sessions)
	a := sessions[1]
	if a.Project != "web" || a.Turns[0].Project != "web" || a.Turns[1].Project != "own" {
		t.Errorf("a: got %q, turns %q and %q", a.Project, a.Turns[0].Project, a.Turns[1].Project)
	}
	if sessions[2].Project != "kept" || sessions[3].Project != "" {
		t.Errorf("got %q and %q", sessions[2].Project, sessions[3].Project)
	}
}

// The full report runs on the fixture with every finding filled in.
func TestFullReportOnFixture(t *testing.T) {
	l := source.Log{Path: filepath.Join("..", "..", "testdata", "sessions", "tools.jsonl")}
	s, _, err := readSession(cc, &l, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	settings := report.DefaultSettings()
	settings.UncompactedLimit, settings.ContextThreshold, settings.MinFixSaving = 5_000, 2_000, 0
	r := report.Analyse([]event.Session{s}, report.Read{Logs: 1}, settings, t0)
	var buf bytes.Buffer
	if err := report.WriteText(&buf, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"repo", "1 compactions (1 asked for)", "/compact", "No sub-agent runs."} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report has no %q", want)
		}
	}
}

// write sends HTML to a file and prints a summary; an unwritable path fails.
func TestWrite(t *testing.T) {
	r := report.Analyse(nil, report.Read{Logs: 1}, report.DefaultSettings(), t0)
	path := filepath.Join(t.TempDir(), "r.html")
	if err := write(r, "html", path); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err != nil || !strings.Contains(string(b), "<title>Trimloop report</title>") {
		t.Errorf("got err %v, page %.40q", err, b)
	}
	if err := write(r, "json", filepath.Join(t.TempDir(), "missing", "r.json")); err == nil {
		t.Error("got no error writing into a missing folder")
	}
}
