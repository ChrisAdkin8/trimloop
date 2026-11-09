package live

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ChrisAdkin8/trimloop/internal/history"
	"github.com/ChrisAdkin8/trimloop/internal/safefile"
)

// The events trimloop hooks into, and the argument each hook command takes.
var hookEvents = map[string]string{"Stop": "stop", "UserPromptSubmit": "prompt"}

// hookTimeout is how long Claude Code waits for a trimloop hook, in seconds.
const hookTimeout = 5

// statusRefresh is how often, in seconds, Claude Code reruns the status line
// even when nothing happens, so the cache countdown moves while the user is
// away.
const statusRefresh = 30

// shellQuote quotes s for a POSIX shell, which runs hook and status-line
// commands: inside single quotes nothing is expanded.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func statusCommand(exe string) string { return shellQuote(exe) + " statusline" }

func hookCommand(exe, arg string) string { return shellQuote(exe) + " hook " + arg }

// Settings Claude Code needs to run the nudges.
func claudeSettings(exe string) map[string]any {
	hooks := map[string]any{}
	for event, arg := range hookEvents {
		hooks[event] = []any{hookEntry(exe, arg)}
	}
	return map[string]any{
		"statusLine": statusEntry(exe),
		"hooks":      hooks,
	}
}

// statusEntry is trimloop's status line. These are the keys trimloop owns;
// any other key in an existing status line (such as "padding") is the
// user's.
func statusEntry(exe string) map[string]any {
	return map[string]any{"type": "command", "command": statusCommand(exe), "refreshInterval": statusRefresh}
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

// HooksSnippet is the JSON for trimloop's hooks alone, for settings that
// keep another program's status line.
func HooksSnippet(exe string) string {
	b, _ := json.MarshalIndent(map[string]any{"hooks": claudeSettings(exe)["hooks"]}, "", "  ")
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
	Stale                     // runs trimloop from another path or with old settings: trimloop's keys updated
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
		if slMap["type"] == "command" && sameNumber(slMap["refreshInterval"], statusRefresh) {
			return Current
		}
		return Stale
	case isTrimloop(cmd, "statusline"):
		return Stale
	}
	return Other
}

// sameNumber reports whether a JSON value is the number want, however it is
// written (5, 5.0) and whether it was decoded as a float64 or a json.Number.
func sameNumber(v any, want int) bool {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return err == nil && f == float64(want)
	case float64:
		return n == float64(want)
	}
	return false
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
	return !runsExe(cmd, exe, "hook "+arg) || !sameNumber(hook["timeout"], hookTimeout)
}

// decodeSettings parses a settings file's contents, which must be a single
// JSON object and nothing after it: WriteSettings would otherwise drop what
// follows, and Claude Code may not read it. Numbers are kept as written so
// large ones stay exact.
func decodeSettings(path string, b []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s has more after its settings object (a comment or a second value)", path)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s does not hold a settings object", path)
	}
	return m, nil
}

