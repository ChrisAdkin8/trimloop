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

// LikelyCompactionSaving estimates what compacting each main session
// whenever its context passed at tokens would have saved (P19), and the most
// that fix could save. after is the context the first request after a
// compaction sends (the summary, the system prompt and tool definitions, and
// what the harness re-attaches); summary is the summary's own size, which
// the compaction request writes as output.
//
// It replays each main session (sub-agents cannot be compacted by the user):
// from a simulated compaction on, every turn sends the tokens the compaction
// removed fewer, priced at what history cost on that turn (historyRate: a
// cache read, or a write when the cache was rebuilt). The turn still writes
// its own new tokens, so those are not counted as saved. A simulated
// compaction happens before the turn that would pass at, at the session's
// size after the previous turn, and costs a summary request (reading that
// size, writing the summary) and writing the new context to the cache. Real
// compactions in the log reset the replay. It assumes the session then went
// on the same way; re-reading files after a compaction would lower the
// saving.
//
// upTo is the same replay with every affected turn cut to after tokens and
// compactions free, so likely never exceeds it.
func LikelyCompactionSaving(sessions []event.Session, at, after, summary int) (likely, upTo float64) {
	if after >= at {
		return 0, 0
	}
	for _, s := range sessions {
		if s.Kind == "subagent" {
			continue
		}
		removed := 0 // tokens the simulated compactions have removed so far
		prev := 0    // the previous turn's context, as logged
		for _, t := range s.Turns {
			if t.CompactedBefore {
				removed, prev = 0, 0 // a real compaction: nothing left to compact before this turn
			}
			ctx := t.Context()
			if !t.Priced || ctx == 0 {
				continue
			}
			// Compact before this turn when it would pass the limit: the
			// session is then its size after the previous turn, and this
			// turn's own new tokens stay.
			if size := prev - removed; ctx-removed > at && size > after {
				removed += size - after
				likely -= compactionCost(t, size, after, summary)
			}
			prev = ctx
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
// summary request reads the session and writes the summary, and the next
// turn writes the new context to the cache. Some of that context (the system
// prompt and tool definitions) is usually still cached, so this errs high.
func compactionCost(t event.Turn, size, after, summary int) float64 {
	c, ok := price.Cost(t.Model, event.Usage{CacheRead: size, CacheWrite1h: after, Output: summary},
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
			if !userBreak(s, t, minGap) || writes == 0 || ctx <= fresh {
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
