// Package source defines what every harness reader provides, so the engine can
// read session logs without knowing which harness wrote them.
//
// Each harness has its own package under internal/adapters/source/ (for
// example claudecode) with a type that implements Reader. Readers are listed
// explicitly in cmd/trimloop, not registered on import, so tests can build
// exactly the set they need (docs/analyzer.md §10).
package source

import (
	"errors"
	"iter"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// A Reader reads one harness's session logs from disk.
type Reader interface {
	// Name identifies the harness, e.g. "claude-code". It is used by
	// --harness and as Log.Harness.
	Name() string

	// Discover finds the harness's logs on disk.
	Discover() ([]Log, error)

	// Read streams the messages in one log, in file order: responses with
	// usage (and the tools they call), tool results, compactions, titles,
	// text the harness added, records of the system prompt and tool list,
	// and the harness's own cost records. See Kind.
	//
	// Contract:
	//   - Read starts at log.Offset and moves it past each complete line it
	//     reads. The caller saves the new offset only after saving what was
	//     read.
	//   - Read stops before a final line with no newline (still being
	//     written); that line is read on the next run.
	//   - If the log is now shorter than log.Offset, it was replaced or cut
	//     short; Read starts again from the beginning.
	//   - Read yields every message line that carries usage, repeats
	//     included. The caller counts each message id once (ADR 0026), but
	//     collects the tool calls from every line with that id: a harness
	//     may log one content block per line.
	//   - A line Read cannot parse is yielded as an error wrapping ErrBadLine,
	//     and Read carries on. Any other error ends the log.
	Read(log *Log) iter.Seq2[Message, error]
}

// Log is one session log file to read.
type Log struct {
	Harness string // Name of the Reader that found it
	Path    string

	// Offset is the byte where reading starts, so a re-run reads only new
	// lines. Read moves it past each complete line. Discover always sets it
	// to 0: offsets are not saved between runs yet.
	Offset int64
}

// Message is one log line that matters to the analysis. It stands in for
// event.Raw until that type exists. Kind says which fields are set.
type Message struct {
	Kind Kind

	// Set for every kind.
	SessionID string    // the harness's session id, if it logs one
	Time      time.Time // when the line was logged; zero if not known
	// LineID identifies the log line itself, where the harness gives one. A
	// resumed or forked session copies earlier lines with their ids, so the
	// caller counts injected text and compactions once per LineID.
	LineID  string
	Project string // name of the project folder the line was logged in; "" if not known

	// KindTurn: a model response that carries usage.
	ID        string // provider message id; usage is counted once per id
	Model     string
	Usage     event.Usage
	Fast      bool             // fast mode, which changes the price
	USOnly    bool             // US-only inference, which changes the price
	Effort    string           // effort level requested ("medium", "high", "xhigh", "max"); "" if not recorded
	ToolCalls []event.ToolCall // tools called on this line, with no results yet
	// Attribution is what the harness says caused the turn.
	Attribution event.Attribution

	// KindToolResults: the results of earlier tool calls, and whether the
	// user decided on one of them (allowed or refused it when asked).
	Results     []Result
	UserDecided bool

	// KindCompaction: what the compaction did, as far as it was recorded.
	Compaction event.Compaction

	// KindTitle: the title, and whether the user set it (it then wins over
	// titles the harness generates).
	Title       string
	CustomTitle bool

	// KindInjected: the pieces of text added.
	Injected []event.Injected

	// KindStanding: the fixed start of later requests.
	Standing *event.Standing

	// KindLimitHit: which usage limit was reached: "session" (the 5-hour
	// limit), "weekly", or "other".
	Limit string

	// KindFailure: why the request failed: "sleep" (the computer slept
	// mid-response), "network" (a connection lost or a server error),
	// "not-sent" (refused before any work was done: an expired login, a rate
	// limit, no connection), or "other".
	Failure string

	// KindCostRecord: the record so far for one run of the harness. Runs
	// are told apart by RunID; within a run, later records include earlier
	// ones.
	RunID  string
	Record event.HarnessRecord
}

// Kind is the kind of line a Message holds.
type Kind int

// The kinds of line.
const (
	KindTurn        Kind = iota // a model response with usage
	KindToolResults             // results of tool calls, sent on the next turn
	KindCompaction              // the session was compacted: earlier context dropped
	KindTitle                   // the session's title, written by the harness or the user
	KindInjected                // text the harness added to the conversation
	KindStanding                // the system prompt and tool definitions, as recorded
	KindCostRecord              // the harness's running record of the session's cost
	KindPrompt                  // the user sent a message (only the fact is kept, not the text)
	KindLimitHit                // the harness refused a request: a usage limit was reached
	KindFailure                 // a request failed: the harness wrote an error reply itself
)

// Result is one tool result, linked to its call by CallID.
type Result struct {
	CallID  string
	Refused bool // the user refused the call when asked
	event.ToolResult
}

// ErrBadLine marks a line a Reader could not parse: either it is not valid
// JSON, or it is JSON in a shape the Reader does not expect. A rise in bad
// lines usually means the harness's log format has changed.
var ErrBadLine = errors.New("cannot parse line")
