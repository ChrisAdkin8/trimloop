package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckSetup(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, "settings.json")
	var out bytes.Buffer
	if code := checkSetup(&out, settings, "/bin/trimloop", home, ""); code != 1 ||
		!strings.Contains(out.String(), "status line: missing; to fix it, run: trimloop setup --write "+settings) ||
		!strings.Contains(out.String(), "Usage-limit readings: none (only Pro and Max") {
		t.Errorf("nothing set up: code %d\n%s", code, out.String())
	}
	cfg := `{"statusLine":{"type":"command","command":"'/bin/trimloop' statusline","refreshInterval":30},"hooks":{` +
		`"Stop":[{"hooks":[{"type":"command","command":"'/bin/trimloop' hook stop","timeout":5}]}],` +
		`"UserPromptSubmit":[{"hooks":[{"type":"command","command":"'/bin/trimloop' hook prompt","timeout":5}]}]}}`
	if err := os.WriteFile(settings, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".trimloop"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".trimloop", "limits.jsonl"),
		[]byte(`{"time":"2026-11-08T09:00:00Z","seven_day":{"used":5,"resets_at":1800000000}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := checkSetup(&out, settings, "/bin/trimloop", home, ""); code != 0 ||
		strings.Count(out.String(), "up to date") != 3 || !strings.Contains(out.String(), "Usage-limit readings: 1 kept") {
		t.Errorf("all set up: code %d\n%s", code, out.String())
	}
}

// A status line without today's refresh interval needs --write; hooks
// turned off in the settings are reported, not called up to date.
func TestCheckSetupProblems(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, "settings.json")
	hooks := `"hooks":{` +
		`"Stop":[{"hooks":[{"type":"command","command":"'/bin/trimloop' hook stop","timeout":5}]}],` +
		`"UserPromptSubmit":[{"hooks":[{"type":"command","command":"'/bin/trimloop' hook prompt","timeout":5}]}]}`
	current := `"statusLine":{"type":"command","command":"'/bin/trimloop' statusline","refreshInterval":30}`
	for _, c := range []struct {
		name, cfg string
		code      int
		want      []string
	}{
		{"no refresh interval", `{"statusLine":{"type":"command","command":"'/bin/trimloop' statusline"},` + hooks + `}`, 1,
			[]string{"status line: out of date; to fix it, run: trimloop setup --write " + settings}},
		{"hooks turned off", `{` + current + `,` + hooks + `,"disableAllHooks":true}`, 1,
			[]string{"hooks are turned off", `"disableAllHooks": true`}},
		{"hooks on", `{` + current + `,` + hooks + `,"disableAllHooks":false}`, 0, nil},
	} {
		if err := os.WriteFile(settings, []byte(c.cfg), 0o600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		code := checkSetup(&out, settings, "/bin/trimloop", home, "")
		if code != c.code {
			t.Errorf("%s: code %d, want %d\n%s", c.name, code, c.code, out.String())
		}
		for _, w := range c.want {
			if !strings.Contains(out.String(), w) {
				t.Errorf("%s: no %q in\n%s", c.name, w, out.String())
			}
		}
	}
}

// --write says when there was nothing to change, rather than claiming it
// added the settings again.
func TestWriteSetup(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "settings.json")
	for _, c := range []struct {
		name, want string
	}{
		{"first run", "Added trimloop's status line and hooks to "},
		{"second run", "are already up to date; nothing changed."},
	} {
		var out, errs bytes.Buffer
		if code := writeSetup(&out, &errs, settings, "/bin/trimloop", false, "", ""); code != 0 ||
			!strings.Contains(out.String(), c.want) || errs.Len() != 0 {
			t.Errorf("%s: code %d\n%s%s", c.name, code, out.String(), errs.String())
		}
		if c.name == "second run" && strings.Contains(out.String(), "Added") {
			t.Errorf("%s: claims it added the settings: %s", c.name, out.String())
		}
	}
	if err := os.WriteFile(settings, []byte(`{"model":"opus"} {"second":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if code := writeSetup(&out, &errs, settings, "/bin/trimloop", false, "", ""); code != 1 ||
		!strings.Contains(errs.String(), "has more after its settings object") {
		t.Errorf("trailing data: code %d\n%s", code, errs.String())
	}
}

// --write says how to undo what it did, warns when CLAUDE_CONFIG_DIR means
// Claude Code reads another file, and, when another program's status line
// is there, says nothing was written and gives both ways forward.
func TestWriteSetupMessages(t *testing.T) {
	const other = `{"statusLine":{"type":"command","command":"mine.sh"}}`
	for _, c := range []struct {
		name      string
		existing  string // "" for no earlier file
		inClaude  bool   // the file is in ~/.claude
		cfgDir    bool   // CLAUDE_CONFIG_DIR points elsewhere
		code      int
		out, errs []string
		notErrs   []string
	}{
		{name: "new file", code: 0,
			out: []string{"There was no settings file before; to undo this, delete "}},
		{name: "earlier file", existing: `{"model":"opus"}`, code: 0,
			out: []string{"To undo this, put the previous file back: cp ", ".trimloop-backup-"}},
		{name: "another status line", existing: other, code: 1,
			errs: []string{"Nothing was written, hooks included", "--replace-statusline",
				"keep your status line and add only trimloop's hooks", `"Stop"`, `"UserPromptSubmit"`},
			notErrs: []string{`"statusLine"`}},
		{name: "config dir elsewhere", inClaude: true, cfgDir: true, code: 0,
			errs: []string{"Warning: CLAUDE_CONFIG_DIR is set, so Claude Code reads ", "run: trimloop setup --write "}},
		{name: "config dir, file in it", cfgDir: true, code: 0, notErrs: []string{"Warning"}},
	} {
		home := t.TempDir()
		t.Setenv("HOME", home)                    // setup keeps backups in ~/.trimloop/backups
		dir := filepath.Join(home, "my settings") // a space, so paths are quoted
		if c.inClaude {
			dir = filepath.Join(home, ".claude")
		}
		cfgDir := ""
		if c.cfgDir {
			cfgDir = filepath.Join(home, "config")
			if !c.inClaude {
				dir = cfgDir
			}
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		settings := filepath.Join(dir, "settings.json")
		if c.existing != "" {
			if err := os.WriteFile(settings, []byte(c.existing), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		var out, errs bytes.Buffer
		code := writeSetup(&out, &errs, settings, "/bin/trimloop", false, home, cfgDir)
		if code != c.code {
			t.Errorf("%s: code %d, want %d\n%s%s", c.name, code, c.code, out.String(), errs.String())
		}
		for _, w := range c.out {
			if !strings.Contains(out.String(), w) {
				t.Errorf("%s: no %q in\n%s", c.name, w, out.String())
			}
		}
		for _, w := range c.errs {
			if !strings.Contains(errs.String(), w) {
				t.Errorf("%s: no %q in\n%s", c.name, w, errs.String())
			}
		}
		for _, w := range c.notErrs {
			if strings.Contains(errs.String(), w) {
				t.Errorf("%s: %q in\n%s", c.name, w, errs.String())
			}
		}
		if strings.Contains(dir, " ") && code == 0 && !strings.Contains(out.String(), "'"+settings+"'") &&
			!strings.Contains(out.String(), "delete "+settings) {
			t.Errorf("%s: path not quoted in\n%s", c.name, out.String())
		}
		if c.existing == other {
			if b, _ := os.ReadFile(settings); string(b) != other {
				t.Errorf("%s: the file changed: %s", c.name, b)
			}
		}
	}
}

// With another program's status line, --check does not suggest a --write
// that would refuse; it gives the same two choices as --write. It warns
// when the file is not the one Claude Code reads.
func TestCheckSetupAdvice(t *testing.T) {
	for _, c := range []struct {
		name, cfg      string
		inClaude, cfgD bool
		want, notWant  []string
	}{
		{name: "another status line", cfg: `{"statusLine":{"type":"command","command":"mine.sh"}}`,
			want: []string{"status line: another program's; trimloop's status line is not shown (see the choices below)",
				"(Stop): missing; see the choices below", "would refuse and write nothing",
				"--replace-statusline", "add only trimloop's hooks"},
			notWant: []string{"to fix it, run"}},
		{name: "another status line, hooks done", cfg: `{"statusLine":{"type":"command","command":"mine.sh"},"hooks":{` +
			`"Stop":[{"hooks":[{"type":"command","command":"'/bin/trimloop' hook stop","timeout":5}]}],` +
			`"UserPromptSubmit":[{"hooks":[{"type":"command","command":"'/bin/trimloop' hook prompt","timeout":5}]}]}}`,
			want:    []string{"trimloop's hooks are in place", "--replace-statusline"},
			notWant: []string{"add only trimloop's hooks", "to fix it, run"}},
		{name: "nothing set up", cfg: `{}`,
			want:    []string{"(Stop): missing; to fix it, run: trimloop setup --write "},
			notWant: []string{"see the choices below", "Warning"}},
		{name: "config dir elsewhere", cfg: `{}`, inClaude: true, cfgD: true,
			want: []string{"Warning: CLAUDE_CONFIG_DIR is set, so Claude Code reads ", "run: trimloop setup --check"}},
	} {
		home := t.TempDir()
		dir := home
		if c.inClaude {
			dir = filepath.Join(home, ".claude")
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		cfgDir := ""
		if c.cfgD {
			cfgDir = filepath.Join(home, "config")
		}
		settings := filepath.Join(dir, "settings.json")
		if err := os.WriteFile(settings, []byte(c.cfg), 0o600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if code := checkSetup(&out, settings, "/bin/trimloop", home, cfgDir); code != 1 {
			t.Errorf("%s: code %d, want 1", c.name, code)
		}
		for _, w := range c.want {
			if !strings.Contains(out.String(), w) {
				t.Errorf("%s: no %q in\n%s", c.name, w, out.String())
			}
		}
		for _, w := range c.notWant {
			if strings.Contains(out.String(), w) {
				t.Errorf("%s: %q in\n%s", c.name, w, out.String())
			}
		}
	}
}

// A file in ~/.claude is warned about only when CLAUDE_CONFIG_DIR points
// Claude Code at another folder; a project's settings never are.
func TestUnreadSettings(t *testing.T) {
	home := t.TempDir()
	claude := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claude, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "link")
	if err := os.Symlink(claude, link); err != nil {
		t.Fatal(err)
	}
	inClaude := filepath.Join(claude, "settings.json")
	for _, c := range []struct {
		name, path, cfgDir, want string
	}{
		{"no config dir", inClaude, "", ""},
		{"config dir elsewhere", inClaude, "/cfg", "/cfg/settings.json"},
		{"local settings too", filepath.Join(claude, "settings.local.json"), "/cfg", "/cfg/settings.json"},
		{"config dir is ~/.claude", inClaude, claude, ""},
		{"config dir links to ~/.claude", inClaude, link, ""},
		{"the file it reads", "/cfg/settings.json", "/cfg", ""},
		{"a project's settings", "/work/app/.claude/settings.json", "/cfg", ""},
	} {
		if got := unreadSettings(c.path, home, c.cfgDir); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// ~/.claude/settings.json that links to the file in CLAUDE_CONFIG_DIR is
// the file Claude Code reads, whether that file is made yet or not; a link
// elsewhere is still warned about.
func TestUnreadSettingsFollowsLinks(t *testing.T) {
	home, cfg, other := t.TempDir(), t.TempDir(), t.TempDir()
	claude := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claude, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(claude, "settings.json")
	reads := filepath.Join(cfg, "settings.json")
	link := func(target string) {
		_ = os.Remove(path)
		if err := os.Symlink(target, path); err != nil {
			t.Skip("no symlinks here:", err)
		}
	}
	link(reads) // not made yet
	if got := unreadSettings(path, home, cfg); got != "" {
		t.Errorf("link to a file not made yet: got %q, want no warning", got)
	}
	if err := os.WriteFile(reads, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := unreadSettings(path, home, cfg); got != "" {
		t.Errorf("link to the file: got %q, want no warning", got)
	}
	rel, err := filepath.Rel(claude, reads)
	if err != nil {
		t.Fatal(err)
	}
	link(rel)
	if got := unreadSettings(path, home, cfg); got != "" {
		t.Errorf("relative link: got %q, want no warning", got)
	}
	// A link to a link to the file not made yet: setup --write writes the
	// file Claude Code reads, so there is nothing to warn about.
	if err := os.Remove(reads); err != nil {
		t.Fatal(err)
	}
	middle := filepath.Join(other, "middle.json")
	if err := os.Symlink(reads, middle); err != nil {
		t.Fatal(err)
	}
	link(middle)
	if got := unreadSettings(path, home, cfg); got != "" {
		t.Errorf("link to a link to a file not made yet: got %q, want no warning", got)
	}
	link(filepath.Join(other, "settings.json"))
	if got := unreadSettings(path, home, cfg); got != reads {
		t.Errorf("link elsewhere: got %q, want %q", got, reads)
	}
}
