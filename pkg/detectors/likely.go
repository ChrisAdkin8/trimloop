package detectors

import (
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// The findings' savings are upper bounds. The estimates here are likelier
// figures for one specific fix, replayed over the same turns. They assume
// the session would otherwise have gone the same way, which no fix
// guarantees, so reports show them as estimates beside the upper bound.

// LikelyCompactionSaving estimates what compacting each session whenever its
// context passed at tokens, down to after tokens, would have saved (P19),
// and the most that fix could save.
//
// It replays each session: from a simulated compaction on, every turn sends
// the tokens the compaction removed fewer, priced at what history cost on
// that turn (historyRate: a cache read, or a write when the cache was
// rebuilt). The turn still writes its own new tokens, so those are not
// counted as saved. Each simulated compaction costs a summary request (reading
// at tokens, writing about after tokens) and re-caching the summary. Real
// compactions in the log reset the replay. It assumes the session then went
// on the same way; re-reading files after a compaction would lower the
// saving.
//
// upTo is the same replay with every affected turn cut to after tokens and
// compactions free, so likely never exceeds it.
func LikelyCompactionSaving(sessions []event.Session, at, after int) (likely, upTo float64) {
	if after >= at {
		return 0, 0
	}
	for _, s := range sessions {
		removed := 0 // tokens the simulated compactions have removed so far
		for _, t := range s.Turns {
			if t.CompactedBefore {
				removed = 0
			}
			ctx := t.Context()
			if !t.Priced || ctx == 0 {
				continue
			}
			if ctx-removed > at {
				removed += ctx - removed - after
				likely -= compactionCost(t, at, after)
			}
			if cut := min(removed, ctx-after); cut > 0 {
				rate := historyRate(t)
				likely += float64(cut) * rate
				upTo += float64(ctx-after) * rate
			}
		}
	}
	return max(likely, 0), upTo
}

// compactionCost prices one compaction at turn t's model and options: the
// summary request reads the context and writes the summary, and the next
// turn writes the summary to the cache.
func compactionCost(t event.Turn, at, after int) float64 {
	c, ok := price.Cost(t.Model, event.Usage{CacheRead: at, CacheWrite1h: after, Output: after},
		price.Options{Fast: t.Fast, USOnly: t.USOnly})
	if !ok {
		return 0
	}
	return c.Total()
}

// LikelyPauseSaving estimates what starting a new session after each long
// pause would have saved (P09): the turn after the pause would have written
// fresh tokens to the cache instead of the whole context. fresh is the size
// of a new session's first request.
func LikelyPauseSaving(sessions []event.Session, minGap time.Duration, fresh int) float64 {
	saving := 0.0
	for _, s := range sessions {
		for _, t := range s.Turns {
			writes := t.Cost.CacheWrites()
			ctx := t.Context()
			if t.First || t.IdleBefore <= minGap || writes == 0 || !t.Priced || ctx <= fresh {
				continue
			}
			saving += writes * float64(ctx-fresh) / float64(ctx)
		}
	}
	return saving
}

// TypicalStart is the median context of the first turn of main sessions: what
// a new session costs to start. It returns 0 if there are none.
func TypicalStart(sessions []event.Session) int {
	var firsts []int
	for _, s := range sessions {
		if s.Kind == "main" && len(s.Turns) > 0 {
			firsts = append(firsts, s.Turns[0].Context())
		}
	}
	return median(firsts)
}
