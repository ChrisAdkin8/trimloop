package live

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

var t0 = time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)

func reply(context int, at time.Time) Reply {
	return Reply{Time: at, Model: "claude-opus-5-5", Context: context,
		Usage: event.Usage{CacheRead: context - 1000, CacheWrite1h: 1000, Output: 500}}
}

func TestStatus(t *testing.T) {
	s := DefaultSettings()
	tests := []struct {
		name    string
		r       Reply
		now     time.Time
		effort  string
		context string
		level   Level
		cache   string
		eff     string
	}{
		{"small, fresh cache", reply(50_000, t0), t0.Add(time.Minute), "medium", "50k tokens", Small, "", ""},
		{"large, cache ending", reply(312_000, t0), t0.Add(52 * time.Minute), "", "312k tokens", Large, "cache ends in 9 min", ""},
		{"very large, expired", reply(610_000, t0), t0.Add(2 * time.Hour), "max", "610k tokens", VeryLarge, "cache expired · next message ≈ $4.88", "max effort"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := s.Status(tt.r, tt.now, 4.1, tt.effort)
			if p.Context != tt.context || p.Level != tt.level || p.Cache != tt.cache || p.Effort != tt.eff || p.Cost != "$4.10" {
				t.Errorf("got %+v", p)
			}
		})
	}
	if p := s.Status(Reply{Context: 10}, t0, -1, ""); p.Cost != "" || p.Cache != "" {
		t.Errorf("no cost or time known: got %+v", p)
	}
}

func TestCacheLife(t *testing.T) {
	if (Reply{Usage: event.Usage{CacheWrite5m: 5}}).CacheLife() != 5*time.Minute {
		t.Error("a 5-minute write should give a 5-minute cache")
	}
	if (Reply{Usage: event.Usage{CacheRead: 5}, Writer: event.Usage{CacheWrite5m: 9}}).CacheLife() != 5*time.Minute {
		t.Error("a reply that wrote nothing should keep the lifetime of the last reply that wrote")
	}
	if (Reply{Usage: event.Usage{CacheRead: 5}}).CacheLife() != time.Hour {
		t.Error("no write should assume the hour")
	}
}

func TestAfterReply(t *testing.T) {
	s := DefaultSettings()
	crossed := map[int]bool{}
	if msg, _ := s.AfterReply(reply(150_000, t0), crossed); msg != "" {
		t.Errorf("under 200k: got %q", msg)
	}
	msg, th := s.AfterReply(reply(210_000, t0), crossed)
	if th != 200_000 || !strings.Contains(msg, "/compact") {
		t.Fatalf("at 210k: got %q, %d", msg, th)
	}
	crossed[th] = true
	if msg, _ := s.AfterReply(reply(300_000, t0), crossed); msg != "" {
		t.Errorf("shown once: got %q", msg)
	}
	msg, th = s.AfterReply(reply(610_000, t0), crossed)
	if th != 500_000 || !strings.Contains(msg, "20× what a fresh session sends") {
		t.Fatalf("at 610k: got %q, %d", msg, th)
	}
	// A session first seen past 500k gets only the stronger message.
	if _, th := s.AfterReply(reply(700_000, t0), map[int]bool{}); th != 500_000 {
		t.Errorf("first seen at 700k: got threshold %d", th)
	}
	if msg, _ := s.AfterReply(reply(250_000, t0), map[int]bool{500_000: true}); msg != "" {
		t.Errorf("after the stronger message, the weaker should not follow: got %q", msg)
	}
}

func TestBeforeSend(t *testing.T) {
	s := DefaultSettings()
	big := reply(610_000, t0)
	if r := s.BeforeSend(big, t0.Add(30*time.Minute), time.Time{}); r != "" {
		t.Errorf("cache still warm: got %q", r)
	}
	if r := s.BeforeSend(reply(300_000, t0), t0.Add(3*time.Hour), time.Time{}); r != "" {
		t.Errorf("not very large: got %q", r)
	}
	r := s.BeforeSend(big, t0.Add(3*time.Hour), time.Time{})
	if !strings.Contains(r, "away 3 hours") || !strings.Contains(r, "610k tokens") || !strings.Contains(r, "Send your message again") {
		t.Fatalf("got %q", r)
	}
	if r := s.BeforeSend(big, t0.Add(3*time.Hour), t0); r != "" {
		t.Errorf("second send after the same break: got %q", r)
	}
}

