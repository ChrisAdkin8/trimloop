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
	Sessions int     `json:"sessions"` // sessions with at least one such pause
	Cost     float64 `json:"cost"`     // cache-write cost of those turns

	// Saving is Cost: what those re-writes would have saved had the session
	// been compacted, or a new one started, before carrying on. It is an
	// upper bound: a compacted or new session still writes its smaller
	// context.
	Saving float64 `json:"saving"`

	Top []SessionStat `json:"top"` // sessions with the largest re-cache cost
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

			st := stat(stats, s.ID)
			st.Turns++
			st.Cost += writes
			st.Saving += writes
			st.Peak = max(st.Peak, t.Context())
		}
	}
	f.Saving = f.Cost
	f.Sessions = len(stats)
	f.Top = top(stats, topN)
	return f
}
