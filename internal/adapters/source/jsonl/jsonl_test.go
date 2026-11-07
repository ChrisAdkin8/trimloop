package jsonl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// parse is a minimal ParseFunc for these tests. Lines look like
// {"id":"a","out":1}; a line with "skip":true carries no usage.
func parse(line []byte) (source.Message, bool, error) {
	var v struct {
		ID   string `json:"id"`
		Out  int    `json:"out"`
		Skip bool   `json:"skip"`
	}
	if err := json.Unmarshal(line, &v); err != nil {
		return source.Message{}, false, err
	}
	if v.Skip {
		return source.Message{}, false, nil
	}
	return source.Message{ID: v.ID, Usage: event.Usage{Output: v.Out}}, true, nil
}

func line(id string, out int) string {
	return fmt.Sprintf(`{"id":%q,"out":%d}`+"\n", id, out)
}

func msg(id string, out int) source.Message {
	return source.Message{ID: id, Usage: event.Usage{Output: out}}
}

func writeLog(t *testing.T, content string) *source.Log {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return &source.Log{Path: p}
}

func appendLog(t *testing.T, log *source.Log, content string) {
	t.Helper()
	f, err := os.OpenFile(log.Path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil { // a write is only safe once Close succeeds
		t.Fatal(err)
	}
}

// readAll reads a log and returns its messages and bad-line errors. Any other
// error fails the test.
func readAll(t *testing.T, log *source.Log) ([]source.Message, []error) {
	t.Helper()
	var got []source.Message
	var bad []error
	for m, err := range Lines(log, parse) {
		if errors.Is(err, source.ErrBadLine) {
			bad = append(bad, err)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, m)
	}
	return got, bad
}

func TestLines(t *testing.T) {
	before := line("a", 1) + `{"skip":true}` + "\n" + line("a", 1) + "\n" // blank line
	rest := "not json\n" + line("b", 2)
	unfinished := `{"id":"c"`
	log := writeLog(t, before+rest+unfinished)

	got, bad := readAll(t, log)

	want := []source.Message{msg("a", 1), msg("a", 1), msg("b", 2)} // repeats are yielded
	if !reflect.DeepEqual(got, want) {
		t.Errorf("messages:\n got %+v\nwant %+v", got, want)
	}
	wantBad := fmt.Sprintf("at byte %d: cannot parse line: ", len(before))
	if len(bad) != 1 || !strings.HasPrefix(bad[0].Error(), wantBad) {
		t.Errorf("bad lines: got %v, want one starting %q", bad, wantBad)
	}
	if want := int64(len(before + rest)); log.Offset != want {
		t.Errorf("offset: got %d, want %d (just before the unfinished line)", log.Offset, want)
	}
}

func TestLinesResumesFromOffset(t *testing.T) {
	log := writeLog(t, line("a", 1)+line("b", 2)+`{"id":"c",`)

	got, _ := readAll(t, log)
	if want := []source.Message{msg("a", 1), msg("b", 2)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first run: got %+v, want %+v", got, want)
	}

	// The half-written line is finished and another is added.
	appendLog(t, log, `"out":3}`+"\n"+line("d", 4))

	got, _ = readAll(t, log)
	if want := []source.Message{msg("c", 3), msg("d", 4)}; !reflect.DeepEqual(got, want) {
		t.Errorf("second run: got %+v, want %+v", got, want)
	}

	// Nothing new: a third run reads nothing and leaves the offset alone.
	end := log.Offset
	got, _ = readAll(t, log)
	if len(got) != 0 || log.Offset != end {
		t.Errorf("third run: got %+v at offset %d, want nothing at offset %d", got, log.Offset, end)
	}
}

func TestLinesFileCutShort(t *testing.T) {
	log := writeLog(t, line("a", 1)+line("b", 2))
	readAll(t, log)

	// The file is replaced by a shorter one: start again from the beginning.
	if err := os.WriteFile(log.Path, []byte(line("x", 9)), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := readAll(t, log)
	if want := []source.Message{msg("x", 9)}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if want := int64(len(line("x", 9))); log.Offset != want {
		t.Errorf("offset: got %d, want %d", log.Offset, want)
	}
}

func TestLinesMissingFile(t *testing.T) {
	for _, err := range Lines(&source.Log{Path: filepath.Join(t.TempDir(), "gone.jsonl")}, parse) {
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("got %v, want a not-exist error", err)
		}
		return
	}
	t.Error("got no error for a missing file")
}

func TestLinesStopsEarly(t *testing.T) {
	log := writeLog(t, line("a", 1)+line("b", 2))
	n := 0
	for _, err := range Lines(log, parse) {
		if err != nil {
			t.Fatal(err)
		}
		n++
		break
	}
	if n != 1 {
		t.Errorf("got %d messages after break, want 1", n)
	}
	// Only the first line was used, so the next run starts at the second.
	if want := int64(len(line("a", 1))); log.Offset != want {
		t.Errorf("offset: got %d, want %d", log.Offset, want)
	}
}
