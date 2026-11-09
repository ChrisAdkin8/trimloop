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
		!strings.Contains(out.String(), "status line: missing; run 'trimloop setup --write") ||
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
			[]string{"status line: out of date; run 'trimloop setup --write"}},
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
		if code := writeSetup(&out, &errs, settings, "/bin/trimloop", false); code != 0 ||
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
	if code := writeSetup(&out, &errs, settings, "/bin/trimloop", false); code != 1 ||
		!strings.Contains(errs.String(), "has more after its settings object") {
		t.Errorf("trailing data: code %d\n%s", code, errs.String())
	}
}
