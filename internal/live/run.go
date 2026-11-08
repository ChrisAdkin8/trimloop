package live

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source/claudecode"
)

// Input is the JSON Claude Code passes on standard input to a status-line
// command or a hook. Fields a version does not send stay empty.
type Input struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	HookEventName  string `json:"hook_event_name"`
	Cost           *struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
	} `json:"cost"`
	Effort *struct {
		Level string `json:"level"`
	} `json:"effort"`
}

// Env is what the commands need besides their input.
type Env struct {
	Settings Settings
	Store    Store  // per-session nudge state
	Log      string // file each shown nudge is appended to; "" for none
	Now      time.Time
}

// DefaultEnv keeps state and the nudge log under home/.trimloop.
func DefaultEnv(home string) Env {
	dir := filepath.Join(home, ".trimloop")
	return Env{
		Settings: DefaultSettings(),
		Store:    Store{Dir: filepath.Join(dir, "live")},
		Log:      filepath.Join(dir, "nudges.jsonl"),
		Now:      time.Now(),
	}
}

// readInput decodes the input and the session's latest reply.
func readInput(in io.Reader) (Input, Reply, error) {
	var inp Input
	if err := json.NewDecoder(in).Decode(&inp); err != nil {
		return inp, Reply{}, err
	}
	t, err := claudecode.ReadTail(inp.TranscriptPath)
	if err != nil {
		return inp, Reply{}, err
	}
	m := t.Latest
	r := Reply{Time: m.Time, Model: m.Model, Usage: m.Usage, Writer: t.Writer.Usage, Fast: m.Fast, USOnly: m.USOnly,
		Context: m.Usage.InputTotal()}
	if c := t.Compaction; c != nil && c.Compaction.After > 0 {
		// Compacted since the latest reply: the session is now the summary.
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
	inp, r, err := readInput(in)
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
	inp, r, err := readInput(in)
	if err != nil || inp.SessionID == "" {
		return
	}
	st := env.Store.Load(inp.SessionID)
	var output map[string]any
	var nudge string
	switch event {
	case "stop":
		if r.Context <= env.Settings.Large && len(st.Crossed) > 0 {
			// Compacted or cleared: the nudges can be useful again.
			st.Crossed = map[int]bool{}
			_ = env.Store.Save(inp.SessionID, st)
		}
		msg, threshold := env.Settings.AfterReply(r, st.Crossed)
		if msg == "" {
			return
		}
		st.Crossed[threshold] = true
		output = map[string]any{"systemMessage": msg}
		nudge = fmt.Sprintf("after-reply-%d", threshold)
	case "prompt":
		reason := env.Settings.BeforeSend(r, env.Now, st.PausedFor)
		if reason == "" {
			return
		}
		st.PausedFor = r.Time
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
