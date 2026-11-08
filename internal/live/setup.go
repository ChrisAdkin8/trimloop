package live

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Settings Claude Code needs to run the nudges. The status line refreshes
// every 30 seconds so the cache countdown moves while the user is away.
func claudeSettings(exe string) map[string]any {
	hook := func(event string, timeout int) []any {
		return []any{map[string]any{"hooks": []any{map[string]any{
			"type": "command", "command": fmt.Sprintf("%q hook %s", exe, event), "timeout": timeout,
		}}}}
	}
	return map[string]any{
		"statusLine": map[string]any{"type": "command", "command": fmt.Sprintf("%q statusline", exe), "refreshInterval": 30},
		"hooks": map[string]any{
			"Stop":             hook("stop", 5),
			"UserPromptSubmit": hook("prompt", 5),
		},
	}
}

// SetupSnippet is the JSON to add to Claude Code's settings for the nudges.
func SetupSnippet(exe string) string {
	b, _ := json.MarshalIndent(claudeSettings(exe), "", "  ")
	return string(b)
}

// ErrStatusLineTaken means the settings already have a different status line.
var ErrStatusLineTaken = errors.New("the settings already have a status line; use --replace-statusline to replace it")

// WriteSettings adds the nudges to the Claude Code settings file at path,
// keeping everything else in it, after copying it to a backup. It does not
// replace an existing status line unless replace is true, does not add a
// hook that is already there, and changes nothing if nothing is missing. It
// refuses, rather than guesses, when the file or its "hooks" are not shaped
// as Claude Code expects. The file is replaced atomically, and other values
// keep their exact text (numbers and characters such as & < >); only the
// order of keys may change. It returns the backup's path ("" if none).
func WriteSettings(path, exe string, replace bool) (string, error) {
	cfg := map[string]any{}
	old, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		old = nil
	case err != nil:
		return "", err
	default:
		dec := json.NewDecoder(bytes.NewReader(old))
		dec.UseNumber() // keep large numbers exact
		var v any
		if err := dec.Decode(&v); err != nil {
			return "", fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
		m, ok := v.(map[string]any)
		if !ok {
			return "", fmt.Errorf("%s does not hold a settings object; not changing it", path)
		}
		cfg = m
	}

	want := claudeSettings(exe)
	changed := false
	if sl, ok := cfg["statusLine"]; ok && !sameCommand(sl, want["statusLine"]) {
		if !replace {
			return "", ErrStatusLineTaken
		}
		cfg["statusLine"], changed = want["statusLine"], true
	} else if !ok {
		cfg["statusLine"], changed = want["statusLine"], true
	}

	hooks := map[string]any{}
	if h, ok := cfg["hooks"]; ok {
		if hooks, ok = h.(map[string]any); !ok {
			return "", fmt.Errorf("%s: \"hooks\" is not an object; not changing it", path)
		}
	}
	for event, entries := range want["hooks"].(map[string]any) {
		var list []any
		if l, ok := hooks[event]; ok {
			if list, ok = l.([]any); !ok {
				return "", fmt.Errorf("%s: hooks for %s are not a list; not changing them", path, event)
			}
		}
		if !hasTrimloopHook(list) {
			hooks[event], changed = append(list, entries.([]any)...), true
		}
	}
	cfg["hooks"] = hooks
	if !changed {
		return "", nil
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // keep & < > as written in other commands
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		return "", err
	}
	backup := ""
	if old != nil {
		if backup, err = writeBackup(path, old); err != nil {
			return "", err
		}
	}
	return backup, writeAtomic(path, buf.Bytes())
}

// writeBackup copies data to a new backup file next to path, never
// replacing an earlier backup.
func writeBackup(path string, data []byte) (string, error) {
	base := fmt.Sprintf("%s.trimloop-backup-%s", path, time.Now().Format("20060102-150405"))
	name := base
	for i := 2; ; i++ {
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			name = fmt.Sprintf("%s-%d", base, i)
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			return "", err
		}
		return name, f.Close()
	}
}

// writeAtomic replaces path with data, writing a temporary file in the same
// folder and renaming it, so a crash cannot leave half a settings file.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func sameCommand(a, b any) bool {
	am, _ := a.(map[string]any)
	bm, _ := b.(map[string]any)
	return am != nil && bm != nil && am["command"] == bm["command"]
}

// hasTrimloopHook reports whether a hook list already runs trimloop.
func hasTrimloopHook(list []any) bool {
	for _, e := range list {
		entry, _ := e.(map[string]any)
		inner, _ := entry["hooks"].([]any)
		for _, h := range inner {
			hook, _ := h.(map[string]any)
			if cmd, _ := hook["command"].(string); strings.Contains(cmd, "trimloop") && strings.Contains(cmd, " hook ") {
				return true
			}
		}
	}
	return false
}
