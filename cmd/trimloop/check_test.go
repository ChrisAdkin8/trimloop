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
	cfg := `{"statusLine":{"command":"'/bin/trimloop' statusline"},"hooks":{` +
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
