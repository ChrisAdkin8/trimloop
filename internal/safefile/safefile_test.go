package safefile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// leftovers lists the temporary files left in dir.
func leftovers(t *testing.T, dir string) []string {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(dir, ".*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	return left
}

func TestWriteReplacesWithPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteBytes(path, 0o640, []byte("new")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	info, statErr := os.Stat(path)
	if err != nil || statErr != nil || string(b) != "new" || info.Mode().Perm() != 0o640 {
		t.Errorf("got %q, %v, mode %v", b, err, info.Mode().Perm())
	}
	if left := leftovers(t, dir); len(left) > 0 {
		t.Errorf("left behind: %v", left)
	}
}

// A render that fails leaves the old file and no new one.
func TestWriteRenderFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	if err := Write(path, 0o600, func(io.Writer) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("got %v, want boom", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "old" {
		t.Errorf("file changed to %q", b)
	}
	if left := leftovers(t, dir); len(left) > 0 {
		t.Errorf("left behind: %v", left)
	}
}

// A rename that fails (here: path is a folder) removes the new file too.
func TestWriteRenameFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "taken")
	if err := os.MkdirAll(filepath.Join(path, "inside"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteBytes(path, 0o600, []byte("new")); err == nil {
		t.Fatal("got no error renaming over a folder that is not empty")
	}
	if left := leftovers(t, dir); len(left) > 0 {
		t.Errorf("left behind: %v", left)
	}
}

// A link at path is replaced by the file, not followed.
func TestWriteReplacesLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "link")
	if err := os.Symlink(target, path); err != nil {
		t.Skip("no symlinks here:", err)
	}
	if err := WriteBytes(path, 0o600, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "keep" {
		t.Errorf("the link's target changed to %q", b)
	}
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		t.Errorf("path is not a plain file now: %v, %v", info, err)
	}
}
