package detectors

import (
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// ModelSwitches is P01 (cache-prefix churn) in reduced fidelity, for the one
// cause the logs show directly: a session moving to another model mid-way.
// The cache belongs to a model, so the first turn on the new one writes the
// whole context again (ADR 0050).
type ModelSwitches struct {
	Switches int           `json:"switches"`
	Cost     float64       `json:"cost"`   // what the switching turns cost
	Saving   float64       `json:"saving"` // those turns less the same turns reading the context instead
	Sessions int           `json:"sessions"`
	Top      []SessionStat `json:"top"`
}

// minSwitchContext is the least context a switch is counted at: below it,
// writing the context again costs cents.
const minSwitchContext = 20_000

// FindModelSwitches counts turns on a different model from the session's
// previous turn, not after a compaction, while the previous turn's cache
// was still alive, at a previous context over minSwitchContext.
func FindModelSwitches(sessions []event.Session, topN int) ModelSwitches {
	var f ModelSwitches
	stats := map[string]*SessionStat{}
	for _, s := range sessions {
		life := time.Duration(0) // the lifetime of the session's latest cache write
		for i, t := range s.Turns {
			if i > 0 {
				p := s.Turns[i-1]
				prev := p.Context()
				if t.Model != p.Model && t.Priced && !t.CompactedBefore && prev > minSwitchContext &&
					life > 0 && t.IdleBefore <= life {
					saving := t.Cost.Total() - readInstead(t, prev)
					if saving > 0 {
						f.Switches++
						f.Cost += t.Cost.Total()
						f.Saving += saving
						st := stat(stats, s.ID)
						st.Turns++
						st.Cost += t.Cost.Total()
						st.Saving += saving
						st.Peak = max(st.Peak, t.Context())
					}
				}
			}
			if l := t.Usage.WriteLife(); l != 0 {
				life = l
			}
		}
	}
	f.Sessions = len(stats)
	f.Top = top(stats, topN)
	return f
}

// readInstead is what turn t would have cost reading the earlier context
// (prev tokens) from the cache instead of writing it, writing only its new
// part, at its own model's prices and lifetime.
func readInstead(t event.Turn, prev int) float64 {
	u := t.Usage
	ctx := t.Context()
	read := min(prev, ctx-u.InputUncached)
	alt := event.Usage{InputUncached: u.InputUncached, Output: u.Output, CacheRead: read}
	fresh := max(0, ctx-u.InputUncached-read)
	if u.WriteLife() == event.LongCacheLife {
		alt.CacheWrite1h = fresh
	} else {
		alt.CacheWrite5m = fresh
	}
	c, ok := price.Cost(t.Model, alt, price.Options{Fast: t.Fast, USOnly: t.USOnly})
	if !ok {
		return t.Cost.Total()
	}
	return c.Total()
}

// Failures is P12 (retries) in reduced fidelity: requests that failed, as
// the harness's own error replies record them, and were sent again
// (ADR 0050).
type Failures struct {
	Sleep    int `json:"sleep"`   // the computer slept mid-response
	Network  int `json:"network"` // the connection failed
	Other    int `json:"other"`
	Sessions int `json:"sessions"`
	// Resent is what sending the context again cost, at least: each
	// failure's context re-read from the cache. The cut-off output is not
	// logged, so it is not counted.
	Resent      float64 `json:"resent"`
	SleepResent float64 `json:"sleep_resent"`
	// MedianContext is the median context, in tokens, of the requests the
	// computer's sleeping cut off.
	MedianContext int `json:"median_context"`
}

// FindFailures counts failed requests and what re-sending them cost.
func FindFailures(sessions []event.Session) Failures {
	var f Failures
	var sleepCtx []int
	for _, s := range sessions {
		if len(s.Failures) > 0 {
			f.Sessions++
		}
		for _, x := range s.Failures {
			resent := 0.0
			if c, ok := price.Cost(x.Model, event.Usage{CacheRead: x.Context}, price.Options{}); ok && x.Context > 0 {
				resent = c.Total()
			}
			switch x.Kind {
			case "sleep":
				f.Sleep++
				f.SleepResent += resent
				sleepCtx = append(sleepCtx, x.Context)
			case "network":
				f.Network++
			default:
				f.Other++
			}
			f.Resent += resent
		}
	}
	f.MedianContext = median(sleepCtx)
	return f
}
