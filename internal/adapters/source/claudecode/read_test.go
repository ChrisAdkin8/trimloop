package claudecode

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Offsets, unfinished lines, replaced files and bad-line reporting are tested
// in package jsonl. These tests cover what is specific to Claude Code: which
// lines carry usage and how they are parsed.

func TestParseLine(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   source.Message
		wantOK bool
		errors bool
	}{
		{
			name:   "assistant message with usage",
			line:   `{"type":"assistant","message":{"id":"m1","model":"claude-x","usage":{"input_tokens":3,"output_tokens":5}}}`,
			want:   source.Message{ID: "m1", Model: "claude-x", Usage: event.Usage{InputUncached: 3, Output: 5}},
			wantOK: true,
		},
		{
			name: "session, time and price options",
			line: `{"type":"assistant","sessionId":"s1","timestamp":"2026-10-30T08:06:18.721Z",` +
				`"message":{"id":"m1","usage":{"output_tokens":1,"speed":"fast","inference_geo":"us"}}}`,
			want: source.Message{ID: "m1", SessionID: "s1", Time: time.Date(2026, 10, 30, 8, 6, 18, 721e6, time.UTC),
				Usage: event.Usage{Output: 1}, Fast: true, USOnly: true},
			wantOK: true,
		},
		{
			name:   "bad timestamp keeps the usage, with no time",
			line:   `{"type":"assistant","timestamp":"30/10/2026 08:06","message":{"id":"m1","usage":{"output_tokens":1}}}`,
			want:   source.Message{ID: "m1", Usage: event.Usage{Output: 1}},
			wantOK: true,
		},
		{
			name:   "timestamp of the wrong JSON type keeps the usage, with no time",
			line:   `{"type":"assistant","timestamp":1730275578,"message":{"id":"m1","usage":{"output_tokens":1}}}`,
			want:   source.Message{ID: "m1", Usage: event.Usage{Output: 1}},
			wantOK: true,
		},
		{
			name: "user line is skipped",
			line: `{"type":"user","message":{"role":"user","content":"hi"}}`,
		},
		{
			name: "assistant line without usage is skipped",
			line: `{"type":"assistant","message":{"id":"m2","model":"claude-x"}}`,
		},
		{
			name: "other line types are skipped",
			line: `{"type":"summary","summary":"x"}`,
		},
		{
			name:   "not JSON",
			line:   `not json`,
			errors: true,
		},
		{
			name:   "valid JSON in the wrong shape",
			line:   `{"type":"assistant","message":"hello"}`,
			errors: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := parseLine([]byte(tt.line))
			if (err != nil) != tt.errors {
				t.Fatalf("error: got %v, want error=%v", err, tt.errors)
			}
			if ok != tt.wantOK || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v ok=%v, want %+v ok=%v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// TestRead checks that Read wires parseLine into the shared line reader.
func TestRead(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	log := &source.Log{Path: p}
	writeFile(t, p, ""+
		`{"type":"user","message":{"role":"user","content":"hi"}}`+"\n"+
		`{"type":"assistant","message":{"id":"m1","usage":{"output_tokens":5}}}`+"\n"+
		`{"type":"assistant","message":{"id":"m1","usage":{"output_tokens":5}}}`+"\n")
	var got []source.Message
	for m, err := range (Reader{}).Read(log) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, m)
	}
	m1 := source.Message{ID: "m1", Usage: event.Usage{Output: 5}}
	if want := []source.Message{m1, m1}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseLineToolCalls(t *testing.T) {
	line := `{"type":"assistant","cwd":"/repo","message":{"id":"m1","usage":{"output_tokens":1},"content":[` +
		`{"type":"text","text":"looking"},` +
		`{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"cd sub && sed -n '1,9p' main.go | head"}},` +
		`{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"/repo/main.go","limit":20}},` +
		`{"type":"tool_use","id":"t3","name":"Edit","input":{"file_path":"main.go"}},` +
		`{"type":"tool_use","id":"t4","name":"mcp__github__get_issue","input":{"command":["not","a","string"]}},` +
		`{"type":"tool_use","id":"t5","name":"Grep","input":{"pattern":"x"}}]}}`
	m, ok, err := parseLine([]byte(line))
	if err != nil || !ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
	mainGo := fileKey("/repo/main.go", "")
	want := []event.ToolCall{
		// The shell's cd is not followed, so sed's file resolves against the session's cwd.
		{ID: "t1", Tool: "Bash", Shape: "sed -n", Noise: event.NoiseFileView, Filtered: true, File: mainGo, FileOp: event.FileView},
		{ID: "t2", Tool: "Read", Noise: event.NoiseFileView, File: mainGo, FileOp: event.FileView},
		{ID: "t3", Tool: "Edit", File: mainGo, FileOp: event.FileEdit},
		{ID: "t4", Tool: "mcp__github__get_issue", Noise: event.NoiseMCP},
		{ID: "t5", Tool: "Grep", Noise: event.NoiseSearch},
	}
	if !reflect.DeepEqual(m.ToolCalls, want) {
		t.Errorf("got  %+v\nwant %+v", m.ToolCalls, want)
	}
}

func TestParseLineToolResults(t *testing.T) {
	line := `{"type":"user","timestamp":"2026-10-30T08:06:18Z","message":{"role":"user","content":[` +
		`{"type":"tool_result","tool_use_id":"t1","content":"12345"},` +
		`{"type":"tool_result","tool_use_id":"t2","is_error":true,"content":[{"type":"text","text":"ab"},{"type":"image","source":{}},{"type":"text","text":"cd"}]},` +
		`{"type":"tool_result","tool_use_id":"t3","content":"<persisted-output>\nOutput too large"},` +
		`{"type":"text","text":"not a result"}]}}`
	m, ok, err := parseLine([]byte(line))
	if err != nil || !ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
	want := source.Message{
		Kind: source.KindToolResults,
		Time: time.Date(2026, 10, 30, 8, 6, 18, 0, time.UTC),
		Results: []source.Result{
			{CallID: "t1", ToolResult: event.ToolResult{Bytes: 5}},
			{CallID: "t2", ToolResult: event.ToolResult{Bytes: 4, Error: true}},
			{CallID: "t3", ToolResult: event.ToolResult{Bytes: 35, Persisted: true}},
		},
	}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("got  %+v\nwant %+v", m, want)
	}
}

