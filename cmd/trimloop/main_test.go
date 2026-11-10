package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/internal/report"
)

var t0 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

// write sends HTML to a file and prints a summary; an unwritable path fails.
func TestWrite(t *testing.T) {
	r := report.Analyse(nil, report.Read{Logs: 1}, report.Local{}, report.DefaultSettings(), t0)
	path := filepath.Join(t.TempDir(), "r.html")
	if got, err := write(r, "html", path); err != nil || got != path {
		t.Fatalf("write: got %q, %v; want %q", got, err, path)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("report file mode: got %v, %v; want 0600", info.Mode().Perm(), err)
	}
	if b, err := os.ReadFile(path); err != nil || !strings.Contains(string(b), "<title>Trimloop report</title>") {
		t.Errorf("got err %v, page %.40q", err, b)
	}
	if _, err := write(r, "json", filepath.Join(t.TempDir(), "missing", "r.json")); err == nil {
		t.Error("got no error writing into a missing folder")
	}
}

// Writing over an existing, readable report makes it private.
func TestWriteMakesExistingReportPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := report.Analyse(nil, report.Read{}, report.Local{}, report.DefaultSettings(), t0)
	if _, err := write(r, "json", path); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("got %v, want 0600", info.Mode().Perm())
	}
}

// A link at --out is replaced, not followed, so a link planted in a shared
// folder cannot make trimloop overwrite the file it points to.
func TestWriteReplacesLinks(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "report.json")
	if err := os.Symlink(victim, path); err != nil {
		t.Skip("no symlinks here:", err)
	}
	r := report.Analyse(nil, report.Read{}, report.Local{}, report.DefaultSettings(), t0)
	if _, err := write(r, "json", path); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Errorf("the linked file was overwritten: %q", b)
	}
	if info, _ := os.Stat(victim); info.Mode().Perm() != 0o644 {
		t.Errorf("the linked file's mode changed to %v", info.Mode().Perm())
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("report: got %v, %v; want a private file", info, err)
	}
	if b, _ := os.ReadFile(path); !json.Valid(b) {
		t.Errorf("report is not JSON: %.40q", b)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*.tmp")); len(left) > 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

// Anything at --out but a file or a link is refused, and nothing is left
// behind.
func TestWriteRefusesOtherThings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	r := report.Analyse(nil, report.Read{}, report.Local{}, report.DefaultSettings(), t0)
	if _, err := write(r, "json", path); err == nil || !strings.Contains(err.Error(), "is not a file") {
		t.Errorf("got %v, want a refusal", err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*")); len(left) > 0 {
		t.Errorf("files left: %v", left)
	}
}

// --out - and --out /dev/stdout write to standard output, and --out
// /dev/null throws the report away; none of them makes a file.
func TestWriteSpecialNames(t *testing.T) {
	t.Chdir(t.TempDir())
	r := report.Analyse(nil, report.Read{}, report.Local{}, report.DefaultSettings(), t0)
	for _, path := range []string{"-", "/dev/stdout", "/dev/null"} {
		out := captureStdout(t, func() {
			if got, err := write(r, "json", path); err != nil || got != "" {
				t.Errorf("%s: got %q, %v; want no file and no error", path, got, err)
			}
		})
		if wantJSON := path != "/dev/null"; json.Valid([]byte(out)) != wantJSON || (out != "") != wantJSON {
			t.Errorf("%s: standard output %.40q", path, out)
		}
	}
	if left, _ := filepath.Glob("*"); len(left) > 0 {
		t.Errorf("files made: %v", left)
	}
}

// captureStdout returns what f writes to standard output.
func captureStdout(t *testing.T, f func()) string { return capture(t, &os.Stdout, f) }

// capture returns what f writes to *file, standard output or standard
// error.
func capture(t *testing.T, file **os.File, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := *file
	*file = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { *file = old }()
	f()
	_ = w.Close()
	return <-done
}

func TestDefaultHistory(t *testing.T) {
	if got := defaultHistory("/h", ""); got != "/h/.trimloop/history.json" {
		t.Errorf("default logs: got %q", got)
	}
	if got := defaultHistory("/h", "/h/.claude/"); got != "/h/.trimloop/history.json" {
		t.Errorf("the default folder named explicitly: got %q", got)
	}
	a, b := defaultHistory("/h", "/x/one"), defaultHistory("/h", "/x/two")
	if a == b || !strings.HasPrefix(a, "/h/.trimloop/history-") || a != defaultHistory("/h", "/x/one/") {
		t.Errorf("other folders: got %q and %q", a, b)
	}
}

func TestPartialWeek(t *testing.T) {
	if partialWeek(time.Time{}) != "" {
		t.Error("no logs: want no partial week")
	}
	wed := time.Date(2026, 9, 23, 12, 0, 0, 0, time.Local)
	if got := partialWeek(wed); got != "2026-09-21" {
		t.Errorf("got %q, want the Monday of that week", got)
	}
}
