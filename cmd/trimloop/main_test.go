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
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("report file mode: got %v, %v; want 0600", info.Mode().Perm(), err)
	}
	if b, err := os.ReadFile(path); err != nil || !strings.Contains(string(b), "<title>Trimloop report</title>") {
		t.Errorf("got err %v, page %.40q", err, b)
	}
	if err := write(r, "json", filepath.Join(t.TempDir(), "missing", "r.json")); err == nil {
		t.Error("got no error writing into a missing folder")
	}
}

// Injected text arrives with the next turn; the latest system prompt and
// tool list apply to every later turn; each run's latest cost record counts.
func TestBuildSessionContextAndRecords(t *testing.T) {
	st := &event.Standing{SystemBytes: 100}
	rec := func(run string, cost float64, at time.Duration) source.Message {
		return source.Message{Kind: source.KindCostRecord, RunID: run, Record: event.HarnessRecord{Cost: cost,
			ByModel: map[string]event.ModelRecord{"m": {Cost: cost}}, At: t0.Add(at)}}
	}
	d := logData{log: source.Log{Path: "/p/s.jsonl"}, msgs: []source.Message{
		{Kind: source.KindStanding, Standing: st},
		{Kind: source.KindInjected, Injected: []event.Injected{{Kind: "skill_listing", Bytes: 40}}},
		{ID: "a"},
		rec("r1", 1, time.Minute),
		rec("r1", 3, 2*time.Minute), // running total for the same run
		{ID: "b"},
		rec("r2", 2, time.Hour), // a second run (resumed)
	}}
	s := buildSession(d, map[string]bool{})
	if len(s.Turns) != 2 || len(s.Turns[0].Injected) != 1 || len(s.Turns[1].Injected) != 0 {
		t.Fatalf("injected: got %+v", s.Turns)
	}
	if s.Turns[0].Standing != st || s.Turns[1].Standing != st {
		t.Error("both turns should carry the recorded system prompt")
	}
	if s.Harness == nil || s.Harness.Cost != 5 || s.Harness.ByModel["m"].Cost != 5 || !s.Harness.At.Equal(t0.Add(time.Hour)) {
		t.Errorf("harness record: got %+v", s.Harness)
	}
}

// A record of the system prompt without a tool list keeps the last tool list.
func TestBuildSessionKeepsToolsWhenUnrecorded(t *testing.T) {
	tools := []event.ToolDef{{Name: "Bash", Bytes: 9}}
	d := logData{log: source.Log{Path: "/p/s.jsonl"}, msgs: []source.Message{
		{Kind: source.KindStanding, Standing: &event.Standing{SystemBytes: 1, Tools: tools}},
		{Kind: source.KindStanding, Standing: &event.Standing{SystemBytes: 2}},
		{ID: "a"},
	}}
	st := buildSession(d, map[string]bool{}).Turns[0].Standing
	if st.SystemBytes != 2 || len(st.Tools) != 1 {
		t.Errorf("got %+v, want the new system prompt with the earlier tools", st)
	}
}

// A resumed log starts at the same moment as the original (it copies the
// original's messages): the one that ended first is the original.
func TestOldestFirstTieGoesToTheOneThatEndedFirst(t *testing.T) {
	data := []logData{
		{log: source.Log{Path: "a-resumed"}, start: t0, end: t0.Add(5 * time.Hour)},
		{log: source.Log{Path: "z-original"}, start: t0, end: t0.Add(time.Hour)},
	}
	oldestFirst(data)
	if data[0].log.Path != "z-original" {
		t.Errorf("got %s first, want the original", data[0].log.Path)
	}
}

// Injected text and compaction markers copied into a resumed log are counted
// once; replies the harness made up are not turns.
func TestBuildSessionCountsCopiedLinesOnce(t *testing.T) {
	seen := map[string]bool{}
	skills := source.Message{Kind: source.KindInjected, LineID: "u1", Injected: []event.Injected{{Kind: "skill_listing", Bytes: 40}}}
	compact := source.Message{Kind: source.KindCompaction, LineID: "u2"}
	first := buildSession(logData{log: source.Log{Path: "/p/a.jsonl"}, msgs: []source.Message{skills, compact, {ID: "a"}}}, seen)
	if len(first.Turns[0].Injected) != 1 || !first.Turns[0].CompactedBefore {
		t.Fatalf("original: got %+v", first.Turns[0])
	}
	second := buildSession(logData{log: source.Log{Path: "/p/b.jsonl"}, msgs: []source.Message{skills, compact, {ID: "b"},
		{ID: "c", Model: "<synthetic>"}}}, seen)
	if len(second.Turns) != 1 || len(second.Turns[0].Injected) != 0 || second.Turns[0].CompactedBefore {
		t.Errorf("resumed: got %+v, want one turn with nothing copied counted again", second.Turns)
	}
}

// An early line of a reply can carry a partial count: the fullest one counts.
func TestBuildSessionKeepsFullestUsage(t *testing.T) {
	d := logData{log: source.Log{Path: "/p/s.jsonl"}, msgs: []source.Message{
		{ID: "a", Model: "claude-opus-5-5", Usage: event.Usage{CacheRead: 100, Output: 4}},
		{ID: "a", Model: "claude-opus-5-5", Usage: event.Usage{CacheRead: 100, Output: 450}},
	}}
	s := buildSession(d, map[string]bool{})
	if len(s.Turns) != 1 || s.Turns[0].Usage.Output != 450 || s.Turns[0].Cost.Output == 0 {
		t.Errorf("got %+v", s.Turns)
	}
}

// Claude Code's cost records carry no time: each is stamped with the last
// turn logged before it, so a session that went on afterwards is seen.
func TestBuildSessionStampsCostRecords(t *testing.T) {
	d := logData{log: source.Log{Path: "/p/s.jsonl"}, msgs: []source.Message{
		{ID: "a", Time: t0},
		{Kind: source.KindCostRecord, RunID: "r", Record: event.HarnessRecord{Cost: 1, Start: t0.Add(-time.Minute)}},
		{ID: "b", Time: t0.Add(time.Hour)},
	}}
	s := buildSession(d, map[string]bool{})
	if !s.Harness.At.Equal(t0) || !s.Harness.Start.Equal(t0.Add(-time.Minute)) {
		t.Errorf("got At %v, Start %v", s.Harness.At, s.Harness.Start)
	}
}

// Writing over an existing, readable report makes it private.
func TestWriteMakesExistingReportPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := report.Analyse(nil, report.Read{}, report.DefaultSettings(), t0)
	if err := write(r, "json", path); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("got %v, want 0600", info.Mode().Perm())
	}
}
