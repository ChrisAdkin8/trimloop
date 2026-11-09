package sessions

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// readSession reads one log and builds its session, as Load does for each log.
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
	main := sessionFor(source.Log{Path: "/p/proj/abc.jsonl", Session: "abc"})
	if main.ID != "abc" || main.Kind != "main" {
		t.Errorf("main: got %+v", main)
	}
	sub := sessionFor(source.Log{Path: "/p/proj/abc/subagents/agent-x.jsonl", Session: "abc/agent-x", Parent: "abc"})
	if sub.ID != "abc/agent-x" || sub.Kind != "subagent" || sub.ParentID != "abc" {
		t.Errorf("subagent: got %+v", sub)
	}
	// A reader that gives no session id: the log is named after its file.
	if got := sessionFor(source.Log{Path: "/p/proj/xyz.jsonl"}); got.ID != "xyz" || got.Kind != "main" {
		t.Errorf("unnamed: got %+v", got)
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
	if s.Project != "/repo" || s.Turns[0].Project != "/repo" {
		t.Errorf("project: got %q, want /repo (from the cwd; named later, across sessions)", s.Project)
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

// A subagent started in a subfolder of its conversation's project belongs
// to that project, as the main session's turns there do; one that worked
// in an unrelated folder keeps it.
func TestInheritProjectsBySubfolder(t *testing.T) {
	sessions := []event.Session{
		{ID: "m", Kind: "main", Project: "/code/app", Turns: []event.Turn{{Project: "/code/app"}}},
		{ID: "m/a", Kind: "subagent", ParentID: "m", Project: "/code/app/spec/harness",
			Turns: []event.Turn{{Project: "/code/app/spec/harness"}, {Project: "/code/other"}}},
		{ID: "m/b", Kind: "subagent", ParentID: "m", Project: "/code/other", Turns: []event.Turn{{Project: "/code/other"}}},
	}
	inheritProjects(sessions)
	nameProjects(sessions)
	a, b := sessions[1], sessions[2]
	if a.Project != "app" || a.Turns[0].Project != "app" || a.Turns[1].Project != "other" {
		t.Errorf("a: got %q, turns %q and %q; want app, app, other", a.Project, a.Turns[0].Project, a.Turns[1].Project)
	}
	if b.Project != "other" || b.Turns[0].Project != "other" {
		t.Errorf("b: got %q, turn %q; want other", b.Project, b.Turns[0].Project)
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
	for _, want := range []string{"repo", "1 compaction (1 asked for)", "/compact", "No sub-agent runs."} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report has no %q", want)
		}
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
		{ID: "c", Model: "<synthetic>", Synthetic: true}}}, seen)
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

