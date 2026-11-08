// Package live gives nudges inside Claude Code while a session runs: a status
// line, a message after a reply that makes the session large, and a one-time
// pause before an expensive message after a long break (ADR 0034). It reads
// only the end of the current session's log and never adds text to Claude's
// conversation.
package live

import (
	"fmt"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// Settings are the thresholds the nudges use.
type Settings struct {
	Large     int           // context tokens at which to suggest compacting (P19)
	VeryLarge int           // context tokens at which to warn more strongly and pause after breaks
	Warn      time.Duration // how long before the cache expires to show the countdown
	Fresh     int           // tokens a fresh session starts with, for comparisons
}

// DefaultSettings are the thresholds measured on the founder's logs: 200k
// is P19's threshold; past 500k a turn cost about 5× a fresh session's.
func DefaultSettings() Settings {
	return Settings{Large: 200_000, VeryLarge: 500_000, Warn: 10 * time.Minute, Fresh: 30_000}
}

// Reply is what the nudges need to know about the session's latest reply.
type Reply struct {
	Time    time.Time
	Model   string
	Usage   event.Usage
	Fast    bool
	USOnly  bool
	Context int // tokens the request sent
}

// CacheLife is how long the reply's cache lasts: an hour if it wrote with the
// long lifetime, else five minutes if it wrote with the short one; an hour
// if it wrote nothing (Claude Code asks for the hour for main sessions).
func (r Reply) CacheLife() time.Duration {
	if r.Usage.CacheWrite5m > 0 && r.Usage.CacheWrite1h == 0 {
		return 5 * time.Minute
	}
	return time.Hour
}

// ResendCost estimates what the next message costs to send the whole
// conversation again once the cache has expired: every token written to the
// cache again. ok is false if the model has no price.
func (r Reply) ResendCost() (float64, bool) {
	u := event.Usage{CacheWrite1h: r.Context}
	if r.CacheLife() < time.Hour {
		u = event.Usage{CacheWrite5m: r.Context}
	}
	c, ok := price.Cost(r.Model, u, price.Options{Fast: r.Fast, USOnly: r.USOnly})
	return c.Total(), ok
}

// Level is how large a session is, for colour.
type Level int

// The levels.
const (
	Small Level = iota
	Large
	VeryLarge
)

// LevelOf sorts a context size.
func (s Settings) LevelOf(context int) Level {
	switch {
	case context > s.VeryLarge:
		return VeryLarge
	case context > s.Large:
		return Large
	}
	return Small
}

// StatusParts are the pieces of the status line, before colouring.
type StatusParts struct {
	Context string // "312k tokens"
	Level   Level
	Cost    string // "$4.10" (the harness's own figure), "" if not known
	Cache   string // "cache ends in 8 min", "cache expired · next message ≈ $4.90", or ""
	Effort  string // "max effort" when effort is max or xhigh, else ""
}

// Status works out the status line for the latest reply at time now. cost is
// the session's cost so far as the harness reports it, or negative if not
// known; effort is the harness's effort level, or "".
func (s Settings) Status(r Reply, now time.Time, cost float64, effort string) StatusParts {
	p := StatusParts{Context: tokens(r.Context) + " tokens", Level: s.LevelOf(r.Context)}
	if cost >= 0 {
		p.Cost = money(cost)
	}
	if effort == "max" || effort == "xhigh" {
		p.Effort = effort + " effort"
	}
	if r.Time.IsZero() {
		return p
	}
	left := r.Time.Add(r.CacheLife()).Sub(now)
	switch {
	case left <= 0:
		p.Cache = "cache expired"
		if c, ok := r.ResendCost(); ok && c >= 0.01 {
			p.Cache += " · next message ≈ " + money(c)
		}
	case left <= s.Warn:
		p.Cache = fmt.Sprintf("cache ends in %d min", int(left.Minutes())+1)
	}
	return p
}

// AfterReply returns the message to show after a reply, and the threshold it
// is for, or "" if none is due. crossed holds the thresholds already shown
// for this session, so each is shown once.
func (s Settings) AfterReply(r Reply, crossed map[int]bool) (string, int) {
	switch {
	case r.Context > s.VeryLarge && !crossed[s.VeryLarge]:
		msg := fmt.Sprintf("This session is now %s tokens. Every turn re-sends all of it", tokens(r.Context))
		if s.Fresh > 0 {
			msg += fmt.Sprintf(", about %d× what a fresh session sends", r.Context/s.Fresh)
		}
		return msg + ". Finish the current step, then run /compact or start a new session.", s.VeryLarge
	case r.Context > s.Large && !crossed[s.Large] && !crossed[s.VeryLarge]:
		return fmt.Sprintf("This session just passed %s tokens, and each turn now re-sends all of it. "+
			"At the next natural break, run /compact, or start a new session for the next task.", tokens(s.Large)), s.Large
	}
	return "", 0
}

// BeforeSend decides whether to pause a message the user just sent: only when
// the cache has expired and the session is very large, and only once per
// break (pausedFor is the reply time a pause was already shown for). It
// returns the reason to show, or "" to let the message through.
func (s Settings) BeforeSend(r Reply, now time.Time, pausedFor time.Time) string {
	if r.Time.IsZero() || r.Context <= s.VeryLarge || !pausedFor.IsZero() && pausedFor.Equal(r.Time) {
		return ""
	}
	away := now.Sub(r.Time)
	if away <= r.CacheLife() {
		return ""
	}
	cost := ""
	if c, ok := r.ResendCost(); ok {
		cost = fmt.Sprintf(" (about %s)", money(c))
	}
	return fmt.Sprintf("Paused once: you were away %s, so the saved copy of this conversation (the cache) has expired. "+
		"Sending now re-sends all %s tokens%s. Press Enter again to send anyway, or run /clear and paste a short "+
		"note of where things stand to start fresh.", awayText(away), tokens(r.Context), cost)
}

// tokens writes a token count briefly: 312k, 1.2M.
func tokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%dk", (n+500)/1000)
	}
	return fmt.Sprint(n)
}

func money(v float64) string {
	if v >= 10 {
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

// awayText writes a time away: "75 min", "3 hours", "2 days".
func awayText(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	case d >= 2*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	return fmt.Sprintf("%d min", int(d.Minutes()))
}