func TestStore(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "live")}
	st := s.Load("abc-123")
	st.Crossed[200_000] = true
	st.PausedFor = t0
	if err := s.Save("abc-123", st); err != nil {
		t.Fatal(err)
	}
	got := s.Load("abc-123")
	if !got.Crossed[200_000] || !got.PausedFor.Equal(t0) {
		t.Errorf("got %+v", got)
	}
	if err := s.Save("../escape", st); err == nil || fileExists(filepath.Join(s.Dir, "..", "escape.json")) {
		t.Error("an unsafe session id must not write outside the folder")
	}
	old := filepath.Join(s.Dir, "old.json")
	if err := os.WriteFile(old, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(old, t0.AddDate(0, -2, 0), t0.AddDate(0, -2, 0)); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("abc-123", st); err != nil || fileExists(old) {
		t.Error("states untouched for 30 days should be removed")
	}
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// session writes a transcript whose last reply sent context tokens at the
// given time, and returns the hook input for it.
func session(t *testing.T, context int, at time.Time) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	line := fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"id":"m1","model":"claude-opus-5-5",`+
		`"usage":{"cache_read_input_tokens":%d,"cache_creation":{"ephemeral_1h_input_tokens":1000},"output_tokens":10}}}`+"\n",
		at.Format(time.RFC3339), context-1000)
	if err := os.WriteFile(p, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"session_id": "s1", "transcript_path": p, "cost": map[string]any{"total_cost_usd": 4.1}})
	return string(b)
}

func env(t *testing.T, now time.Time) Env {
	dir := t.TempDir()
	return Env{Settings: DefaultSettings(), Store: Store{Dir: filepath.Join(dir, "live")}, Log: filepath.Join(dir, "nudges.jsonl"), Now: now}
}

func TestHookStopShowsOnce(t *testing.T) {
	in := session(t, 250_000, t0)
	e := env(t, t0.Add(time.Minute))
	var out bytes.Buffer
	Hook("stop", strings.NewReader(in), &out, e)
	var got map[string]string
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || !strings.Contains(got["systemMessage"], "200k") {
		t.Fatalf("first: got %q (%v)", out.String(), err)
	}
	if _, ok := got["decision"]; ok {
		t.Error("a Stop hook must not set decision: it would keep Claude working")
	}
	out.Reset()
	Hook("stop", strings.NewReader(in), &out, e)
	if out.Len() != 0 {
		t.Errorf("second: got %q, want nothing", out.String())
	}
	if b, err := os.ReadFile(e.Log); err != nil || !strings.Contains(string(b), `"after-reply-200000"`) {
		t.Errorf("nudge log: got %q, %v", b, err)
	}
}

func TestHookPromptPausesOnce(t *testing.T) {
	in := session(t, 650_000, t0)
	e := env(t, t0.Add(2*time.Hour))
	var out bytes.Buffer
	Hook("prompt", strings.NewReader(in), &out, e)
	var got map[string]string
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got["decision"] != "block" || !strings.Contains(got["reason"], "650k") {
		t.Fatalf("first send: got %q (%v)", out.String(), err)
	}
	out.Reset()
	Hook("prompt", strings.NewReader(in), &out, e)
	if out.Len() != 0 {
		t.Errorf("second send: got %q, want it let through", out.String())
	}
}

func TestHookFailsQuietly(t *testing.T) {
	var out bytes.Buffer
	for _, in := range []string{"not json", `{"session_id":"s1","transcript_path":"/missing"}`, `{}`} {
		Hook("stop", strings.NewReader(in), &out, env(t, t0))
		Hook("prompt", strings.NewReader(in), &out, env(t, t0))
		Hook("other", strings.NewReader(in), &out, env(t, t0))
	}
	if out.Len() != 0 {
		t.Errorf("got %q, want nothing", out.String())
	}
}

func TestStatusLine(t *testing.T) {
	var out bytes.Buffer
	StatusLine(strings.NewReader(session(t, 610_000, t0)), &out, env(t, t0.Add(2*time.Hour)))
	line := out.String()
	for _, want := range []string{"610k tokens", "$4.10", "cache expired", red} {
		if !strings.Contains(line, want) {
			t.Errorf("status line %q has no %q", line, want)
		}
	}
	out.Reset()
	StatusLine(strings.NewReader(`{"transcript_path":"/missing","cost":{"total_cost_usd":1.5}}`), &out, env(t, t0))
	if strings.TrimSpace(out.String()) != "$1.50" {
		t.Errorf("without a log: got %q, want the cost alone", out.String())
	}
}

func TestWriteSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	existing := `{"model":"opus","hooks":{"Stop":[{"hooks":[{"type":"command","command":"say done"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	backup, changed, err := WriteSettings(path, "/bin/trimloop", false)
	if err != nil || backup == "" || !changed {
		t.Fatalf("got %q, %v, %v", backup, changed, err)
	}
	if b, _ := os.ReadFile(backup); string(b) != existing {
		t.Error("the backup should hold the old file")
	}
	var cfg map[string]any
	b, _ := os.ReadFile(path)
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["model"] != "opus" || !strings.Contains(string(b), "say done") || !strings.Contains(string(b), `'/bin/trimloop' hook stop`) {
		t.Errorf("settings lost or missing: %s", b)
	}
	// Running it again adds nothing twice, and says nothing changed.
	if backup, changed, err := WriteSettings(path, "/bin/trimloop", false); err != nil || changed || backup != "" {
		t.Fatalf("second run: got %q, %v, %v", backup, changed, err)
	}
	b2, _ := os.ReadFile(path)
	if strings.Count(string(b2), "hook stop") != 1 {
		t.Errorf("hook added twice: %s", b2)
	}
	// Someone else's status line is kept unless replacing is asked for.
	other := filepath.Join(dir, "other.json")
	if err := os.WriteFile(other, []byte(`{"statusLine":{"type":"command","command":"mine.sh"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteSettings(other, "/bin/trimloop", false); !errors.Is(err, ErrStatusLineTaken) {
		t.Errorf("got %v, want ErrStatusLineTaken", err)
	}
	if _, _, err := WriteSettings(other, "/bin/trimloop", true); err != nil {
		t.Error(err)
	}
	// A new file is created, with no backup.
	if backup, changed, err := WriteSettings(filepath.Join(dir, "new.json"), "/bin/trimloop", false); err != nil || backup != "" || !changed {
		t.Errorf("new file: got %q, %v, %v", backup, changed, err)
	}
	if _, _, err := WriteSettings(filepath.Join(dir, "missing", "x.json"), "/bin/trimloop", false); err == nil {
		t.Error("got no error writing into a missing folder")
	}
}

// Settings files in unexpected shapes are refused, never crashed on or
// silently changed; other values keep their exact text; backups never
// overwrite each other; nothing is written when nothing is missing.
func TestWriteSettingsSafety(t *testing.T) {
	dir := t.TempDir()
	for _, body := range []string{"null", "[1]", `{"hooks":[1]}`, `{"hooks":{"Stop":"x"}}`} {
		p := filepath.Join(dir, "bad.json")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := WriteSettings(p, "/bin/trimloop", false); err == nil {
			t.Errorf("%s: got no error", body)
		}
		if b, _ := os.ReadFile(p); string(b) != body {
			t.Errorf("%s: file changed to %s", body, b)
		}
	}

	// A hook list holding something unexpected is kept as it is, with
	// trimloop's hook added after it.
	odd := filepath.Join(dir, "odd.json")
	if err := os.WriteFile(odd, []byte(`{"hooks":{"Stop":["x"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteSettings(odd, "/bin/trimloop", false); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(odd); !strings.Contains(string(b), `"x"`) || !strings.Contains(string(b), "hook stop") {
		t.Errorf("odd hook list: got %s", b)
	}

	p := filepath.Join(dir, "settings.json")
	orig := `{"big":12345678901234567890,"hooks":{"PostToolUse":[{"hooks":[{"type":"command","command":"a && b < c > d"}]}]}}`
	if err := os.WriteFile(p, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	b1, _, err := WriteSettings(p, "/bin/trimloop", false)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "12345678901234567890") || !strings.Contains(string(got), `a && b < c > d`) {
		t.Errorf("values changed: %s", got)
	}
	if b2, changed, err := WriteSettings(p, "/bin/trimloop", false); err != nil || b2 != "" || changed {
		t.Errorf("second run should change nothing: backup %q, %v", b2, err)
	}
	// Force a second backup in the same second: it must not replace the first.
	if err := os.WriteFile(p, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	b3, _, err := WriteSettings(p, "/bin/trimloop", false)
	if err != nil || b3 == b1 {
		t.Errorf("backups: first %q, second %q (%v)", b1, b3, err)
	}
	if first, _ := os.ReadFile(b1); string(first) != orig {
		t.Error("the first backup was overwritten")
	}
}

func TestMoneyNegative(t *testing.T) {
	if money(-4.1) != "-$4.10" || money(-12) != "-$12" {
		t.Errorf("got %q and %q", money(-4.1), money(-12))
	}
}

// A linked settings file is updated where it really lives, keeping its
// permissions; a moved binary's entries are updated, not duplicated; paths
// are quoted for the shell.
func TestWriteSettingsLinksMovesAndQuoting(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"model":"opus"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteSettings(link, "/old/trimloop", false); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced by a file")
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode: got %v, want 0644 kept", fi.Mode().Perm())
	}

	// The binary moved: the status line and hooks follow, with no duplicates.
	if _, _, err := WriteSettings(link, "/new/trimloop", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(target)
	if strings.Contains(string(b), "/old/") || strings.Count(string(b), "hook stop") != 1 || strings.Count(string(b), "statusline") != 1 {
		t.Errorf("after the move: %s", b)
	}

	if got := hookCommand("/p$x/tri'loop", "stop"); got != `'/p$x/tri'\''loop' hook stop` {
		t.Errorf("quoting: got %s", got)
	}
}

// After a session shrinks (compacted or cleared), the nudges re-arm.
func TestHookStopRearmsAfterShrinking(t *testing.T) {
	e := env(t, t0.Add(time.Minute))
	var out bytes.Buffer
	Hook("stop", strings.NewReader(session(t, 250_000, t0)), &out, e)
	out.Reset()
	Hook("stop", strings.NewReader(session(t, 40_000, t0)), &out, e)  // compacted
	Hook("stop", strings.NewReader(session(t, 260_000, t0)), &out, e) // large again
	if !strings.Contains(out.String(), "200k") {
		t.Errorf("got %q, want the 200k nudge again", out.String())
	}

	// Compacted to a size still over 200k: only the 500k nudge comes back.
	e = env(t, t0.Add(time.Minute))
	out.Reset()
	Hook("stop", strings.NewReader(session(t, 600_000, t0)), &out, e)
	out.Reset()
	Hook("stop", strings.NewReader(session(t, 300_000, t0)), &out, e)
	if out.Len() != 0 {
		t.Errorf("at 300k after compacting: got %q, want nothing", out.String())
	}
	Hook("stop", strings.NewReader(session(t, 550_000, t0)), &out, e)
	if !strings.Contains(out.String(), "550k") {
		t.Errorf("got %q, want the 500k nudge again", out.String())
	}
}

// After a /compact the nudges use the compacted size: no hold, no countdown.
func TestNudgesAfterCompaction(t *testing.T) {
	in := session(t, 552_000, t0)
	var hook struct {
		Path string `json:"transcript_path"`
	}
	if err := json.Unmarshal([]byte(in), &hook); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(hook.Path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintf(f, `{"type":"system","subtype":"compact_boundary","timestamp":%q,"compactMetadata":{"preTokens":552000,"postTokens":30000,"trigger":"manual"}}`+"\n",
		t0.Add(time.Minute).Format(time.RFC3339))
	_ = f.Close()

	e := env(t, t0.Add(2*time.Hour))
	var out bytes.Buffer
	Hook("prompt", strings.NewReader(in), &out, e)
	if out.Len() != 0 {
		t.Errorf("prompt after compaction: got %q, want it let through", out.String())
	}
	StatusLine(strings.NewReader(in), &out, e)
	if line := out.String(); !strings.Contains(line, "compacted") || strings.Contains(line, "30k") || strings.Contains(line, "cache") {
		t.Errorf("status line after compaction: got %q", line)
	}
}

func TestStatusLineKeepsLimitReadings(t *testing.T) {
	e := env(t, t0)
	e.Limits = filepath.Join(t.TempDir(), "limits.jsonl")
	in := func(used float64) string {
		var m map[string]any
		_ = json.Unmarshal([]byte(session(t, 100_000, t0)), &m)
		m["rate_limits"] = map[string]any{
			"five_hour": map[string]any{"used_percentage": used, "resets_at": t0.Add(3 * time.Hour).Unix()},
			"seven_day": map[string]any{"used_percentage": 40, "resets_at": t0.Add(5 * 24 * time.Hour).Unix()},
		}
		b, _ := json.Marshal(m)
		return string(b)
	}
	for _, used := range []float64{12, 12, 15} {
		StatusLine(strings.NewReader(in(used)), &bytes.Buffer{}, e)
	}
	b, err := os.ReadFile(e.Limits)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "\n"); n != 2 || !strings.Contains(string(b), `"used":15`) || strings.Contains(string(b), "s1") {
		t.Errorf("limit readings: %d lines, %s; want 2, numbers only", n, b)
	}
	// No rate limits (an API key, or before the first reply): nothing kept.
	e.Limits = filepath.Join(t.TempDir(), "none.jsonl")
	StatusLine(strings.NewReader(session(t, 100_000, t0)), &bytes.Buffer{}, e)
	if fileExists(e.Limits) {
		t.Error("readings kept without rate limits")
	}
}

func TestIsTrimloop(t *testing.T) {
	for _, c := range []struct {
		cmd, args string
		want      bool
	}{
		{`'/usr/local/bin/trimloop' statusline`, "statusline", true},
		{`"/Users/a b/bin/trimloop" hook stop`, "hook stop", true},
		{`trimloop statusline`, "statusline", true},
		{`/opt/bin/trimloop-wrapper statusline`, "statusline", false}, // someone else's program
		{`trimloop hook stop`, "hook prompt", false},
		{`'/bin/trimloop`, "statusline", false}, // unclosed quote
		{``, "statusline", false},
	} {
		if got := isTrimloop(c.cmd, c.args); got != c.want {
			t.Errorf("isTrimloop(%q, %q) = %v, want %v", c.cmd, c.args, got, c.want)
		}
	}
}

func TestCheckSettingsStates(t *testing.T) {
	dir := t.TempDir()
	exe := "/home/u/bin/trimloop"
	write := func(s string) string {
		p := filepath.Join(dir, "settings.json")
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	hook := func(cmd string) string {
		return `[{"hooks":[{"type":"command","command":` + strconv.Quote(cmd) + `,"timeout":5}]}]`
	}
	c, err := CheckSettings(write(`{"statusLine":{"type":"command","command":"\"/home/u/bin/trimloop\" statusline","refreshInterval":30},"hooks":{`+
		`"Stop":`+hook(`"/home/u/bin/trimloop" hook stop`)+`,`+
		`"UserPromptSubmit":`+hook(`/old/trimloop hook prompt`)+`}}`), exe)
	if err != nil {
		t.Fatal(err)
	}
	if c.StatusLine != Current || c.Hooks["Stop"] != Current || c.Hooks["UserPromptSubmit"] != Stale || c.OK() {
		t.Errorf("double-quoted current, old path stale: got %+v", c)
	}
	c, _ = CheckSettings(write(`{"statusLine":{"command":"ccstatus"}}`), exe)
	if c.StatusLine != Other || c.Hooks["Stop"] != Missing || c.StatusLine.String() != "another program's" {
		t.Errorf("someone else's status line, no hooks: got %+v", c)
	}
	if c, err := CheckSettings(filepath.Join(dir, "missing.json"), exe); err != nil || c.StatusLine != Missing {
		t.Errorf("no settings file: got %+v, %v", c, err)
	}
	if _, err := CheckSettings(write(`[1]`), exe); err == nil {
		t.Error("a settings file that is not an object: want an error")
	}
}

// The status line is up to date only with today's type and refresh
// interval, like hooks with their timeout; numbers count however written.
func TestCheckSettingsStatusLineFields(t *testing.T) {
	exe := "/bin/trimloop"
	p := filepath.Join(t.TempDir(), "settings.json")
	for _, c := range []struct {
		name, statusLine string
		want             EntryState
	}{
		{"today's", `{"type":"command","command":"'/bin/trimloop' statusline","refreshInterval":30}`, Current},
		{"written as 30.0", `{"type":"command","command":"'/bin/trimloop' statusline","refreshInterval":30.0}`, Current},
		{"no refresh", `{"type":"command","command":"'/bin/trimloop' statusline"}`, Stale},
		{"other refresh", `{"type":"command","command":"'/bin/trimloop' statusline","refreshInterval":5}`, Stale},
		{"refresh as text", `{"type":"command","command":"'/bin/trimloop' statusline","refreshInterval":"30"}`, Stale},
		{"no type", `{"command":"'/bin/trimloop' statusline","refreshInterval":30}`, Stale},
		{"old path", `{"type":"command","command":"/old/trimloop statusline","refreshInterval":30}`, Stale},
		{"someone else's", `{"type":"command","command":"ccstatus","refreshInterval":30}`, Other},
	} {
		if err := os.WriteFile(p, []byte(`{"statusLine":`+c.statusLine+`}`), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := CheckSettings(p, exe)
		if err != nil || got.StatusLine != c.want {
			t.Errorf("%s: got %v (%v), want %v", c.name, got.StatusLine, err, c.want)
		}
	}
}

// "disableAllHooks": true stops Claude Code running any hook or status
// line, so the entries are not working even when they are up to date.
func TestCheckSettingsHooksOff(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	for _, c := range []struct {
		value  string
		off    bool
		wantOK bool
	}{
		{`true`, true, false},
		{`false`, false, true},
		{`"true"`, false, true}, // not a JSON true: Claude Code ignores it too
	} {
		if _, _, err := WriteSettings(p, "/bin/trimloop", false); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(p)
		var cfg map[string]any
		if err := json.Unmarshal(b, &cfg); err != nil {
			t.Fatal(err)
		}
		cfg["disableAllHooks"] = json.RawMessage(c.value)
		b, _ = json.Marshal(cfg)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := CheckSettings(p, "/bin/trimloop")
		if err != nil || got.HooksOff != c.off || got.OK() != c.wantOK || got.StatusLine != Current {
			t.Errorf("disableAllHooks %s: got %+v, OK %v, %v", c.value, got, got.OK(), err)
		}
	}
}

// Anything after the settings object is refused by --write, leaving the
// file as it was, and by --check, so the two agree.
func TestSettingsTrailingData(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	for _, c := range []struct {
		name, body string
		wantErr    bool
	}{
		{"comment and second object", "{\"model\":\"opus\"}\n// comment\n{\"second\":true}", true},
		{"second object", `{"model":"opus"} {"second":true}`, true},
		{"stray value", `{"model":"opus"} 1`, true},
		{"stray brace", `{"model":"opus"}}`, true},
		{"trailing blank lines", "{\"model\":\"opus\"}\n\n  \t\n", false},
	} {
		if err := os.WriteFile(p, []byte(c.body), 0o600); err != nil {
			t.Fatal(err)
		}
		_, checkErr := CheckSettings(p, "/bin/trimloop")
		_, _, writeErr := WriteSettings(p, "/bin/trimloop", false)
		if (checkErr != nil) != c.wantErr || (writeErr != nil) != c.wantErr {
			t.Errorf("%s: check %v, write %v; want an error: %v", c.name, checkErr, writeErr, c.wantErr)
		}
		if b, _ := os.ReadFile(p); c.wantErr && string(b) != c.body {
			t.Errorf("%s: file changed to %s", c.name, b)
		}
		if c.wantErr && !strings.Contains(fmt.Sprint(writeErr), "has more after its settings object") {
			t.Errorf("%s: unclear error %v", c.name, writeErr)
		}
	}
}

// Updating trimloop's own status line changes only the keys trimloop owns,
// keeping the user's others; someone else's is replaced whole when asked.
func TestWriteSettingsStatusLineKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	for _, c := range []struct {
		name, statusLine string
		replace          bool
		wantPadding      bool
	}{
		{"old path", `{"type":"command","command":"/old/trimloop statusline","refreshInterval":30,"padding":2}`, false, true},
		{"no refresh", `{"type":"command","command":"'/bin/trimloop' statusline","padding":2}`, false, true},
		{"other refresh", `{"type":"command","command":"'/bin/trimloop' statusline","refreshInterval":5,"padding":2}`, false, true},
		{"someone else's", `{"type":"command","command":"ccstatus","padding":2}`, true, false},
	} {
		if err := os.WriteFile(p, []byte(`{"statusLine":`+c.statusLine+`}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, changed, err := WriteSettings(p, "/bin/trimloop", c.replace); err != nil || !changed {
			t.Fatalf("%s: changed %v, %v", c.name, changed, err)
		}
		b, _ := os.ReadFile(p)
		var cfg struct {
			StatusLine map[string]any `json:"statusLine"`
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			t.Fatal(err)
		}
		sl := cfg.StatusLine
		if sl["command"] != "'/bin/trimloop' statusline" || sl["type"] != "command" || sl["refreshInterval"] != float64(30) {
			t.Errorf("%s: trimloop's keys not set: %v", c.name, sl)
		}
		if _, has := sl["padding"]; has != c.wantPadding {
			t.Errorf("%s: padding kept %v, want %v", c.name, has, c.wantPadding)
		}
		if got, _ := CheckSettings(p, "/bin/trimloop"); !got.OK() {
			t.Errorf("%s: not up to date after --write: %+v", c.name, got)
		}
	}
}

func TestStateAfterReply(t *testing.T) {
	s := DefaultSettings()
	st := State{Crossed: map[int]bool{}}
	steps := []struct {
		context    int
		wantThresh int
		rearmed    bool
	}{
		{150_000, 0, false},       // small: nothing
		{250_000, s.Large, false}, // passed large: announced once
		{260_000, 0, false},       // still large: not news
		{600_000, s.VeryLarge, false},
		{650_000, 0, false},
		{120_000, 0, true},            // compacted: both re-armed, nothing shown
		{550_000, s.VeryLarge, false}, // straight past very large: only the highest
		{300_000, 0, true},            // under very large again: re-armed, large stays announced
		{520_000, s.VeryLarge, false},
	}
	for i, c := range steps {
		msg, th, re := st.afterReply(s, Reply{Context: c.context})
		if th != c.wantThresh || (msg != "") != (c.wantThresh != 0) || re != c.rearmed {
			t.Errorf("step %d (%d tokens): got threshold %d, rearmed %v, msg %q; want %d, %v",
				i, c.context, th, re, msg, c.wantThresh, c.rearmed)
		}
	}
}

func TestStateBeforeSendPausesOncePerBreak(t *testing.T) {
	s := DefaultSettings()
	st := State{Crossed: map[int]bool{}}
	r := Reply{Time: t0, Context: 600_000, Model: "claude-opus-5-5", Usage: event.Usage{CacheWrite1h: 1000}}
	if st.beforeSend(s, r, t0.Add(30*time.Minute)) != "" {
		t.Error("cache still warm: no pause")
	}
	if st.beforeSend(s, r, t0.Add(2*time.Hour)) == "" || !st.PausedFor.Equal(t0) {
		t.Error("cache expired, very large: pause, and remember it")
	}
	if st.beforeSend(s, r, t0.Add(3*time.Hour)) != "" {
		t.Error("the same break: no second pause")
	}
	r.Time = t0.Add(4 * time.Hour) // a new reply, then another long break
	if st.beforeSend(s, r, r.Time.Add(2*time.Hour)) == "" {
		t.Error("a new break: pause again")
	}
}

// TestInputIsLenient checks that a field sent with an unexpected shape loses
// only that field: the hooks still see the session, and no zero or
// half-read usage-limit reading is kept.
func TestInputIsLenient(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	line := fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"id":"m1","model":"claude-opus-5-5",`+
		`"usage":{"cache_read_input_tokens":249000,"cache_creation":{"ephemeral_1h_input_tokens":1000},"output_tokens":10}}}`+"\n",
		t0.Format(time.RFC3339))
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf(`"session_id":"s1","transcript_path":%q,"cost":{"total_cost_usd":4.1}`, path)
	five := func(v string) string { return `{` + base + `,"rate_limits":{"five_hour":` + v + `}}` }
	for _, c := range []struct {
		name, in  string
		hooks     bool  // the hooks still see the session
		size      bool  // the status line still shows the context size
		cost      bool  // the status line still shows the cost
		resetsAt  int64 // the five-hour reading kept; 0 for none
		effortHas string
	}{
		{name: "effort as a string", in: `{` + base + `,"effort":"max"}`, hooks: true, size: true, cost: true},
		{name: "effort as a struct", in: `{` + base + `,"effort":{"level":"max"}}`, hooks: true, size: true, cost: true, effortHas: "max effort"},
		{name: "unknown fields", in: `{` + base + `,"new_thing":{"a":[1,2]},"workspace":7}`, hooks: true, size: true, cost: true},
		{name: "cost as a string", in: fmt.Sprintf(`{"session_id":"s1","transcript_path":%q,"cost":"4.10"}`, path), hooks: true, size: true},
		{name: "resets_at as RFC 3339", in: five(`{"used_percentage":12,"resets_at":"2026-11-09T18:00:00Z"}`),
			hooks: true, size: true, cost: true, resetsAt: time.Date(2026, 11, 9, 18, 0, 0, 0, time.UTC).Unix()},
		{name: "resets_at as a number", in: five(`{"used_percentage":12,"resets_at":1794150000}`), hooks: true, size: true, cost: true, resetsAt: 1_794_150_000},
		{name: "resets_at in milliseconds", in: five(`{"used_percentage":12,"resets_at":1794150000000}`), hooks: true, size: true, cost: true, resetsAt: 1_794_150_000},
		{name: "resets_at far in the future", in: five(`{"used_percentage":12,"resets_at":1e300}`), hooks: true, size: true, cost: true},
		{name: "used_percentage null", in: five(`{"used_percentage":null,"resets_at":1794150000}`), hooks: true, size: true, cost: true},
		{name: "resets_at not a time", in: five(`{"used_percentage":12,"resets_at":"soon"}`), hooks: true, size: true, cost: true},
		{name: "resets_at missing", in: five(`{"used_percentage":12}`), hooks: true, size: true, cost: true},
		{name: "resets_at zero", in: five(`{"used_percentage":12,"resets_at":0}`), hooks: true, size: true, cost: true},
		{name: "used_percentage a string", in: five(`{"used_percentage":"12","resets_at":1794150000}`), hooks: true, size: true, cost: true},
		{name: "rate_limits a list", in: `{` + base + `,"rate_limits":[1]}`, hooks: true, size: true, cost: true},
		{name: "session_id a number", in: fmt.Sprintf(`{"session_id":1,"transcript_path":%q}`, path), size: true},
		{name: "not JSON", in: `not json`},
		{name: "a JSON list", in: `[1,2]`},
		{name: "empty", in: ``},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := env(t, t0.Add(2*time.Hour))
			e.Limits = filepath.Join(t.TempDir(), "limits.jsonl")
			var out bytes.Buffer
			StatusLine(strings.NewReader(c.in), &out, e)
			line := out.String()
			if c.size != strings.Contains(line, "250k tokens") {
				t.Errorf("status line %q: size shown %v, want %v", line, !c.size, c.size)
			}
			if c.cost != strings.Contains(line, "$4.10") {
				t.Errorf("status line %q: cost shown %v, want %v", line, !c.cost, c.cost)
			}
			if c.effortHas != "" && !strings.Contains(line, c.effortHas) {
				t.Errorf("status line %q: want effort %q", line, c.effortHas)
			}
			if !c.size && !c.cost && line != "" {
				t.Errorf("status line %q, want nothing", line)
			}
			b, err := os.ReadFile(e.Limits)
			switch {
			case c.resetsAt == 0 && err == nil:
				t.Errorf("limit reading kept: %s", b)
			case c.resetsAt != 0 && !strings.Contains(string(b), fmt.Sprintf(`"used":12,"resets_at":%d`, c.resetsAt)):
				t.Errorf("limit reading: %s (%v), want resets_at %d", b, err, c.resetsAt)
			}
			out.Reset()
			Hook("stop", strings.NewReader(c.in), &out, e)
			if got := strings.Contains(out.String(), "200k"); got != c.hooks {
				t.Errorf("stop hook: got %q, want a nudge %v", out.String(), c.hooks)
			}
		})
	}
}

// TestDecodeLimit checks which usage-limit readings are plausible enough to
// keep: a reading that is not would sort after every genuine one in the
// limits history, which would then drop them all.
func TestDecodeLimit(t *testing.T) {
	now := t0
	in3h := now.Add(3 * time.Hour).Unix()
	for _, c := range []struct {
		name, in string
		ok       bool
		used     float64
		resetsAt int64
	}{
		{name: "a normal reading", in: fmt.Sprintf(`{"used_percentage":12.5,"resets_at":%d}`, in3h), ok: true, used: 12.5, resetsAt: in3h},
		{name: "RFC 3339", in: `{"used_percentage":12,"resets_at":"2026-11-08T12:00:00Z"}`, ok: true, used: 12, resetsAt: in3h},
		{name: "milliseconds", in: fmt.Sprintf(`{"used_percentage":12,"resets_at":%d}`, in3h*1000), ok: true, used: 12, resetsAt: in3h},
		{name: "a weekly reset", in: fmt.Sprintf(`{"used_percentage":40,"resets_at":%d}`, now.Add(7*24*time.Hour).Unix()), ok: true, used: 40,
			resetsAt: now.Add(7 * 24 * time.Hour).Unix()},
		{name: "reset a few hours ago", in: fmt.Sprintf(`{"used_percentage":12,"resets_at":%d}`, now.Add(-5*time.Hour).Unix()), ok: true, used: 12,
			resetsAt: now.Add(-5 * time.Hour).Unix()},
		{name: "over 100 is read as 100", in: fmt.Sprintf(`{"used_percentage":150,"resets_at":%d}`, in3h), ok: true, used: 100, resetsAt: in3h},
		{name: "far future number", in: `{"used_percentage":12,"resets_at":1e300}`},
		{name: "far future RFC 3339", in: `{"used_percentage":12,"resets_at":"9999-12-31T00:00:00Z"}`},
		{name: "more than 8 days ahead", in: fmt.Sprintf(`{"used_percentage":12,"resets_at":%d}`, now.Add(9*24*time.Hour).Unix())},
		{name: "more than a day ago", in: fmt.Sprintf(`{"used_percentage":12,"resets_at":%d}`, now.Add(-25*time.Hour).Unix())},
		{name: "long past", in: `{"used_percentage":12,"resets_at":1000}`},
		{name: "used_percentage null", in: fmt.Sprintf(`{"used_percentage":null,"resets_at":%d}`, in3h)},
		{name: "used_percentage missing", in: fmt.Sprintf(`{"resets_at":%d}`, in3h)},
		{name: "used_percentage negative", in: fmt.Sprintf(`{"used_percentage":-1,"resets_at":%d}`, in3h)},
		{name: "resets_at null", in: `{"used_percentage":12,"resets_at":null}`},
		{name: "missing", in: ``},
	} {
		t.Run(c.name, func(t *testing.T) {
			var b json.RawMessage
			if c.in != "" {
				b = json.RawMessage(c.in)
			}
			r := decodeLimit(b, now)
			switch {
			case !c.ok && r != nil:
				t.Errorf("got %+v, want none", *r)
			case c.ok && r == nil:
				t.Error("got none, want a reading")
			case c.ok && (r.UsedPercentage != c.used || int64(r.ResetsAt) != c.resetsAt):
				t.Errorf("got %+v, want used %v, resets_at %d", *r, c.used, c.resetsAt)
			}
		})
	}
}
