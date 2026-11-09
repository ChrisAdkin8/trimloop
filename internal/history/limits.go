package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
)

// LimitsPath is where status-line readings of usage limits are kept
// (ADR 0048): one file per Claude Code folder, like the history file.
func LimitsPath(home, configDir string) string {
	return PathFor(home, configDir, "limits", ".jsonl")
}

// AppendLimits adds a reading to the file at path, one JSON line, when it
// moves a limit forward from the last line kept: a higher use in the same
// window, or a later window. The status line runs every few seconds in every
// open session, and a session that has been idle can repeat an older
// reading, so anything else is dropped; the file then grows by at most one
// line per percentage point per window. A limit in the last line whose reset
// time is implausibly far after the new reading (stored before readings were
// checked) counts as absent, so it cannot hold back every later reading; the
// line itself is left in the file. It holds numbers and times only, readable
// by the owner only.
func AppendLimits(path string, r detectors.LimitReading) error {
	if r.FiveHour == nil && r.SevenDay == nil {
		return nil
	}
	if last, ok := lastReading(path); ok {
		lastFive, lastSeven := plausible(last.FiveHour, r.At), plausible(last.SevenDay, r.At)
		if !ahead(r.FiveHour, lastFive) && !ahead(r.SevenDay, lastSeven) {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// LoadLimits reads every reading in the file at path. A missing file has
// none; lines that cannot be read are skipped.
func LoadLimits(path string) ([]detectors.LimitReading, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []detectors.LimitReading
	br := bufio.NewReader(f) // no line-length limit: one damaged line must not hide the rest
	for {
		line, err := br.ReadBytes('\n')
		var r detectors.LimitReading
		if len(line) > 0 && json.Unmarshal(line, &r) == nil && !r.At.IsZero() {
			out = append(out, r)
		}
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
	}
}

// lastReading reads the last line of the file at path.
func lastReading(path string) (detectors.LimitReading, bool) {
	var r detectors.LimitReading
	f, err := os.Open(path)
	if err != nil {
		return r, false
	}
	defer func() { _ = f.Close() }()
	const tail = 1024 // far longer than one line
	info, err := f.Stat()
	if err != nil {
		return r, false
	}
	start := max(info.Size()-tail, 0)
	b := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(b, start); err != nil && !errors.Is(err, io.EOF) {
		return r, false
	}
	b = trimNewline(b)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == '\n' {
			b = b[i+1:]
			break
		}
	}
	return r, json.Unmarshal(b, &r) == nil
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

// plausible returns w, or nil when its reset time is more than
// detectors.ResetsAtFuture after at (the weekly window plus slack, the same
// bound the status line's decoder puts on new readings): version 0.1.0
// stored readings unchecked, so a file can end with a reset time in
// milliseconds or in the year 9999. A zero at checks nothing. Only the later
// bound is applied here, as before; FindLimits applies both to the rate.
func plausible(w *detectors.LimitWindow, at time.Time) *detectors.LimitWindow {
	if w == nil || at.IsZero() || w.ResetsAt <= at.Add(detectors.ResetsAtFuture).Unix() {
		return w
	}
	return nil
}

// ahead reports whether reading r of one limit moves it forward from last:
// it is the first, a later window, or the same window used more.
func ahead(r, last *detectors.LimitWindow) bool {
	switch {
	case r == nil:
		return false
	case last == nil:
		return true
	case detectors.SameWindow(r.ResetsAt, last.ResetsAt):
		return r.Used > last.Used
	}
	return r.ResetsAt > last.ResetsAt
}
