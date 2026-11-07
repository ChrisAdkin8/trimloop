package event

import "time"

// Turn is one model request and its response: a subset of the Turn entity in
// docs/data-model.md, holding only what the first findings need.
type Turn struct {
	ID    string // provider response or message id
	Seq   int    // order within the session, from 0
	Model string
	Time  time.Time // when the response was first logged

	// IdleBefore is how long the session was idle before this turn: from the
	// last activity of the previous turn to this one. It is 0 for the first
	// turn of a session (see First).
	IdleBefore time.Duration
	First      bool // no earlier activity in this session

	Usage  Usage
	Cost   Cost
	Priced bool // false if the price catalogue has no price for Model
	Fast   bool // fast mode, which changes the price
	USOnly bool // US-only inference, which changes the price

	// Project is the name of the project folder the turn ran in; "" if not
	// known. A session can move between projects.
	Project string

	// Attribution is what the harness says caused the turn, if anything.
	Attribution Attribution

	// ToolCalls are the tools this turn called. Their results are sent to
	// the model from the next turn of the session onwards.
	ToolCalls []ToolCall

	// CompactedBefore is true when the session was compacted just before
	// this turn: earlier tool results are no longer in the context.
	// Compaction describes it, as far as the harness recorded it.
	CompactedBefore bool
	Compaction      Compaction
}

// Attribution names the skill, plugin, MCP server or agent type a turn's
// spend belongs to, as the harness reports it. Empty fields mean none.
type Attribution struct {
	Skill     string
	Plugin    string
	MCPServer string
	Agent     string // agent type, e.g. "general-purpose", for sub-agent turns
}

// Compaction is one compaction of a session's context.
type Compaction struct {
	Before   int           // context tokens before; 0 if not recorded
	After    int           // context tokens after; 0 if not recorded
	Manual   bool          // the user asked for it, rather than the harness
	Duration time.Duration // how long it took; 0 if not recorded
}

// Context is the number of input tokens the turn sent, cached or not.
func (t Turn) Context() int { return t.Usage.InputTotal() }

// Session is one conversation with its own cache: a main session or one
// subagent run. Each has its own cached prefix, so cache findings are worked
// out per Session.
type Session struct {
	ID       string // display id: session id, or session/agent for a subagent
	Kind     string // "main" or "subagent"
	ParentID string // for a subagent, the main session's id
	Turns    []Turn // in time order

	// Project is the project of the session's first turn that names one; a
	// subagent with none takes its main session's.
	Project string
	// AgentType is a subagent's type, e.g. "general-purpose"; "" if unknown.
	AgentType string
}

// Conversation is the ID of the main session a session belongs to: its own
// ID for a main session, its parent's for a subagent.
func (s Session) Conversation() string {
	if s.ParentID != "" {
		return s.ParentID
	}
	return s.ID
}
