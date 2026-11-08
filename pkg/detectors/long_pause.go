package detectors

import (
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// LongPause is P09, resume bloat, in reduced fidelity: sessions picked up
// again after a pause longer than the cache lives. The cached context has
// expired, so the first turn after the pause writes the whole context to the
// cache again.
type LongPause struct {
	MinGap time.Duration `json:"min_gap_ns"` // pauses longer than this count

	Pauses   int     `json:"pauses"`   // turns that followed such a pause and wrote to the cache
	Sessions int     `json:"sessions"` // sessions with at least one break the user took
	Cost     float64 `json:"cost"`     // cache-write cost of those turns

	// Unprompted counts the pauses that were not breaks in a session the
	// user could restart: no message from the user ended them (the session
	// was waiting on a tool, a sub-agent, a timer or another agent), or they
	// were in a sub-agent. Starting fresh does not apply to them.
	// UnpromptedCost is their share of Cost.
	Unprompted     int     `json:"unprompted"`
	UnpromptedCost float64 `json:"unprompted_cost"`

	// Saving is the part of Cost after breaks the user took (a message from
	// the user ended the wait): what those re-writes would have saved had
	// the session been compacted before the break, or a new one started
	// after it. It is an upper bound: a compacted or new session still
	// writes its smaller context.
	Saving float64 `json:"saving"`

	Top []SessionStat `json:"top"` // sessions with the largest re-cache cost after breaks the user took
}

// FindLongPauses reports turns that came after more than minGap idle and
// re-wrote the cache. With minGap of an hour, even the longer cache lifetime
// had expired.
func FindLongPauses(sessions []event.Session, minGap time.Duration, topN int) LongPause {
	f := LongPause{MinGap: minGap}
	stats := map[string]*SessionStat{}
	for _, s := range sessions {
		for _, t := range s.Turns {
			writes := t.Cost.CacheWrites()
			if t.First || t.IdleBefore <= minGap || writes == 0 || !t.Priced {
				continue
			}
			f.Pauses++
			f.Cost += writes
			if !userBreak(s, t, minGap) {
				f.Unprompted++
				f.UnpromptedCost += writes
				continue
			}
			f.Saving += writes

			st := stat(stats, s.ID)
			st.Turns++
			st.Cost += writes
			st.Saving += writes
			st.Peak = max(st.Peak, t.Context())
		}
	}
	f.Sessions = len(stats)
	f.Top = top(stats, topN)
	return f
}

// userBreak reports whether turn t of session s came after a break the user
// took and could have started fresh from: a long pause in a main session
// that a message from the user ended, with a priced turn.
func userBreak(s event.Session, t event.Turn, minGap time.Duration) bool {
	return s.Kind != event.SubAgentSession && !t.First && t.Prompted && t.IdleBefore > minGap && t.Priced
}
