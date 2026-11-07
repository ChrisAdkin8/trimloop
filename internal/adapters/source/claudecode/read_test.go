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