func TestParseLineCompaction(t *testing.T) {
	m, ok, err := parseLine([]byte(`{"type":"system","subtype":"compact_boundary","sessionId":"s1","compactMetadata":{"preTokens":5}}`))
	if err != nil || !ok || m.Kind != source.KindCompaction || m.SessionID != "s1" {
		t.Errorf("got %+v ok=%v err=%v, want a compaction", m, ok, err)
	}
	if _, ok, _ := parseLine([]byte(`{"type":"system","subtype":"turn_duration"}`)); ok {
		t.Error("other system lines should be skipped")
	}
}

func TestFileKey(t *testing.T) {
	if fileKey("a/../b.go", "/r") != fileKey("/r/b.go", "") {
		t.Error("relative and absolute paths to the same file differ")
	}
	for _, p := range []string{"", "$f", "*.go", "a?.go"} {
		if k := fileKey(p, "/r"); k != "" {
			t.Errorf("fileKey(%q) = %q, want empty", p, k)
		}
	}
}

func TestParseLineAttributionAndProject(t *testing.T) {
	line := `{"type":"assistant","cwd":"/code/web/.claude/worktrees/agent-1","attributionSkill":"review",` +
		`"attributionPlugin":"tools","attributionMcpServer":"github","attributionAgent":"general-purpose",` +
		`"message":{"id":"m1","usage":{"output_tokens":1}}}`
	m, ok, err := parseLine([]byte(line))
	if err != nil || !ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
	want := event.Attribution{Skill: "review", Plugin: "tools", MCPServer: "github", Agent: "general-purpose"}
	if m.Attribution != want || m.Project != "web" {
		t.Errorf("got attribution %+v project %q", m.Attribution, m.Project)
	}
}

