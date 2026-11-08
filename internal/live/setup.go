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

// The events trimloop hooks into, and the argument each hook command takes.
var hookEvents = map[string]string{"Stop": "stop", "UserPromptSubmit": "prompt"}

// hookTimeout is how long Claude Code waits for a trimloop hook, in seconds.
const hookTimeout = 5

// shellQuote quotes s for a POSIX shell, which runs hook and status-line
// commands: inside single quotes nothing is expanded.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func statusCommand(exe string) string { return shellQuote(exe) + " statusline" }

func hookCommand(exe, arg string) string { return shellQuote(exe) + " hook " + arg }

// Settings Claude Code needs to run the nudges. The status line refreshes
// every 30 seconds so the cache countdown moves while the user is away.
func claudeSettings(exe string) map[string]any {
	hooks := map[string]any{}
	for event, arg := range hookEvents {
		hooks[event] = []any{hookEntry(exe, arg)}
	}
	return map[string]any{
		"statusLine": map[string]any{"type": "command", "command": statusCommand(exe), "refreshInterval": 30},
		"hooks":      hooks,
	}
}

func hookEntry(exe, arg string) map[string]any {
	return map[string]any{"hooks": []any{map[string]any{
		"type": "command", "command": hookCommand(exe, arg), "timeout": hookTimeout,
	}}}
}

// SetupSnippet is the JSON to add to Claude Code's settings for the nudges.
func SetupSnippet(exe string) string {
	b, _ := json.MarshalIndent(claudeSettings(exe), "", "  ")
	return string(b)
}

// ErrStatusLineTaken means the settings already have a different status line.
var ErrStatusLineTaken = errors.New("the settings already have a status line; use --replace-statusline to replace it")

// isTrimloop reports whether a command runs trimloop with the given
// arguments, wherever trimloop is installed: "statusline" or "hook stop".
// The program must be named trimloop (quoted or not); a wrapper such as
// trimloop-wrapper is someone else's.
func isTrimloop(cmd, args string) bool {
	prog, rest := splitCommand(strings.TrimSpace(cmd))
	return filepath.Base(prog) == "trimloop" && strings.TrimSpace(rest) == args
}

// splitCommand splits a shell command into its program, unquoted, and the
// rest. It understands a program in single or double quotes, which is how
// setup and earlier versions wrote it.
func splitCommand(cmd string) (prog, rest string) {
	if cmd == "" {
		return "", ""
	}
	if q := cmd[0]; q == '\'' || q == '"' {
		if end := strings.IndexByte(cmd[1:], q); end >= 0 {
			return cmd[1 : end+1], cmd[end+2:]
		}
		return "", cmd
	}
	if sp := strings.IndexByte(cmd, ' '); sp >= 0 {
		return cmd[:sp], cmd[sp:]
	}
	return cmd, ""
}

// EntryState is where one of trimloop's settings entries stands: the
// status line, or one hook. WriteSettings and CheckSettings both decide
// from it.
type EntryState int

// The states of a settings entry, and what WriteSettings does in each.
const (
	Missing EntryState = iota // not there: added
	Current                   // runs this trimloop with today's settings: left alone
	Stale                     // runs trimloop from another path or with old settings: updated
	Other                     // a status line that is not trimloop's: kept unless replace is set
)

func (s EntryState) String() string {
	return [...]string{"missing", "up to date", "out of date", "another program's"}[s]
}

// statusLineState classifies the settings' status line.
func statusLineState(cfg map[string]any, exe string) EntryState {
	sl, has := cfg["statusLine"]
	if !has {
		return Missing
	}
	slMap, _ := sl.(map[string]any)
	cmd, _ := slMap["command"].(string)
	switch {
	case runsExe(cmd, exe, "statusline"):
		return Current
	case isTrimloop(cmd, "statusline"):
		return Stale
	}
	return Other
}

// runsExe reports whether cmd runs exe with args, however exe is quoted.
func runsExe(cmd, exe, args string) bool {
	prog, rest := splitCommand(strings.TrimSpace(cmd))
	return prog == exe && strings.TrimSpace(rest) == args
}

// hookState classifies trimloop's hooks for one event in list.
func hookState(list []any, exe, arg string) EntryState {
	hooks := trimloopHooks(list, arg)
	if len(hooks) == 0 {
		return Missing
	}
	for _, h := range hooks {
		if staleHook(h, exe, arg) {
			return Stale
		}
	}
	return Current
}

