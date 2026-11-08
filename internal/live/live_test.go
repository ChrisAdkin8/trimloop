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
	if !strings.Contains(r, "away 3 hours") || !strings.Contains(r, "610k tokens") || !strings.Contains(r, "Enter again") {
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
	if err := s.Save("../escape", st); err != nil || fileExists(filepath.Join(s.Dir, "..", "escape.json")) {
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
	if cfg["model"] != "opus" || !strings.Contains(string(b), "say done") || !strings.Contains(string(b), `\"/bin/trimloop\" hook stop`) {
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
