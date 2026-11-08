package claudecode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentType(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "agent-a1.jsonl")
	if got := AgentType(log); got != "" {
		t.Errorf("no meta file: got %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-a1.meta.json"), []byte(`{"agentType":"general-purpose","spawnDepth":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := AgentType(log); got != "general-purpose" {
		t.Errorf("got %q, want general-purpose", got)
	}
	if got := AgentType(filepath.Join(dir, "agent-a1.txt")); got != "" {
		t.Errorf("not a log: got %q", got)
	}
}
