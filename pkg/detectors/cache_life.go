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
// session whose cache lasts 5 minutes (Claude Code's sub-agents) that waits
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

	// Saving is what they would have cost less had the cache lasted an
	// hour: the history read rather than written again.
	Saving float64 `json:"saving"`

	// HourActual is what the turns that wrote with the 1-hour lifetime
	// cost; HourAsShort is what they would have cost on the 5-minute one,
	// with the history written again after every wait over 5 minutes.
	HourActual  float64 `json:"hour_actual"`
	HourAsShort float64 `json:"hour_as_short"`

	Top []SessionStat `json:"top"`
}

// FindCacheLife reports P02 and the 1-hour comparison.
func FindCacheLife(sessions []event.Session, topN int) CacheLife {
	var f CacheLife
	stats := map[string]*SessionStat{}
	for _, s := range sessions {
		life := time.Duration(0) // the lifetime of the session's latest cache write so far
		prev := 0                // the previous turn's context
		for i, t := range s.Turns {
			u := t.Usage
			ctx := t.Context()
			opts := price.Options{Fast: t.Fast, USOnly: t.USOnly}

			// P02: expired after a 5-minute-to-an-hour wait on the 5-minute cache.
			if i > 0 && t.Priced && !t.CompactedBefore && life == shortLife &&
				t.IdleBefore > shortLife && t.IdleBefore <= longLife &&
				t.Model == s.Turns[i-1].Model && u.CacheRead < ctx/5 {
				// On the hour's cache, the previous context would have been read
				// and only what is new written.
				warm := event.Usage{InputUncached: u.InputUncached, Output: u.Output,
					CacheRead: min(prev, ctx-u.InputUncached), CacheWrite1h: max(0, ctx-u.InputUncached-prev)}
				if alt, ok := price.Cost(t.Model, warm, opts); ok && t.Cost.Total() > alt.Total() {
					saving := t.Cost.Total() - alt.Total()
					f.Waits++
					if s.Kind == "subagent" {
						f.SubAgent++
					}
					f.Cost += t.Cost.Total()
					f.Saving += saving
					st := stat(stats, s.ID)
					st.Turns++
					st.Cost += t.Cost.Total()
					st.Saving += saving
					st.Peak = max(st.Peak, ctx)
				}
			}

			// The hour against 5 minutes, for turns that wrote with the hour.
			if t.Priced && u.CacheWrite1h > 0 {
				short := u
				short.CacheWrite5m, short.CacheWrite1h = u.CacheWrite5m+u.CacheWrite1h, 0
				if u.CacheRead > 0 && t.IdleBefore > shortLife {
					// The 5-minute cache would have expired: all of it written again.
					short.CacheWrite5m += short.CacheRead
					short.CacheRead = 0
				}
				if alt, ok := price.Cost(t.Model, short, opts); ok {
					f.HourActual += t.Cost.Total()
					f.HourAsShort += alt.Total()
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
	}
	f.Sessions = len(stats)
	f.Top = top(stats, topN)
	return f
}
