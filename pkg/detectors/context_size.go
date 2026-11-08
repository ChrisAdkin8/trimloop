package detectors

import "github.com/ChrisAdkin8/trimloop/pkg/event"

// ContextSize is P19, large working context: turns that send a very large
// context. Every turn re-sends the whole conversation, so a long session pays
// again, on every turn, for everything it has accumulated.
type ContextSize struct {
	Threshold int `json:"threshold"` // input tokens per turn above which a turn counts

	Turns    int     `json:"turns"`    // turns above the threshold
	Sessions int     `json:"sessions"` // sessions with at least one such turn
	Cost     float64 `json:"cost"`     // input cost of those turns (everything they sent)

	// Saving is the cost of the tokens above the threshold: what those
	// turns would have saved had context been kept to Threshold tokens (by
	// starting a new session per task, or compacting earlier). It is an upper
	// bound: some of that context was needed.
	Saving float64 `json:"saving"`

	Top []SessionStat `json:"top"` // sessions with the largest saving
}

// FindContextSize reports turns whose context exceeds threshold tokens.
func FindContextSize(sessions []event.Session, threshold, topN int) ContextSize {
	f := ContextSize{Threshold: threshold}
	stats := map[string]*SessionStat{}
	for _, s := range sessions {
		for _, t := range s.Turns {
			ctx := t.Context()
			if ctx <= threshold || !t.Priced {
				continue
			}
			// The tokens above the threshold are old history, so price them at
			// what history cost on this turn: a cache read, or a cache write
			// when the cache had to be rebuilt. The turn's average input price
			// would also count writing its new tokens, which a smaller context
			// still pays.
			saving := float64(ctx-threshold) * historyRate(t)

			f.Turns++
			f.Cost += t.Cost.Input()
			f.Saving += saving

			st := stat(stats, s.ID)
			st.Turns++
			st.Cost += t.Cost.Input()
			st.Saving += saving
			st.Peak = max(st.Peak, ctx)
		}
	}
	f.Sessions = len(stats)
	f.Top = top(stats, topN)
	return f
}
