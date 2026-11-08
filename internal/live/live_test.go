package live

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	backup, err := WriteSettings(path, "/bin/trimloop", false)
	if err != nil || backup == "" {
		t.Fatalf("got %q, %v", backup, err)
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
	// Running it again adds nothing twice.
	if _, err := WriteSettings(path, "/bin/trimloop", false); err != nil {
		t.Fatal(err)
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
	if _, err := WriteSettings(other, "/bin/trimloop", false); !errors.Is(err, ErrStatusLineTaken) {
		t.Errorf("got %v, want ErrStatusLineTaken", err)
	}
	if _, err := WriteSettings(other, "/bin/trimloop", true); err != nil {
		t.Error(err)
	}
	// A new file is created, with no backup.
	if backup, err := WriteSettings(filepath.Join(dir, "new.json"), "/bin/trimloop", false); err != nil || backup != "" {
		t.Errorf("new file: got %q, %v", backup, err)
	}
	if _, err := WriteSettings(filepath.Join(dir, "missing", "x.json"), "/bin/trimloop", false); err == nil {
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
		if _, err := WriteSettings(p, "/bin/trimloop", false); err == nil {
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
	if _, err := WriteSettings(odd, "/bin/trimloop", false); err != nil {
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
	b1, err := WriteSettings(p, "/bin/trimloop", false)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "12345678901234567890") || !strings.Contains(string(got), `a && b < c > d`) {
		t.Errorf("values changed: %s", got)
	}
	if b2, err := WriteSettings(p, "/bin/trimloop", false); err != nil || b2 != "" {
		t.Errorf("second run should change nothing: backup %q, %v", b2, err)
	}
	// Force a second backup in the same second: it must not replace the first.
	if err := os.WriteFile(p, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	b3, err := WriteSettings(p, "/bin/trimloop", false)
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
	if _, err := WriteSettings(link, "/old/trimloop", false); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced by a file")
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode: got %v, want 0644 kept", fi.Mode().Perm())
	}

	// The binary moved: the status line and hooks follow, with no duplicates.
	if _, err := WriteSettings(link, "/new/trimloop", false); err != nil {
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
}
