package detectors

import (
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// Cache lifetimes Anthropic offers (docs/measurements.md §0).
const (
	shortLife = 5 * time.Minute
	longLife  = time.Hour
)

// CacheLife is P02, cache TTL expiry, in reduced fidelity (ADR 0042). A
// session whose cache lasts 5 minutes (sub-agents and SDK runs, in the
// founder's logs) that waits
// longer, up to the hour P09 starts at, pays to write its whole history
// again; the 1-hour lifetime would have kept it. It also compares the
// sessions that do use the hour with what the 5-minute lifetime would have
// cost them, so the report can say whether the hour pays off.
type CacheLife struct {
	// Waits counts turns after a wait of 5 minutes to an hour in a session
	// on the 5-minute cache, whose cache had expired; SubAgent of them in
	// sub-agents.
	Waits    int     `json:"waits"`
	SubAgent int     `json:"sub_agent"`
	Sessions int     `json:"sessions"`
	Cost     float64 `json:"cost"` // what those turns cost

	// Saving is what the expiry cost: those turns' cost less what they would
	// have cost with the cache still there (the history read, only the new
	// part written). Avoiding the wait (the card's advice) saves this.
	Saving float64 `json:"saving"`

	// HourNet is what putting every session on the 5-minute cache on the
	// hour's instead would have saved: its waits no longer expire, but every
	// other write in every such session costs the hour's higher price. Over
	// all of them, not only those known afterwards to have waited, since the
	// lifetime is chosen before anyone knows. Usually negative.
	HourNet float64 `json:"hour_net"`

	// HourActual is what the turns of sessions on the 1-hour cache cost;
	// HourAsShort is what they would have cost on the 5-minute one, with the
	// history written again after every wait of over 5 minutes (up to an
	// hour, when both would have expired); HourAsShortLow counts only waits
	// over 10 minutes. The two bound it: a logged wait includes the next
	// reply's own time, and requests the logs do not record can keep a cache
	// alive, so not every wait over 5 minutes would have expired (ADR 0044).
	// Counted from a session's first write on the hour.
	HourActual     float64 `json:"hour_actual"`
	HourAsShort    float64 `json:"hour_as_short"`
	HourAsShortLow float64 `json:"hour_as_short_low"`

	// Sessions by the lifetime they wrote with, so advice can say which
	// kind of session keeps its cache longer in these logs. Sessions that
	// wrote with both count as the hour.
	MainShort int `json:"main_short"`
	MainHour  int `json:"main_hour"`
	SubShort  int `json:"sub_short"`
	SubHour   int `json:"sub_hour"`

	Top []SessionStat `json:"top"`
}

// shortWait is one P02 turn: its index in the session, what the expiry
// cost on it, and what the hour's cache would have saved on it.
type shortWait struct {
	turn   int
	saving float64 // against the cache still there, on the 5-minute price
	hour   float64 // against the hour's cache: the new part at the hour's price
}

// shortWaits finds the P02 turns of a session: after a wait of 5 minutes to
// an hour, in a session whose latest cache write was on the 5-minute cache,
// with the cache expired. The saving is the turn's cost less what it would
// have cost with the cache still there (the earlier context read, the new
// part written at the session's 5-minute price).
func shortWaits(s event.Session) []shortWait {
	var out []shortWait
	life := time.Duration(0) // the lifetime of the session's latest cache write so far
	prev := 0                // the previous turn's context
	for i, t := range s.Turns {
		u := t.Usage
		ctx := t.Context()
		if i > 0 && t.Priced && !t.CompactedBefore && life == shortLife &&
			t.IdleBefore > shortLife && t.IdleBefore <= longLife &&
			t.Model == s.Turns[i-1].Model && u.CacheRead < ctx/5 {
			opts := price.Options{Fast: t.Fast, USOnly: t.USOnly}
			warm := event.Usage{InputUncached: u.InputUncached, Output: u.Output,
				CacheRead: min(prev, ctx-u.InputUncached), CacheWrite5m: max(0, ctx-u.InputUncached-prev)}
			hour := warm
			hour.CacheWrite1h, hour.CacheWrite5m = hour.CacheWrite5m, 0
			alt, ok := price.Cost(t.Model, warm, opts)
			alt1h, ok1h := price.Cost(t.Model, hour, opts)
			if ok && ok1h && t.Cost.Total() > alt.Total() {
				out = append(out, shortWait{turn: i, saving: t.Cost.Total() - alt.Total(), hour: t.Cost.Total() - alt1h.Total()})
			}
		}
		switch {
		case u.CacheWrite1h > 0:
			life = longLife
		case u.CacheWrite5m > 0:
			life = shortLife
		}
		prev = ctx
	}
	return out
}

// FindCacheLife reports P02 and the 1-hour comparison.
func FindCacheLife(sessions []event.Session, topN int) CacheLife {
	var f CacheLife
	stats := map[string]*SessionStat{}
	for _, s := range sessions {
		short, hour := false, false
		for _, t := range s.Turns {
			short = short || t.Usage.CacheWrite5m > 0
			hour = hour || t.Usage.CacheWrite1h > 0
		}
		switch {
		case hour && s.Kind == "subagent":
			f.SubHour++
		case hour:
			f.MainHour++
		case short && s.Kind == "subagent":
			f.SubShort++
		case short:
			f.MainShort++
		}
		waits := shortWaits(s)
		isWait := map[int]bool{}
		for _, w := range waits {
			isWait[w.turn] = true
			if short && !hour { // the hour's option is for sessions wholly on the 5-minute cache
				f.HourNet += w.hour
			}
		}
		if short && !hour {
			// The hour's premium on every other write in the session.
			for i, t := range s.Turns {
				if isWait[i] || !t.Priced || t.Usage.CacheWrite5m == 0 {
					continue
				}
				u := t.Usage
				u.CacheWrite1h, u.CacheWrite5m = u.CacheWrite1h+u.CacheWrite5m, 0
				if c, ok := price.Cost(t.Model, u, price.Options{Fast: t.Fast, USOnly: t.USOnly}); ok {
					f.HourNet -= c.Total() - t.Cost.Total()
				}
			}
		}
		for _, w := range waits {
			t := s.Turns[w.turn]
			f.Waits++
			if s.Kind == "subagent" {
				f.SubAgent++
			}
			f.Cost += t.Cost.Total()
			f.Saving += w.saving
			st := stat(stats, s.ID)
			st.Turns++
			st.Cost += t.Cost.Total()
			st.Saving += w.saving
			st.Peak = max(st.Peak, t.Context())
		}
	}
	for _, s := range sessions {
		onHour := false // from the session's first write on the hour
		for _, t := range s.Turns {
			u := t.Usage
			onHour = onHour || u.CacheWrite1h > 0
			if t.Priced && onHour {
				opts := price.Options{Fast: t.Fast, USOnly: t.USOnly}
				hi, okHi := price.Cost(t.Model, onShort(u, t.IdleBefore, shortLife), opts)
				lo, okLo := price.Cost(t.Model, onShort(u, t.IdleBefore, 2*shortLife), opts)
				if okHi && okLo {
					f.HourActual += t.Cost.Total()
					f.HourAsShort += hi.Total()
					f.HourAsShortLow += lo.Total()
				}
			}
		}
	}
	f.Sessions = len(stats)
	f.Top = top(stats, topN)
	return f
}

// onShort is a turn's usage as the 5-minute cache would have made it: its
// writes at the 5-minute price, and, after a wait longer than expiry (and up
// to an hour, past which both lifetimes expire), what it read written again.
func onShort(u event.Usage, idle, expiry time.Duration) event.Usage {
	u.CacheWrite5m, u.CacheWrite1h = u.CacheWrite5m+u.CacheWrite1h, 0
	if u.CacheRead > 0 && idle > expiry && idle <= longLife {
		u.CacheWrite5m += u.CacheRead
		u.CacheRead = 0
	}
	return u
}
