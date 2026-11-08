package detectors

import (
	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// Effort is P14, thinking-budget overspend, in reduced fidelity (ADRs 0028,
// 0042): spend by the effort level each request asked for, and how much
// less output the turns at the highest levels would have cost at high's
// mean output per turn. An upper bound: harder work gets higher effort on
// purpose, and output includes the answer as well as the thinking.
type Effort struct {
	ByLevel []EffortLevel `json:"by_level"` // lowest level first

	// HighMean is the mean output tokens of a turn at "high"; 0 if none.
	HighMean int `json:"high_mean"`
	// Saving is the output cost above HighMean on "xhigh" and "max" turns.
	Saving float64 `json:"saving"`
	Turns  int     `json:"turns"` // the "xhigh" and "max" turns
}

// EffortLevel is one effort level's turns and spend.
type EffortLevel struct {
	Level   string  `json:"level"` // "" when the log does not say
	Turns   int     `json:"turns"`
	Output  int     `json:"output"`      // output tokens
	Cost    float64 `json:"cost"`        // all of those turns' spend
	OutCost float64 `json:"output_cost"` // their output spend
}

var effortOrder = map[string]int{"": 0, "low": 1, "medium": 2, "high": 3, "xhigh": 4, "max": 5}

// FindEffort reports spend by effort level and the P14 upper bound.
func FindEffort(sessions []event.Session) Effort {
	var f Effort
	by := map[string]*EffortLevel{}
	for _, s := range sessions {
		for _, t := range s.Turns {
			if !t.Priced {
				continue
			}
			g, ok := by[t.Effort]
			if !ok {
				g = &EffortLevel{Level: t.Effort}
				by[t.Effort] = g
			}
			g.Turns++
			g.Output += t.Usage.Output
			g.Cost += t.Cost.Total()
			g.OutCost += t.Cost.Output
		}
	}
	if h := by["high"]; h != nil && h.Turns > 0 {
		f.HighMean = h.Output / h.Turns
		for _, s := range sessions {
			for _, t := range s.Turns {
				if !t.Priced || t.Effort != "xhigh" && t.Effort != "max" {
					continue
				}
				f.Turns++
				if extra := t.Usage.Output - f.HighMean; extra > 0 && t.Usage.Output > 0 {
					f.Saving += t.Cost.Output * float64(extra) / float64(t.Usage.Output)
				}
			}
		}
	}
	for _, g := range by {
		f.ByLevel = append(f.ByLevel, *g)
	}
	sortBy(f.ByLevel, func(g EffortLevel) (float64, string) { return float64(-effortOrder[g.Level]), g.Level })
	return f
}
