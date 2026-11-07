package detectors

import "github.com/ChrisAdkin8/trimloop/pkg/event"

// ContextSize is the finding for turns that send a very large context. Every
// turn re-sends the whole conversation, so a long session pays again, on every
// turn, for everything it has accumulated.
type ContextSize struct {
	Threshold int // input tokens per turn above which a turn counts

	Turns    int     // turns above the threshold
	Sessions int     // sessions with at least one such turn
	Cost     float64 // input cost of those turns (everything they sent)

	// Saving is the input cost of the tokens above the threshold: what those
	// turns would have saved had context been kept to Threshold tokens (by
	// starting a new session per task, or compacting earlier). It is an upper
	// bound: some of that context was needed.
	Saving float64

	Top []SessionStat // sessions with the largest saving
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
			// Price the excess at this turn's average input price per token.
			saving := t.Cost.Input() * float64(ctx-threshold) / float64(ctx)

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
