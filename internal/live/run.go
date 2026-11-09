package live

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source/claudecode"
	"github.com/ChrisAdkin8/trimloop/internal/history"
	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
)

// Input is the JSON Claude Code passes on standard input to a status-line
// command or a hook. Fields a version does not send stay empty, and so does
// a field sent with an unexpected shape (decodeInput).
type Input struct {
	SessionID      string
	TranscriptPath string
	HookEventName  string
	Cost           *struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
	}
	Effort *struct {
		Level string `json:"level"`
	}
	// RateLimits is sent to the status line for Pro and Max subscribers,
	// after the session's first reply; either limit may be missing. It is
	// nil unless at least one limit decoded and is plausible (rateLimit).
	RateLimits *struct {
		FiveHour *rateLimit
		SevenDay *rateLimit
	}
}

// rateLimit is one usage limit as Claude Code reports it.
type rateLimit struct {
	UsedPercentage float64
	ResetsAt       float64 // Unix seconds
}

func (r *rateLimit) window() *detectors.LimitWindow {
	if r == nil {
		return nil
	}
	return &detectors.LimitWindow{Used: r.UsedPercentage, ResetsAt: int64(r.ResetsAt)}
}

// decodeInput reads the input one field at a time, so a field sent with a
// shape this version does not expect (Claude Code changes its input between
// releases) loses only that field. Only input that is not a JSON object at
// all is an error. now is the clock usage-limit readings are checked against.
func decodeInput(in io.Reader, now time.Time) (Input, error) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(in).Decode(&raw); err != nil {
		return Input{}, err
	}
	var inp Input
	field(raw, "session_id", &inp.SessionID)
	field(raw, "transcript_path", &inp.TranscriptPath)
	field(raw, "hook_event_name", &inp.HookEventName)
	field(raw, "cost", &inp.Cost)
	field(raw, "effort", &inp.Effort)
	var limits map[string]json.RawMessage
	if field(raw, "rate_limits", &limits) {
		five, seven := decodeLimit(limits["five_hour"], now), decodeLimit(limits["seven_day"], now)
		if five != nil || seven != nil {
			inp.RateLimits = &struct {
				FiveHour *rateLimit
				SevenDay *rateLimit
			}{five, seven}
		}
	}
	return inp, nil
}

// field decodes raw[key] into v, leaving v as it was if the key is missing
// or does not fit, and says whether it decoded.
func field[T any](raw map[string]json.RawMessage, key string, v *T) bool {
	b, ok := raw[key]
	if !ok {
		return false
	}
	var x T
	if json.Unmarshal(b, &x) != nil {
		return false
	}
	*v = x
	return true
}

// Bounds on a plausible reset time, around now: a window that reset up to a
// day ago (a reading from an idle session), or one resetting within the
// weekly window plus slack. A reset time outside them would sort after every
// genuine reading, so the limits history would drop all later ones.
const (
	resetsAtPast   = 24 * time.Hour
	resetsAtFuture = 8 * 24 * time.Hour
	// msAbove is the largest value read as Unix seconds (the year 5138);
	// above it a number is taken as Unix milliseconds.
	msAbove = 1e11
)

// decodeLimit decodes one usage limit, or returns nil when it is missing,
// does not fit, or is not plausible: a reading needs a use (a number, not
// negative; above 100 is read as 100) and a reset time between a day before
// now and eight days after. resets_at is Unix seconds (ADR 0048); Unix
// milliseconds and an RFC 3339 time are accepted too.
func decodeLimit(b json.RawMessage, now time.Time) *rateLimit {
	var raw map[string]json.RawMessage
	if b == nil || json.Unmarshal(b, &raw) != nil {
		return nil
	}
	var used *float64 // nil when sent as null, which would otherwise read as 0
	if !field(raw, "used_percentage", &used) || used == nil || *used < 0 {
		return nil
	}
	r := rateLimit{UsedPercentage: min(*used, 100)}
	var at string
	switch {
	case field(raw, "resets_at", &r.ResetsAt):
		if r.ResetsAt > msAbove {
			r.ResetsAt /= 1000
		}
	case field(raw, "resets_at", &at):
		t, err := time.Parse(time.RFC3339, at)
		if err != nil {
			return nil
		}
		r.ResetsAt = float64(t.Unix())
	}
	lo, hi := float64(now.Add(-resetsAtPast).Unix()), float64(now.Add(resetsAtFuture).Unix())
	if r.ResetsAt < lo || r.ResetsAt > hi {
		return nil
	}
	return &r
}

// Env is what the commands need besides their input.
type Env struct {
	Settings Settings
	Store    Store  // per-session nudge state
	Log      string // file each shown nudge is appended to; "" for none
	Limits   string // file usage-limit readings are appended to; "" for none (ADR 0048)
	Now      time.Time
}

