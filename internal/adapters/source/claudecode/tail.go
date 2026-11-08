package claudecode

import (
	"bytes"
	"errors"
	"io"
	"os"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source"
)

// tailBytes is how much of the end of a log LastResponse reads. Claude Code
// writes a usage line after every reply, so the last one is near the end;
// a megabyte covers even a large tool result written after it.
const tailBytes = 1 << 20

// Synthetic is the model name Claude Code logs for a reply it made up itself,
// for example after an API error. It has no real usage, so it is not a turn.
const Synthetic = "<synthetic>"

// ErrNoResponse means the end of the log holds no reply with usage.
var ErrNoResponse = errors.New("no reply with usage near the end of the log")

// LastResponse returns the latest reply with usage in a session log, reading
// only the end of the file, so it is fast enough to run on every status-line
// refresh. A last line still being written is ignored.
func LastResponse(path string) (source.Message, error) {
	m, _, err := LastResponses(path)
	return m, err
}

// LastResponses returns the latest reply with usage and, when that reply
// wrote nothing to the cache, the latest one before it that did (its
// writes say which cache lifetime the session uses). writer is empty if
// the latest reply wrote to the cache itself, or none in the tail did.
func LastResponses(path string) (source.Message, source.Message, error) {
	var none, latest source.Message
	found := false
	f, err := os.Open(path)
	if err != nil {
		return none, none, err
	}
	defer func() { _ = f.Close() }() // read-only: closing cannot lose data
	info, err := f.Stat()
	if err != nil {
		return none, none, err
	}
	start := max(0, info.Size()-tailBytes)
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return none, none, err
	}
	// Drop an unfinished last line, and a first line cut by the start.
	if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
		buf = buf[:i]
	} else {
		return none, none, ErrNoResponse
	}
	if start > 0 {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		} else {
			return none, none, ErrNoResponse
		}
	}
	for len(buf) > 0 {
		line := buf
		if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
			line, buf = buf[i+1:], buf[:i]
		} else {
			buf = nil
		}
		m, ok, err := parseLine(bytes.TrimSpace(line))
		if err != nil || !ok || m.Kind != source.KindTurn || m.Model == Synthetic {
			continue
		}
		wrote := m.Usage.CacheWrite5m+m.Usage.CacheWrite1h > 0
		if !found {
			latest, found = m, true
			if wrote {
				return latest, none, nil
			}
		} else if wrote {
			return latest, m, nil
		}
	}
	if !found {
		return none, none, ErrNoResponse
	}
	return latest, none, nil
}
