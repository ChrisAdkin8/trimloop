package live

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
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
// replace an existing status line unless replace is true, and does not add
// a hook that is already there. It returns the backup's path ("" if the file
// did not exist).
func WriteSettings(path, exe string, replace bool) (string, error) {
	cfg := map[string]any{}
	old, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		old = nil
	case err != nil:
		return "", err
	default:
		if err := json.Unmarshal(old, &cfg); err != nil {
			return "", fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
	}

	want := claudeSettings(exe)
	if sl, ok := cfg["statusLine"]; ok && !replace && !sameCommand(sl, want["statusLine"]) {
		return "", ErrStatusLineTaken
	}
	cfg["statusLine"] = want["statusLine"]

	hooks, _ := cfg["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for event, entries := range want["hooks"].(map[string]any) {
		list, _ := hooks[event].([]any)
		if !hasTrimloopHook(list) {
			list = append(list, entries.([]any)...)
		}
		hooks[event] = list
	}
	cfg["hooks"] = hooks

	backup := ""
	if old != nil {
		backup = fmt.Sprintf("%s.trimloop-backup-%s", path, time.Now().Format("20060102-150405"))
		if err := os.WriteFile(backup, old, 0o600); err != nil {
			return "", err
		}
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return backup, err
	}
	return backup, os.WriteFile(path, append(b, '\n'), 0o600)
}

func sameCommand(a, b any) bool {
	am, _ := a.(map[string]any)
	bm, _ := b.(map[string]any)
	return am != nil && bm != nil && am["command"] == bm["command"]
}

// hasTrimloopHook reports whether a hook list already runs trimloop.
func hasTrimloopHook(list []any) bool {
	for _, e := range list {
		inner, _ := e.(map[string]any)["hooks"].([]any)
		for _, h := range inner {
			if cmd, _ := h.(map[string]any)["command"].(string); strings.Contains(cmd, "trimloop") && strings.Contains(cmd, " hook ") {
				return true
			}
		}
	}
	return false
}