// DefaultEnv keeps state and the nudge log under home/.trimloop.
func DefaultEnv(home string) Env {
	dir := filepath.Join(home, ".trimloop")
	return Env{
		Settings: DefaultSettings(),
		Store:    Store{Dir: filepath.Join(dir, "live")},
		Log:      filepath.Join(dir, "nudges.jsonl"),
		Limits:   history.LimitsPath(home, os.Getenv("CLAUDE_CONFIG_DIR")),
		Now:      time.Now(),
	}
}

// readInput decodes the input and the session's latest reply.
func readInput(in io.Reader, now time.Time) (Input, Reply, error) {
	inp, err := decodeInput(in, now)
	if err != nil {
		return inp, Reply{}, err
	}
	t, err := claudecode.ReadTail(inp.TranscriptPath)
	if err != nil {
		return inp, Reply{}, err
	}
	m := t.Latest
	r := Reply{Time: m.Time, Model: m.Model, Usage: m.Usage, Writer: t.Writer.Usage, Fast: m.Fast, USOnly: m.USOnly,
		Context: m.Usage.InputTotal()}
	if c := t.Compaction; c != nil {
		// Compacted since the latest reply. The harness records only the
		// summary's size; the next request also sends the system prompt and
		// re-attached files (2–5 times more), so no size is shown until then.
		r.Compacted, r.Context = true, c.Compaction.After
		if !c.Time.IsZero() {
			r.Time = c.Time
		}
	}
	return inp, r, nil
}

// ANSI colours for the status line.
const (
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
	dim    = "\033[2m"
	reset  = "\033[0m"
)

// StatusLine writes the status line. Claude Code runs it after every reply
// and on its refresh timer, so it reads only the end of one log. On any
// problem it prints what it can, and never fails.
func StatusLine(in io.Reader, out io.Writer, env Env) {
	inp, r, err := readInput(in, env.Now)
	recordLimits(env, inp)
	cost := -1.0
	if inp.Cost != nil {
		cost = inp.Cost.TotalCostUSD
	}
	if err != nil {
		if cost >= 0 {
			_, _ = fmt.Fprintln(out, money(cost))
		}
		return
	}
	effort := ""
	if inp.Effort != nil {
		effort = inp.Effort.Level
	}
	p := env.Settings.Status(r, env.Now, cost, effort)
	colour := map[Level]string{Small: green, Large: yellow, VeryLarge: red}[p.Level]
	line := colour + p.Context + reset
	for _, part := range []string{p.Cost, p.Effort} {
		if part != "" {
			line += dim + " · " + reset + part
		}
	}
	if p.Cache != "" {
		c := yellow
		if r.Context > env.Settings.Large {
			c = red
		}
		line += dim + " · " + reset + c + p.Cache + reset
	}
	_, _ = fmt.Fprintln(out, line)
}

// Hook runs one hook: "stop" after Claude replies, "prompt" when the user
// sends a message. It writes Claude Code's JSON hook output when a nudge is
// due and nothing otherwise, and never fails: a problem means no nudge.
// It uses only fields shown to the user (systemMessage, and the reason for
// holding a prompt); whether systemMessage also reaches the model is not
// documented (ADR 0034).
func Hook(event string, in io.Reader, out io.Writer, env Env) {
	inp, r, err := readInput(in, env.Now)
	if err != nil || inp.SessionID == "" {
		return
	}
	st := env.Store.Load(inp.SessionID)
	var output map[string]any
	var nudge string
	switch event {
	case "stop":
		msg, threshold, rearmed := st.afterReply(env.Settings, r)
		if msg == "" {
			if rearmed {
				_ = env.Store.Save(inp.SessionID, st)
			}
			return
		}
		output = map[string]any{"systemMessage": msg}
		nudge = fmt.Sprintf("after-reply-%d", threshold)
	case "prompt":
		reason := st.beforeSend(env.Settings, r, env.Now)
		if reason == "" {
			return
		}
		output = map[string]any{"decision": "block", "reason": reason}
		nudge = "pause-after-break"
	default:
		return
	}
	if err := env.Store.Save(inp.SessionID, st); err != nil {
		return // without saving, the nudge would repeat: better none
	}
	b, err := json.Marshal(output)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintln(out, string(b))
	logNudge(env, nudge, r.Context)
}

// logNudge appends a shown nudge to the log, so a later report can see what
// followed. It records the time, the nudge and the context size only.
func logNudge(env Env, nudge string, context int) {
	if env.Log == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(env.Log), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(env.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	b, _ := json.Marshal(map[string]any{"time": env.Now.UTC(), "nudge": nudge, "context": context})
	_, _ = f.Write(append(b, '\n'))
}

// recordLimits keeps the usage-limit reading Claude Code sent, if any, so a
// report can measure how much of the limits list-price spend uses
// (ADR 0048). A problem is ignored: the status line must still show.
func recordLimits(env Env, inp Input) {
	if env.Limits == "" || inp.RateLimits == nil {
		return
	}
	_ = history.AppendLimits(env.Limits, detectors.LimitReading{
		At:       env.Now.UTC(),
		FiveHour: inp.RateLimits.FiveHour.window(),
		SevenDay: inp.RateLimits.SevenDay.window(),
	})
}