// WriteSettings adds the nudges to the Claude Code settings file at path,
// keeping everything else in it, after copying it to a backup. Entries that
// already run trimloop are updated in place (for example after the binary
// moved); another status line is replaced only if replace is true; nothing
// is written if nothing changes. It refuses, rather than guesses, when the
// file or its "hooks" are not shaped as Claude Code expects, or when
// anything follows the settings object. A settings file that is a link is
// followed: the file it points to is updated, keeping its permissions. Other
// values keep their exact text; only key order may change. The backup goes
// in ~/.trimloop/backups (see writeBackup). It returns the backup's path
// ("" if none) and whether the file was written.
func WriteSettings(path, exe string, replace bool) (backup string, changed bool, err error) {
	path = LinkTarget(path)
	cfg := map[string]any{}
	mode := fs.FileMode(0o600)
	old, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		old = nil
		// Name the missing folder, not the temporary file safefile
		// would fail to make in it.
		if dir := filepath.Dir(path); !isDir(dir) {
			return "", false, fmt.Errorf("cannot write %s: the folder %s does not exist (Claude Code makes it the first time it runs); nothing was written", path, dir)
		}
	case err != nil:
		return "", false, err
	default:
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		}
		if cfg, err = decodeSettings(path, old); err != nil {
			return "", false, fmt.Errorf("%w; not changing it", err)
		}
	}

	switch statusLineState(cfg, exe) {
	case Current:
	case Stale:
		// trimloop's own status line: update only the keys trimloop
		// owns, keeping the user's others (such as "padding").
		sl := cfg["statusLine"].(map[string]any)
		for k, v := range statusEntry(exe) {
			sl[k] = v
		}
		changed = true
	case Other:
		if !replace {
			return "", false, ErrStatusLineTaken
		}
		// Someone else's status line is replaced whole: its other keys
		// were set for that program, and the backup keeps them.
		cfg["statusLine"], changed = statusEntry(exe), true
	case Missing:
		cfg["statusLine"], changed = statusEntry(exe), true
	}

	hooks := map[string]any{}
	if h, ok := cfg["hooks"]; ok {
		if hooks, ok = h.(map[string]any); !ok {
			return "", false, fmt.Errorf("%s: \"hooks\" is not an object; not changing it", path)
		}
	}
	for event, arg := range hookEvents {
		var list []any
		if l, ok := hooks[event]; ok {
			if list, ok = l.([]any); !ok {
				return "", false, fmt.Errorf("%s: hooks for %s are not a list; not changing them", path, event)
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
		return "", false, nil
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // keep & < > as written in other commands
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		return "", false, err
	}
	if old != nil {
		if backup, err = writeBackup(path, old); err != nil {
			return "", false, err
		}
	}
	if err := safefile.WriteBytes(path, mode, buf.Bytes()); err != nil {
		return backup, false, err
	}
	return backup, true, nil
}

// Check is where each of trimloop's settings entries stands in a settings
// file, for setup --check.
type Check struct {
	StatusLine EntryState
	Hooks      map[string]EntryState // by Claude Code event: "Stop", "UserPromptSubmit"
	// HooksOff is set when the file has "disableAllHooks": true, which
	// stops Claude Code running any hook or status line, trimloop's too.
	HooksOff bool
}

// OK reports whether every entry is up to date and Claude Code will run them.
func (c Check) OK() bool {
	if c.HooksOff || c.StatusLine != Current {
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
	cfg, err := decodeSettings(path, b)
	if err != nil {
		return c, err
	}
	c.HooksOff = cfg["disableAllHooks"] == true
	c.StatusLine = statusLineState(cfg, exe)
	hooks, _ := cfg["hooks"].(map[string]any)
	for event, arg := range hookEvents {
		list, _ := hooks[event].([]any)
		c.Hooks[event] = hookState(list, exe, arg)
	}
	return c, nil
}

// isDir reports whether path is a folder.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// writeBackup copies data, the settings file at path as it was, to a new
// file in ~/.trimloop/backups, never replacing an earlier backup. Settings
// can hold secrets (API keys in "env", an "apiKeyHelper"), so the backup is
// readable by the user only (0600, in a 0700 folder), and it is not put next
// to the settings file, which may be a link into a dotfiles repository. Its
// name says which file it copies and when: settings.json-<id>.trimloop-
// backup-<time>, where id is a hash of the file's path, so backups of the
// settings in different configuration folders stay apart.
func writeBackup(path string, data []byte) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot back up %s: %w; nothing was written", path, err)
	}
	dir := history.PathFor(home, "", "backups", "") // ~/.trimloop/backups
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("cannot back up %s: %w; nothing was written", path, err)
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	base := filepath.Join(dir, fmt.Sprintf("%s-%x.trimloop-backup-%s",
		filepath.Base(path), sum[:4], time.Now().Format("20060102-150405")))
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
			_ = os.Remove(name)
			return "", err
		}
		return name, f.Close()
	}
}

// LinkTarget is the file a settings path stands for: the file a link points
// to, followed through every link, even when that file does not exist yet
// (a dotfiles link made before the file), so writing it never replaces the
// link with a plain file. Any other path is returned as it is.
func LinkTarget(path string) string {
	for range 40 { // links to links, but not a loop
		if target, err := filepath.EvalSymlinks(path); err == nil {
			return target
		}
		dest, err := os.Readlink(path)
		if err != nil {
			return path // not a link: a plain file, or none yet
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(filepath.Dir(path), dest)
		}
		path = dest
	}
	return path
}
