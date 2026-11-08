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
	t, err := ReadTail(path)
	return t.Latest, err
}

// Tail is what the end of a session log says about where the session stands.
type Tail struct {
	Latest source.Message // the latest reply with usage

	// Writer is the latest reply before Latest that wrote to the cache, when
	// Latest wrote nothing: its writes say which cache lifetime the session
	// uses. Empty if Latest wrote to the cache itself, or no reply in the
	// tail did.
	Writer source.Message

	// Compaction is a compaction logged after Latest, if any: no reply has
	// shown the session's new size yet. Latest may then be empty, when no
	// reply is left in the tail.
	Compaction *source.Message
}

// ReadTail reads the end of a session log, so it is fast enough to run on
// every status-line refresh. A last line still being written is ignored.
func ReadTail(path string) (Tail, error) {
	var t Tail
	found := false
	f, err := os.Open(path)
	if err != nil {
		return t, err
	}
	defer func() { _ = f.Close() }() // read-only: closing cannot lose data
	info, err := f.Stat()
	if err != nil {
		return t, err
	}
	start := max(0, info.Size()-tailBytes)
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return t, err
	}
	// Drop an unfinished last line, and a first line cut by the start.
	if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
		buf = buf[:i]
	} else {
		return t, ErrNoResponse
	}
	if start > 0 {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		} else {
			return t, ErrNoResponse
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
		if err != nil || !ok {
			continue
		}
		if m.Kind == source.KindCompaction && t.Compaction == nil &&
			(!found || !m.Time.IsZero() && m.Time.After(t.Latest.Time)) {
			// After the latest reply, or before it in the log but later in
			// time: the reply that triggered a compaction can be logged after it.
			t.Compaction = &m
			continue
		}
		if m.Kind != source.KindTurn || m.Model == Synthetic {
			continue
		}
		wrote := m.Usage.CacheWrite5m+m.Usage.CacheWrite1h > 0
		if !found {
			t.Latest, found = m, true
			if wrote {
				return t, nil
			}
		} else if wrote {
			t.Writer = m
			return t, nil
		}
	}
	if !found && t.Compaction == nil {
		return Tail{}, ErrNoResponse
	}
	// A compaction with no reply in the tail (what it re-attached can run to
	// most of a megabyte) still says where the session stands.
	return t, nil
}
