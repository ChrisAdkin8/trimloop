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
}