func TestParseLineCompactionMetadata(t *testing.T) {
	m, _, _ := parseLine([]byte(`{"type":"system","subtype":"compact_boundary",` +
		`"compactMetadata":{"trigger":"manual","preTokens":900000,"postTokens":20000,"durationMs":1500}}`))
	want := event.Compaction{Before: 900_000, After: 20_000, Manual: true, Duration: 1500 * time.Millisecond}
	if m.Compaction != want {
		t.Errorf("got %+v, want %+v", m.Compaction, want)
	}
}

func TestProjectOf(t *testing.T) {
	for cwd, want := range map[string]string{
		"":                                  "",
		"/code/web":                         "web",
		"/code/web/.claude/worktrees/x":     "web",
		"/code/web/.claude/worktrees/x/sub": "web",
		"/code/web/sub":                     "sub",
	} {
		if got := projectOf(cwd); got != want {
			t.Errorf("projectOf(%q) = %q, want %q", cwd, got, want)
		}
	}
}

func TestParseLineAttachments(t *testing.T) {
	skill := `{"type":"attachment","sessionId":"s1","attachment":{"type":"skill_listing"},"renderedRole":"system",` +
		`"rendered":[{"content":"abcdef"},{"content":[{"type":"text","text":"x"}]}]}`
	m, ok, err := parseLine([]byte(skill))
	if err != nil || !ok || m.Kind != source.KindInjected || len(m.Injected) != 1 ||
		m.Injected[0].Kind != "skill_listing" || m.Injected[0].Bytes != 6+len(`[{"type":"text","text":"x"}]`) {
		t.Errorf("skill listing: got %+v ok=%v err=%v", m, ok, err)
	}

	memory := `{"type":"attachment","attachment":{"type":"instructions","files":[` +
		`{"path":"/home/me/code/app/CLAUDE.md","type":"Project","content":"12345"},{"path":"/home/me/.claude/CLAUDE.md","type":"User","content":"12"}]},` +
		`"rendered":[{"content":"1234567"}]}`
	m, _, _ = parseLine([]byte(memory))
	want := []event.Injected{{Kind: "memory_file", Name: "Project CLAUDE.md", Bytes: 5}, {Kind: "memory_file", Name: "User CLAUDE.md", Bytes: 2}}
	if !reflect.DeepEqual(m.Injected, want) {
		t.Errorf("memory files: got %+v", m.Injected)
	}

	if _, ok, _ := parseLine([]byte(`{"type":"attachment","attachment":{"type":"date"},"rendered":null}`)); ok {
		t.Error("an attachment that added nothing should be skipped")
	}
}

func TestParseLineSnapshot(t *testing.T) {
	for name, line := range map[string]string{
		"top level": `{"type":"attachment","attachment":{"type":"prompt_snapshot","systemPrompt":["abc","de"],` +
			`"tools":[{"name":"Bash","description":"run","schema":{"a":1}},{"name":"mcp__gh__issue","description":"","schema":{}}]}}`,
		"nested": `{"type":"attachment","attachment":{"type":"prompt_snapshot","snapshot":{"systemPrompt":["abc","de"],` +
			`"tools":[{"name":"Bash","description":"run","schema":{"a":1}},{"name":"mcp__gh__issue","description":"","schema":{}}]}}}`,
	} {
		m, ok, err := parseLine([]byte(line))
		if err != nil || !ok || m.Kind != source.KindStanding {
			t.Fatalf("%s: got %+v ok=%v err=%v", name, m, ok, err)
		}
		want := &event.Standing{SystemBytes: 5, Tools: []event.ToolDef{{Name: "Bash", Bytes: 4 + 3 + 7}, {Name: "mcp__gh__issue", Bytes: 14 + 2}}}
		if !reflect.DeepEqual(m.Standing, want) {
			t.Errorf("%s: got %+v", name, m.Standing)
		}
	}
}

