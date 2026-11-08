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
	f, err := os.Open(path)
	if err != nil {
		return source.Message{}, err
	}
	defer func() { _ = f.Close() }() // read-only: closing cannot lose data
	info, err := f.Stat()
	if err != nil {
		return source.Message{}, err
	}
	start := max(0, info.Size()-tailBytes)
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return source.Message{}, err
	}
	// Drop an unfinished last line, and a first line cut by the start.
	if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
		buf = buf[:i]
	} else {
		return source.Message{}, ErrNoResponse
	}
	if start > 0 {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		} else {
			return source.Message{}, ErrNoResponse
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
		if err == nil && ok && m.Kind == source.KindTurn && m.Model != Synthetic {
			return m, nil
		}
	}
	return source.Message{}, ErrNoResponse
}
