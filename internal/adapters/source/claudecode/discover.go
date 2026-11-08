// Package claudecode reads Claude Code session logs from disk.
package claudecode

import (
	"path/filepath"

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
			logs = append(logs, source.Log{Harness: r.Name(), Path: p})
		}
	}
	return logs, nil
}
