// Package jsonl is the line-reading helper shared by every reader whose
// harness writes JSON Lines logs. It handles offsets, unfinished last lines,
// replaced files and bad lines, so each reader only parses one line at a time.
package jsonl

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source"
)

// ParseFunc turns one non-blank line into a Message. It returns ok=false for
// a line the analysis does not use (to be skipped), and an error for a line
// it cannot parse.
type ParseFunc func(line []byte) (m source.Message, ok bool, err error)

// Lines streams the messages in log, using parse for each line. It meets the
// Read contract in source.Reader.
func Lines(log *source.Log, parse ParseFunc) iter.Seq2[source.Message, error] {
	return func(yield func(source.Message, error) bool) {
		f, err := os.Open(log.Path)
		if err != nil {
			yield(source.Message{}, err)
			return
		}
		defer func() { _ = f.Close() }() // read-only: closing cannot lose data

		info, err := f.Stat()
		if err != nil {
			yield(source.Message{}, err)
			return
		}
		if info.Size() < log.Offset {
			log.Offset = 0 // replaced or cut short: start again
		}
		if _, err := f.Seek(log.Offset, io.SeekStart); err != nil {
			yield(source.Message{}, err)
			return
		}
		r := bufio.NewReaderSize(f, 1<<20) // 1 MB buffer; grows for longer lines

		for {
			start := log.Offset
			line, err := r.ReadBytes('\n')
			if errors.Is(err, io.EOF) {
				return // any bytes here are an unfinished last line
			}
			if err != nil {
				yield(source.Message{}, err)
				return
			}
			log.Offset += int64(len(line)) // the line is complete: move past it

			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			m, ok, err := parse(line)
			if err != nil {
				if !yield(source.Message{}, fmt.Errorf("at byte %d: %w: %w", start, source.ErrBadLine, err)) {
					return
				}
				continue
			}
			if !ok {
				continue
			}
			if !yield(m, nil) {
				return
			}
		}
	}
}
