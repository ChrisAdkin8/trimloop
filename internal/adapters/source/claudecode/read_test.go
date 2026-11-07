package claudecode

import (
	"os"
	"path/filepath"
	"reflect"
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
