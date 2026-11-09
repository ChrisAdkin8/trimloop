package live

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain points HOME at a scratch folder for the whole package, so the
// backups WriteSettings makes in ~/.trimloop/backups never land in the
// real home folder.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "trimloop-live-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Setenv("HOME", home); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

// A backup holds secrets as much as the settings do: it is readable by the
// user only, in ~/.trimloop/backups, never next to the settings file (which
// may be a link into a dotfiles repository), and named after the file it
// copies. Settings of the same name in two folders get separate backups.
func TestWriteSettingsBackup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := `{"env":{"ANTHROPIC_API_KEY":"sk-test"},"apiKeyHelper":"x"}`
	if err := os.WriteFile(target, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	backup, changed, err := WriteSettings(link, "/bin/trimloop", false)
	if err != nil || !changed || backup == "" {
		t.Fatalf("got %q, %v, %v", backup, changed, err)
	}

	backups := filepath.Join(home, ".trimloop", "backups")
	if filepath.Dir(backup) != backups {
		t.Errorf("backup in %s, want %s", filepath.Dir(backup), backups)
	}
	if !strings.HasPrefix(filepath.Base(backup), "settings.json-") || !strings.Contains(backup, ".trimloop-backup-") {
		t.Errorf("backup name %q does not say which file it copies", filepath.Base(backup))
	}
	if fi, err := os.Stat(backup); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("backup mode: %v (%v), want 0600", fi, err)
	}
	if fi, err := os.Stat(backups); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("backups folder mode: %v (%v), want 0700", fi, err)
	}
	if b, _ := os.ReadFile(backup); string(b) != orig {
		t.Errorf("backup holds %q, want the old file", b)
	}
	for _, d := range []string{dir, filepath.Dir(target)} {
		entries, _ := os.ReadDir(d)
		for _, e := range entries {
			if e.Name() != "settings.json" && e.Name() != "dotfiles" {
				t.Errorf("%s left beside the settings file in %s", e.Name(), d)
			}
		}
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o644 {
		t.Errorf("settings mode: got %v, want 0644 kept", fi.Mode().Perm())
	}

	// Another folder's settings.json, backed up in the same second, gets its
	// own backup: a different id, and the first is untouched.
	other := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(other, []byte(`{"model":"opus"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	b2, _, err := WriteSettings(other, "/bin/trimloop", false)
	if err != nil || filepath.Dir(b2) != backups {
		t.Fatalf("second backup: %q, %v", b2, err)
	}
	id := func(p string) string { return strings.SplitN(filepath.Base(p), ".trimloop-backup-", 2)[0] }
	if id(b2) == id(backup) {
		t.Errorf("backups of two files share an id: %s, %s", backup, b2)
	}
	if b, _ := os.ReadFile(backup); string(b) != orig {
		t.Error("the first backup was changed")
	}
}

// A settings path that is a link to a file not made yet is followed: the
// new file is made where the link points, and the link stays.
func TestWriteSettingsThroughDanglingLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dot", "settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(filepath.Join("dot", "settings.json"), link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteSettings(link, "/usr/local/bin/trimloop", false); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced: %v %v", fi, err)
	}
	if b, err := os.ReadFile(target); err != nil || !strings.Contains(string(b), "statusLine") {
		t.Errorf("target: %q %v", b, err)
	}
}