// trimloopHooks returns the hooks in list that run trimloop's hook arg.
func trimloopHooks(list []any, arg string) []map[string]any {
	var out []map[string]any
	for _, e := range list {
		entry, _ := e.(map[string]any)
		inner, _ := entry["hooks"].([]any)
		for _, h := range inner {
			hook, _ := h.(map[string]any)
			cmd, _ := hook["command"].(string)
			if hook != nil && isTrimloop(cmd, "hook "+arg) {
				out = append(out, hook)
			}
		}
	}
	return out
}

func staleHook(hook map[string]any, exe, arg string) bool {
	cmd, _ := hook["command"].(string)
	return !runsExe(cmd, exe, "hook "+arg) || fmt.Sprint(hook["timeout"]) != fmt.Sprint(hookTimeout)
}

// WriteSettings adds the nudges to the Claude Code settings file at path,
// keeping everything else in it, after copying it to a backup. Entries that
// already run trimloop are updated in place (for example after the binary
// moved); another status line is replaced only if replace is true; nothing
// is written if nothing changes. It refuses, rather than guesses, when the
// file or its "hooks" are not shaped as Claude Code expects. A settings file
// that is a link is followed: the file it points to is updated, keeping its
// permissions. Other values keep their exact text; only key order may
// change. It returns the backup's path ("" if none).
func WriteSettings(path, exe string, replace bool) (string, error) {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	cfg := map[string]any{}
	mode := fs.FileMode(0o600)
	old, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		old = nil
	case err != nil:
		return "", err
	default:
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		}
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
	switch statusLineState(cfg, exe) {
	case Current:
	case Other:
		if !replace {
			return "", ErrStatusLineTaken
		}
		cfg["statusLine"], changed = want["statusLine"], true
	default: // Missing, Stale
		cfg["statusLine"], changed = want["statusLine"], true
	}

	hooks := map[string]any{}
	if h, ok := cfg["hooks"]; ok {
		if hooks, ok = h.(map[string]any); !ok {
			return "", fmt.Errorf("%s: \"hooks\" is not an object; not changing it", path)
		}
	}
	for event, arg := range hookEvents {
		var list []any
		if l, ok := hooks[event]; ok {
			if list, ok = l.([]any); !ok {
				return "", fmt.Errorf("%s: hooks for %s are not a list; not changing them", path, event)
			}
		}
		switch hookState(list, exe, arg) {
		case Missing:
			hooks[event], changed = append(list, hookEntry(exe, arg)), true
		case Stale:
			for _, h := range trimloopHooks(list, arg) {
				h["command"], h["timeout"] = hookCommand(exe, arg), hookTimeout
			}
			hooks[event], changed = list, true
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
		if backup, err = writeBackup(path, old, mode); err != nil {
			return "", err
		}
	}
	return backup, writeAtomic(path, buf.Bytes(), mode)
}

// Check is where each of trimloop's settings entries stands in a settings
// file, for setup --check.
type Check struct {
	StatusLine EntryState
	Hooks      map[string]EntryState // by Claude Code event: "Stop", "UserPromptSubmit"
}

// OK reports whether every entry is up to date.
func (c Check) OK() bool {
	if c.StatusLine != Current {
		return false
	}
	for _, s := range c.Hooks {
		if s != Current {
			return false
		}
	}
	return true
}

// CheckSettings reads the Claude Code settings file at path and says where
// each of trimloop's entries stands, without changing anything. A missing
// file has every entry missing.
func CheckSettings(path, exe string) (Check, error) {
	c := Check{StatusLine: Missing, Hooks: map[string]EntryState{}}
	for event := range hookEvents {
		c.Hooks[event] = Missing
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		return c, fmt.Errorf("%s is not a settings object: %w", path, err)
	}
	c.StatusLine = statusLineState(cfg, exe)
	hooks, _ := cfg["hooks"].(map[string]any)
	for event, arg := range hookEvents {
		list, _ := hooks[event].([]any)
		c.Hooks[event] = hookState(list, exe, arg)
	}
	return c, nil
}

// writeBackup copies data to a new backup file next to path, never
// replacing an earlier backup.
func writeBackup(path string, data []byte, mode fs.FileMode) (string, error) {
	base := fmt.Sprintf("%s.trimloop-backup-%s", path, time.Now().Format("20060102-150405"))
	name := base
	for i := 2; ; i++ {
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
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
func writeAtomic(path string, data []byte, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.json")
	if err != nil {
		return err
	}
	cleanup := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return cleanup(err)
	}
	if err := tmp.Chmod(mode); err != nil {
		return cleanup(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
