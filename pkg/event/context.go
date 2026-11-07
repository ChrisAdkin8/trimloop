package event

import "time"

// Injected is text the harness added to the conversation, such as a skill
// listing or a memory file. Like tool output, it stays in the conversation
// and is sent again on every later turn until the session is compacted.
type Injected struct {
	Kind  string // the harness's kind, e.g. "skill_listing", "memory_file"
	Name  string // for a memory file, which one (e.g. "Project CLAUDE.md"); else ""
	Bytes int
}

// Tokens is the estimated size in tokens, rounded up.
func (i Injected) Tokens() int { return (i.Bytes + BytesPerToken - 1) / BytesPerToken }

// Standing is the fixed start of every request: the system prompt and the
// tool definitions, as the harness last recorded them. It is the same on
// every turn until the harness records a change.
type Standing struct {
	SystemBytes int
	Tools       []ToolDef
}

// ToolDef is one tool definition sent with every request.
type ToolDef struct {
	Name  string
	Bytes int
}

// Bytes is the size of the system prompt and all tool definitions.
func (s *Standing) Bytes() int {
	if s == nil {
		return 0
	}
	n := s.SystemBytes
	for _, t := range s.Tools {
		n += t.Bytes
	}
	return n
}

// HarnessRecord is the harness's own account of a session's usage and cost,
// which can include requests that never appear in its logs.
type HarnessRecord struct {
	Cost    float64
	ByModel map[string]ModelRecord
	At      time.Time // when the latest record was written; zero if not known
}

// ModelRecord is the harness's account of one model's usage in a session.
type ModelRecord struct {
	Cost          float64
	InputUncached int
	CacheRead     int
	CacheWrite    int
	Output        int
	WebSearches   int // server-side web search requests, billed per request
}

// Add returns the sum of r and o.
func (r ModelRecord) Add(o ModelRecord) ModelRecord {
	return ModelRecord{
		Cost:          r.Cost + o.Cost,
		InputUncached: r.InputUncached + o.InputUncached,
		CacheRead:     r.CacheRead + o.CacheRead,
		CacheWrite:    r.CacheWrite + o.CacheWrite,
		Output:        r.Output + o.Output,
		WebSearches:   r.WebSearches + o.WebSearches,
	}
}
