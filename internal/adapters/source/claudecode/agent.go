package claudecode

import (
	"encoding/json"
	"os"
	"strings"
)

// AgentType returns the agent type Claude Code recorded beside a sub-agent's
// log (agent-<id>.meta.json next to agent-<id>.jsonl), or "" if there is
// none. Lines in the log name the type only when the agent was started with
// attribution, so this fills the gap.
func AgentType(logPath string) string {
	base, ok := strings.CutSuffix(logPath, ".jsonl")
	if !ok {
		return ""
	}
	b, err := os.ReadFile(base + ".meta.json")
	if err != nil || len(b) > 64<<10 {
		return ""
	}
	var meta struct {
		AgentType string `json:"agentType"`
	}
	if json.Unmarshal(b, &meta) != nil || len(meta.AgentType) > 128 {
		return ""
	}
	return meta.AgentType
}
