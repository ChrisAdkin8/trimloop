package claudecode

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source"
)

func TestDiscover(t *testing.T) {
	home := t.TempDir()
	projects := filepath.Join(home, ".claude", "projects")

	session := filepath.Join(projects, "proj", "s1.jsonl")
	subagent := filepath.Join(projects, "proj", "s1", "subagents", "agent-a.jsonl")
	ignored := []string{
		filepath.Join(projects, "proj", "notes.txt"),                       // not .jsonl
		filepath.Join(projects, "top.jsonl"),                               // too shallow
		filepath.Join(projects, "proj", "s1", "other", "x.jsonl"),          // not a subagents folder
		filepath.Join(projects, "proj", "s1", "subagents", "a", "y.jsonl"), // too deep
	}
	for _, p := range append([]string{session, subagent}, ignored...) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Reader{Home: home}.Discover()
	if err != nil {
		t.Fatal(err)
	}
	want := []source.Log{
		{Harness: "claude-code", Path: session, Session: "s1"},
		{Harness: "claude-code", Path: subagent, Session: "s1/agent-a", Parent: "s1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDiscoverNoProjectsFolder(t *testing.T) {
	got, err := Reader{Home: t.TempDir()}.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want no logs", got)
	}
}

// A moved Claude Code configuration folder (CLAUDE_CONFIG_DIR) is followed.
func TestDiscoverConfigDir(t *testing.T) {
	cfg := t.TempDir()
	p := filepath.Join(cfg, "projects", "proj", "s.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	logs, err := Reader{Home: t.TempDir(), ConfigDir: cfg}.Discover()
	if err != nil || len(logs) != 1 || logs[0].Path != p {
		t.Errorf("got %+v, %v", logs, err)
	}
}

// identify names a main session after its log, and a sub-agent's run after
// the session that started it and its own log.
func TestIdentify(t *testing.T) {
	if s, p := identify("/p/proj/abc.jsonl"); s != "abc" || p != "" {
		t.Errorf("main: got %q, %q", s, p)
	}
	if s, p := identify("/p/proj/abc/subagents/agent-x.jsonl"); s != "abc/agent-x" || p != "abc" {
		t.Errorf("subagent: got %q, %q", s, p)
	}
}

// Discover reads a sub-agent's type from the file Claude Code writes beside
// its log.
func TestDiscoverAgentType(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", "proj", "s1", "subagents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"agent-a.jsonl": "", "agent-a.meta.json": `{"agentType":"Explore"}`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	logs, err := Reader{Home: home}.Discover()
	if err != nil || len(logs) != 1 || logs[0].AgentType != "Explore" {
		t.Errorf("got %+v, %v; want one log of an Explore agent", logs, err)
	}
}