// A turn is Prompted when the user typed a message, answered Claude's
// question or decided on a permission since the previous turn; other user
// lines and copied lines do not count, and the flag does not carry over.
func TestBuildSessionPrompted(t *testing.T) {
	asst := func(id, calls string) string {
		return `{"type":"assistant","message":{"id":"` + id + `","content":[` + calls + `],"usage":{"output_tokens":1}}}` + "\n"
	}
	ask := `{"type":"tool_use","id":"q1","name":"AskUserQuestion","input":{}}`
	bash := `{"type":"tool_use","id":"b1","name":"Bash","input":{"command":"ls"}}`
	bash2 := `{"type":"tool_use","id":"b2","name":"Bash","input":{"command":"ls"}}`
	result := func(id, extra string) string {
		return `{"type":"user"` + extra + `,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"` + id + `","content":"ok"}]}}` + "\n"
	}
	l := writeLog(t, "s.jsonl", ""+
		`{"type":"user","uuid":"u1","origin":{"kind":"human"},"message":{"role":"user","content":"hi"}}`+"\n"+
		asst("m1", ask)+
		result("q1", "")+ // the user answered
		asst("m2", bash)+
		result("b1", "")+ // a tool ran: not the user
		asst("m3", bash2)+
		result("b2", `,"permissionDecision":{"decision":"accept","source":"user_temporary"}`)+ // the user allowed it
		asst("m4", `{"type":"tool_use","id":"b3","name":"Bash","input":{"command":"rm x"}}`)+
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"b3","is_error":true,"content":"The user doesn't want to proceed with this tool use."}]}}`+"\n"+ // the user refused
		asst("m4b", "")+
		`{"type":"user","origin":{"kind":"peer"},"message":{"role":"user","content":"from another agent"}}`+"\n"+
		asst("m5", "")+
		`{"type":"user","isCompactSummary":true,"message":{"role":"user","content":"summary"}}`+"\n"+
		asst("m6", ""))
	s, _, err := readSession(cc, l, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	want := []bool{true, true, false, true, true, false, false}
	for i, w := range want {
		if i >= len(s.Turns) || s.Turns[i].Prompted != w {
			t.Fatalf("turns: got %+v, want prompted %v", s.Turns, want)
		}
	}
	// The same lines copied into a resumed log mark nothing.
	seen := map[string]bool{"line:u1": true}
	l2 := writeLog(t, "r.jsonl", `{"type":"user","uuid":"u1","origin":{"kind":"human"},"message":{"role":"user","content":"hi"}}`+"\n"+asst("m9", ""))
	if s, _, _ := readSession(cc, l2, seen); len(s.Turns) != 1 || s.Turns[0].Prompted {
		t.Errorf("copied prompt: got %+v", s.Turns)
	}
}

// The reply that triggered a compaction can be logged after the compaction
// with an earlier time: it belongs before it, and the next reply is the
// first after the compaction.
func TestBuildSessionReplyLoggedAfterItsCompaction(t *testing.T) {
	l := writeLog(t, "s.jsonl", ""+
		`{"type":"assistant","timestamp":"2026-11-01T09:00:00Z","message":{"id":"m1","usage":{"cache_read_input_tokens":900000}}}`+"\n"+
		`{"type":"system","subtype":"compact_boundary","timestamp":"2026-11-01T09:05:00Z","compactMetadata":{"preTokens":960000,"postTokens":30000}}`+"\n"+
		`{"type":"attachment","uuid":"a1","attachment":{"type":"session_context"},"rendered":[{"content":"re-attached"}]}`+"\n"+
		`{"type":"assistant","timestamp":"2026-11-01T09:04:57Z","message":{"id":"m2","usage":{"cache_read_input_tokens":960000}}}`+"\n"+
		`{"type":"assistant","timestamp":"2026-11-01T09:06:00Z","message":{"id":"m3","usage":{"cache_read_input_tokens":80000}}}`+"\n")
	s, _, err := readSession(cc, l, map[string]bool{})
	if err != nil || len(s.Turns) != 3 {
		t.Fatalf("got %+v, %v", s.Turns, err)
	}
	if s.Turns[1].CompactedBefore || !s.Turns[2].CompactedBefore || s.Turns[2].Compaction.After != 30000 {
		t.Errorf("got compacted-before %v, %v; want only the third turn", s.Turns[1].CompactedBefore, s.Turns[2].CompactedBefore)
	}
	if len(s.Turns[1].Injected) != 0 || len(s.Turns[2].Injected) != 1 {
		t.Errorf("re-attached text: got %d on the triggering reply, %d after; want 0 and 1",
			len(s.Turns[1].Injected), len(s.Turns[2].Injected))
	}
}

// Two compactions with no reply between them are both kept, and one after
// the last reply too.
func TestBuildSessionKeepsUnrepliedCompactions(t *testing.T) {
	marker := func(at string) string {
		return `{"type":"system","subtype":"compact_boundary","timestamp":"` + at + `","compactMetadata":{"preTokens":500000,"postTokens":30000}}` + "\n"
	}
	l := writeLog(t, "s.jsonl", ""+
		`{"type":"assistant","timestamp":"2026-11-01T09:00:00Z","message":{"id":"m1","usage":{"cache_read_input_tokens":500000}}}`+"\n"+
		marker("2026-11-01T09:01:00Z")+marker("2026-11-01T09:02:00Z")+
		`{"type":"assistant","timestamp":"2026-11-01T09:03:00Z","message":{"id":"m2","usage":{"cache_read_input_tokens":80000}}}`+"\n"+
		marker("2026-11-01T09:04:00Z"))
	s, _, err := readSession(cc, l, map[string]bool{})
	if err != nil || len(s.Turns) != 2 || !s.Turns[1].CompactedBefore {
		t.Fatalf("got %+v, %v", s.Turns, err)
	}
	if len(s.Unreplied) != 2 || s.Unreplied[0].At.Minute() != 1 || s.Unreplied[1].At.Minute() != 4 {
		t.Errorf("unreplied: got %+v, want the 09:01 and 09:04 compactions", s.Unreplied)
	}
}

func TestBuildSessionKeepsLimitHits(t *testing.T) {
	hit := func(uuid string) string {
		return `{"type":"assistant","uuid":"` + uuid + `","timestamp":"2026-10-09T21:52:07Z","isApiErrorMessage":true,"error":"rate_limit",` +
			`"message":{"model":"<synthetic>","content":[{"type":"text","text":"You've hit your weekly limit"}],"usage":{"output_tokens":0}}}` + "\n"
	}
	seen := map[string]bool{}
	s, _, err := readSession(cc, writeLog(t, "a.jsonl", hit("h1")), seen)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 0 || len(s.LimitHits) != 1 || s.LimitHits[0].Limit != "weekly" {
		t.Errorf("limit hit: got %d turns, hits %+v; want no turns and one weekly hit", len(s.Turns), s.LimitHits)
	}
	// A copy of the same line in a resumed session counts once.
	s, _, _ = readSession(cc, writeLog(t, "b.jsonl", hit("h1")), seen)
	if len(s.LimitHits) != 0 {
		t.Errorf("copied limit hit counted again: %+v", s.LimitHits)
	}
}

func TestBuildSessionRecordsFailures(t *testing.T) {
	reply := `{"type":"assistant","timestamp":"2026-11-01T09:00:00Z","message":{"id":"m1","model":"claude-opus-5-5","usage":{"cache_read_input_tokens":250000}}}` + "\n"
	sleep := `{"type":"assistant","uuid":"f1","timestamp":"2026-11-01T10:00:00Z","isApiErrorMessage":true,"error":"unknown",` +
		`"message":{"model":"<synthetic>","content":[{"type":"text","text":"API Error: Your computer went to sleep mid-response."}],"usage":{"output_tokens":0}}}` + "\n"
	again := `{"type":"assistant","timestamp":"2026-11-01T10:05:00Z","message":{"id":"m2","model":"claude-opus-5-5","usage":{"cache_read_input_tokens":250000}}}` + "\n"
	s, _, err := readSession(cc, writeLog(t, "s.jsonl", reply+sleep+again), map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Failures) != 1 || s.Failures[0].Kind != "sleep" || s.Failures[0].Context != 250_000 || s.Failures[0].Model != "claude-opus-5-5" {
		t.Errorf("failures: got %+v", s.Failures)
	}
	// The failure is activity: the next turn's idle time runs from it.
	if len(s.Turns) != 2 || s.Turns[1].IdleBefore != 5*time.Minute {
		t.Errorf("idle before the retry: got %+v", s.Turns)
	}
}

func TestLimitHitIsActivity(t *testing.T) {
	reply := func(id, at string) string {
		return `{"type":"assistant","timestamp":"` + at + `","message":{"id":"` + id + `","model":"claude-opus-5-5","usage":{"cache_read_input_tokens":1000}}}` + "\n"
	}
	hit := `{"type":"assistant","uuid":"h1","timestamp":"2026-11-01T10:40:00Z","isApiErrorMessage":true,"error":"rate_limit",` +
		`"message":{"model":"<synthetic>","content":[{"type":"text","text":"You've hit your session limit"}],"usage":{"output_tokens":0}}}` + "\n"
	s, _, err := readSession(cc, writeLog(t, "s.jsonl", reply("m1", "2026-11-01T10:05:00Z")+hit+reply("m2", "2026-11-01T11:10:00Z")), map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 2 || s.Turns[1].IdleBefore != 30*time.Minute || len(s.LimitHits) != 1 {
		t.Errorf("idle after a limit hit: got %+v", s.Turns)
	}
}

// A session refused at its first message (a usage limit, a failure, a reply
// the harness made up) has written no cache: its first real turn is still
// the first, with no idle time before it from the refusals.
func TestBuildSessionFirstTurnAfterRefusals(t *testing.T) {
	d := logData{log: source.Log{Path: "/p/s.jsonl"}, msgs: []source.Message{
		{Kind: source.KindLimitHit, ID: "l", Time: t0, Limit: "session"},
		{Kind: source.KindFailure, ID: "f", Time: t0.Add(time.Minute), Failure: "network"},
		{ID: "x", Model: "<synthetic>", Synthetic: true, Time: t0.Add(2 * time.Minute)},
		{ID: "a", Model: "claude-sonnet-4-5", Time: t0.Add(5 * time.Hour)},
		{ID: "b", Model: "claude-sonnet-4-5", Time: t0.Add(6 * time.Hour)},
	}}
	s := buildSession(d, map[string]bool{})
	if len(s.Turns) != 2 || !s.Turns[0].First || s.Turns[1].First {
		t.Fatalf("first: got %+v", s.Turns)
	}
	if s.Turns[1].IdleBefore != time.Hour {
		t.Errorf("idle before the second turn: got %v", s.Turns[1].IdleBefore)
	}
}

// A turn is credited to its session's project when it runs in a subfolder
// of it or a folder above it, and to another project only when it runs
// somewhere else.
func TestProjectFor(t *testing.T) {
	home := "/code/api"
	for dir, want := range map[string]string{
		"":                  home,
		"/code/api":         home,
		"/code/api/cmd/x":   home,
		"/code":             home, // cd ..
		"/code/api-tools":   "/code/api-tools",
		"/elsewhere/notes":  "/elsewhere/notes",
		"/code/web/sub/api": "/code/web/sub/api",
	} {
		if got := projectFor(dir, home); got != want {
			t.Errorf("projectFor(%q) = %q, want %q", dir, got, want)
		}
	}
	if got := projectFor("/code/api", ""); got != "" {
		t.Errorf("no home yet: got %q, want \"\" (the session's first turn sets it)", got)
	}
}

// Two projects with the same folder name are told apart by their parent
// folders; others keep their own name.
func TestNameProjects(t *testing.T) {
	sessions := []event.Session{
		{Project: "/work/api", Turns: []event.Turn{{Project: "/work/api"}, {Project: "/work/notes"}}},
		{Project: "/home/me/api", Turns: []event.Turn{{Project: "/home/me/api"}}},
		{Project: "/a/x/web", Turns: []event.Turn{{Project: "/b/x/web"}, {Project: ""}}},
		{Project: ""},
	}
	nameProjects(sessions)
	got := []string{sessions[0].Project, sessions[0].Turns[1].Project, sessions[1].Project,
		sessions[2].Project, sessions[2].Turns[0].Project, sessions[2].Turns[1].Project, sessions[3].Project}
	want := []string{"work/api", "notes", "me/api", "a/x/web", "b/x/web", "", ""}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A folder whose path is the end of another's keeps its short name, the
// others gain parents until they differ, and the names do not depend on
// map order.
func TestShortNamesStable(t *testing.T) {
	want := map[string]string{"": "", "/app": "app", "/Users/c/app": "Users/c/app", "/x/c/app": "x/c/app", "/c/web/app": "web/app",
		"/work/api": "work/api", "/home/api": "home/api", `C:\code\web`: "web"}
	for range 200 {
		dirs := map[string]bool{}
		for d := range want {
			if d != "" {
				dirs[d] = true
			}
		}
		got := shortNames(dirs)
		for d, name := range want {
			if got[d] != name {
				t.Fatalf("%q: got %q, want %q (all: %q)", d, got[d], name, got)
			}
		}
	}
}

// A session that moves into a subfolder stays one project; the reader's
// folders become short names only after all sessions are read.
func TestSessionProjectAcrossSubfolders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s1.jsonl")
	lines := `{"type":"assistant","sessionId":"s1","cwd":"/code/api","timestamp":"2026-11-01T09:00:00Z","message":{"id":"m1","model":"claude-opus-5-5","usage":{"output_tokens":1}}}
{"type":"assistant","sessionId":"s1","cwd":"/code/api/web","timestamp":"2026-11-01T09:01:00Z","message":{"id":"m2","model":"claude-opus-5-5","usage":{"output_tokens":1}}}
{"type":"assistant","sessionId":"s1","cwd":"/code","timestamp":"2026-11-01T09:02:00Z","message":{"id":"m3","model":"claude-opus-5-5","usage":{"output_tokens":1}}}
`
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	l := source.Log{Path: path}
	s, _, err := readSession(cc, &l, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	sessions := []event.Session{s}
	nameProjects(sessions)
	for _, turn := range sessions[0].Turns {
		if turn.Project != "api" {
			t.Errorf("turn %s: got project %q, want api", turn.ID, turn.Project)
		}
	}
}
