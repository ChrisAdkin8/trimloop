// Package claudecode reads Claude Code session logs from disk.
package claudecode

import (
	"path/filepath"
	"strings"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source"
)

// Reader reads the session logs Claude Code keeps under Home/.claude/projects.
// It implements source.Reader.
type Reader struct {
	Home string // the user's home directory
	// ConfigDir is Claude Code\'s configuration folder when it was moved
	// (CLAUDE_CONFIG_DIR); empty means Home/.claude.
	ConfigDir string
}

var _ source.Reader = Reader{} // fails to compile if Reader stops meeting the interface

// Name identifies the harness.
func (Reader) Name() string { return "claude-code" }

// Projects is the folder Claude Code keeps its session logs in.
func (r Reader) Projects() string {
	dir := r.ConfigDir
	if dir == "" {
		dir = filepath.Join(r.Home, ".claude")
	}
	return filepath.Join(dir, "projects")
}

// Discover returns every Claude Code session log, including the logs written
// by subagents. Glob supports single '*' (not '**'), so each folder depth
// needs its own pattern.
func (r Reader) Discover() ([]source.Log, error) {
	base := r.Projects()
	patterns := []string{
		filepath.Join(base, "*", "*.jsonl"),                   // main sessions
		filepath.Join(base, "*", "*", "subagents", "*.jsonl"), // subagents
	}

	var logs []source.Log
	for _, pattern := range patterns {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err // only happens if the pattern itself is malformed
		}
		for _, p := range paths {
			l := source.Log{Harness: r.Name(), Path: p}
			l.Session, l.Parent = identify(p)
			if l.Parent != "" {
				l.AgentType = agentType(p)
			}
			logs = append(logs, l)
		}
	}
	return logs, nil
}

// identify names the session a log holds. A main session's log is named
// after its session id. A sub-agent's log (<session>/subagents/<agent>.jsonl)
// is shown with its session id in front, so it can be traced back to the
// session that started it, which is its parent.
func identify(path string) (session, parent string) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	dir := filepath.Dir(path)
	if filepath.Base(dir) == "subagents" {
		parent = filepath.Base(filepath.Dir(dir))
		return parent + "/" + name, parent
	}
	return name, ""
}
