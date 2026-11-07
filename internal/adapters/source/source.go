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

	// Read streams the messages with usage in one log, in file order.
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
	//     included. The caller counts each message id once (ADR 0026).
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

// Message is one message that carries usage. It stands in for event.Raw
// until that type exists.
type Message struct {
	ID        string    // provider message id; usage is counted once per id
	SessionID string    // the harness's session id, if it logs one
	Time      time.Time // when the line was logged; zero if not known
	Model     string
	Usage     event.Usage
	Fast      bool // fast mode, which changes the price
	USOnly    bool // US-only inference, which changes the price
}

// ErrBadLine marks a line a Reader could not parse: either it is not valid
// JSON, or it is JSON in a shape the Reader does not expect. A rise in bad
// lines usually means the harness's log format has changed.
var ErrBadLine = errors.New("cannot parse line")