func TestParseLineCostRecord(t *testing.T) {
	line := `{"type":"cost-state","sessionId":"s1","timestamp":"2026-11-01T10:00:00Z","startTime":1730000000,"totalCostUSD":1.5,` +
		`"modelUsage":{"claude-haiku-5-5":{"inputTokens":10,"outputTokens":2,"cacheReadInputTokens":3,"cacheCreationInputTokens":4,"costUSD":0.5,"webSearchRequests":7}}}`
	m, ok, err := parseLine([]byte(line))
	if err != nil || !ok || m.Kind != source.KindCostRecord || m.RunID != "1730000000" || m.Record.Cost != 1.5 ||
		!m.Record.At.Equal(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %+v ok=%v err=%v", m, ok, err)
	}
	want := event.ModelRecord{Cost: 0.5, InputUncached: 10, CacheRead: 3, CacheWrite: 4, Output: 2, WebSearches: 7}
	if m.Record.ByModel["claude-haiku-5-5"] != want {
		t.Errorf("got %+v", m.Record.ByModel)
	}
}

func TestLastResponse(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, p, ""+
		`{"type":"assistant","timestamp":"2026-11-01T09:00:00Z","message":{"id":"m1","usage":{"cache_read_input_tokens":100}}}`+"\n"+
		`{"type":"assistant","timestamp":"2026-11-01T09:05:00Z","message":{"id":"m2","usage":{"cache_read_input_tokens":200}}}`+"\n"+
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":"ok"}]}}`+"\n"+
		`{"type":"assistant","message":{"id":"m3","usage":{"cache_read_input_tokens":3`) // still being written
	m, err := LastResponse(p)
	if err != nil || m.ID != "m2" || m.Usage.CacheRead != 200 {
		t.Errorf("got %+v, %v; want m2", m, err)
	}

	empty := filepath.Join(t.TempDir(), "e.jsonl")
	writeFile(t, empty, `{"type":"user","message":{"role":"user","content":"hi"}}`+"\n")
	if _, err := LastResponse(empty); !errors.Is(err, ErrNoResponse) {
		t.Errorf("got %v, want ErrNoResponse", err)
	}
	if _, err := LastResponse(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("got no error for a missing file")
	}
}

// Only the end of a large log is read; a line cut by the start is skipped.
func TestLastResponseLargeLog(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big.jsonl")
	filler := `{"type":"user","message":{"role":"user","content":"` + strings.Repeat("x", 4000) + `"}}` + "\n"
	writeFile(t, p, `{"type":"assistant","message":{"id":"early","usage":{"output_tokens":1}}}`+"\n"+
		strings.Repeat(filler, 400)+
		`{"type":"assistant","message":{"id":"late","usage":{"output_tokens":2}}}`+"\n")
	m, err := LastResponse(p)
	if err != nil || m.ID != "late" {
		t.Errorf("got %+v, %v", m, err)
	}
}

// A made-up reply after an API error has no usage and is not the last reply.
func TestLastResponseSkipsSynthetic(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, p, ""+
		`{"type":"assistant","message":{"id":"m1","model":"claude-opus-5-5","usage":{"cache_read_input_tokens":300000}}}`+"\n"+
		`{"type":"assistant","message":{"id":"m2","model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0}}}`+"\n")
	m, err := LastResponse(p)
	if err != nil || m.ID != "m1" {
		t.Errorf("got %+v, %v; want m1", m, err)
	}
}

func TestParseLineKeepsLineID(t *testing.T) {
	m, _, _ := parseLine([]byte(`{"type":"system","subtype":"compact_boundary","uuid":"u1"}`))
	if m.LineID != "u1" {
		t.Errorf("compaction: got %q", m.LineID)
	}
	m, _, _ = parseLine([]byte(`{"type":"attachment","uuid":"u2","attachment":{"type":"skill_listing"},"rendered":[{"content":"x"}]}`))
	if m.LineID != "u2" {
		t.Errorf("attachment: got %q", m.LineID)
	}
}
