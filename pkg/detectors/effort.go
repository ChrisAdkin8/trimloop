package detectors

import (
	"sort"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// Effort is P14, thinking-budget overspend, in reduced fidelity (ADRs 0028,
// 0042, 0044): spend by the effort level each request asked for, and how
// much less the turns at the highest levels would have cost had they
// thought as much per turn, on average, as the same model did at high. Thinking tokens are
// used where the log records them (they are part of output, at its price);
// else output tokens. Averages, not turn by turn: tokens vary so much
// between turns that clipping each turn at high's mean "saves" a large share
// even among high's own turns. Harder work gets higher effort on purpose, so
// the figure overstates what a lower level would save.
type Effort struct {
	ByLevel []EffortLevel `json:"by_level"` // lowest level first

	// HighMean is the mean thinking tokens of a turn at "high", over the
	// turns that record them (output tokens if most do not); 0 if none.
	HighMean int `json:"high_mean"`
	// Thinking is true when every comparison used thinking tokens.
	Thinking bool `json:"thinking"`
	// Saving is, for "xhigh" and "max" on each model, the level's mean per
	// turn above the same model's "high", times its turns, at its output
	// price. Means are over the turns that record thinking (older Claude
	// Code versions do not), applied to all the level's turns.
	Saving float64 `json:"saving"`
	Turns  int     `json:"turns"` // the "xhigh" and "max" turns
	// Unmatched are xhigh and max turns on a model with fewer than
	// minHighTurns high turns to compare with: left out of Saving.
	Unmatched int `json:"unmatched"`
	// Usual is the level most turns ran at, so advice can say what to come
	// back to: lowering max to high is no saving for someone usually on medium.
	Usual string `json:"usual"`
	// TopModels are the models of the xhigh and max turns, most turns
	// first: effort can be set per model, so the advice names them.
	TopModels []string `json:"top_models"`
}

// EffortLevel is one effort level's turns and spend.
type EffortLevel struct {
	Level   string  `json:"level"` // "" when the log does not say
	Turns   int     `json:"turns"`
	Output  int     `json:"output"`         // output tokens
	Think   int     `json:"thinking"`       // thinking tokens, where recorded (part of output)
	ThinkN  int     `json:"thinking_turns"` // turns that record them
	Cost    float64 `json:"cost"`           // all of those turns' spend
	OutCost float64 `json:"output_cost"`    // their output spend
}

// minHighTurns and minHighSessions are the fewest high-effort turns, and
// sessions they come from, on a model to compare its xhigh and max turns
// with: fewer make a baseline that one turn or one session can swing.
const (
	minHighTurns    = 30
	minHighSessions = 3
)

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
			if r := t.Usage.Reasoning; r != nil {
				g.Think += *r
				g.ThinkN++
			}
		}
	}
	// Compare within each model: models think different amounts at the same
	// level, and one model's xhigh against another's high is not effort.
	type key struct{ model, level string }
	bm := map[key]*EffortLevel{}
	highSessions := map[string]map[string]bool{} // model -> sessions with high-effort turns
	for _, s := range sessions {
		for _, t := range s.Turns {
			if !t.Priced {
				continue
			}
			k := key{t.Model, t.Effort}
			g := bm[k]
			if g == nil {
				g = &EffortLevel{Level: t.Effort}
				bm[k] = g
			}
			g.Turns++
			g.Output += t.Usage.Output
			g.OutCost += t.Cost.Output
			if r := t.Usage.Reasoning; r != nil {
				g.Think += *r
				g.ThinkN++
			}
			if t.Effort == "high" {
				if highSessions[t.Model] == nil {
					highSessions[t.Model] = map[string]bool{}
				}
				highSessions[t.Model][s.ID] = true
			}
		}
	}
	recorded := func(g *EffortLevel) bool { return g.ThinkN*2 > g.Turns }
	allThinking, compared := true, 0
	keys := make([]key, 0, len(bm))
	for k := range bm {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { // a fixed order keeps the sum the same on every run
		return keys[i].model < keys[j].model || keys[i].model == keys[j].model && keys[i].level < keys[j].level
	})
	for _, k := range keys {
		g := bm[k]
		if k.level != "xhigh" && k.level != "max" || g.Turns == 0 || g.Output == 0 {
			continue
		}
		f.Turns += g.Turns
		h := bm[key{k.model, "high"}]
		if h == nil || h.Turns < minHighTurns || len(highSessions[k.model]) < minHighSessions {
			f.Unmatched += g.Turns // too few high turns on this model to compare with
			continue
		}
		compared++
		// Mean thinking per turn over the turns that record it, where both
		// levels record it on most turns; else mean output.
		var extra float64
		if recorded(g) && recorded(h) {
			extra = float64(g.Think)/float64(g.ThinkN) - float64(h.Think)/float64(h.ThinkN)
		} else {
			allThinking = false
			extra = float64(g.Output)/float64(g.Turns) - float64(h.Output)/float64(h.Turns)
		}
		if extra > 0 {
			f.Saving += extra * float64(g.Turns) * g.OutCost / float64(g.Output)
		}
	}
	f.Thinking = compared > 0 && allThinking
	topTurns := map[string]int{}
	for k, g := range bm {
		if k.level == "xhigh" || k.level == "max" {
			topTurns[k.model] += g.Turns
		}
	}
	for m := range topTurns {
		f.TopModels = append(f.TopModels, m)
	}
	sort.Slice(f.TopModels, func(i, j int) bool {
		a, b := f.TopModels[i], f.TopModels[j]
		return topTurns[a] > topTurns[b] || topTurns[a] == topTurns[b] && a < b
	})
	most := 0
	for level, g := range by {
		// Ties go to the lower level, then (for levels this code does not
		// know) to the name, so every run picks the same one.
		lower := effortOrder[level] < effortOrder[f.Usual] ||
			effortOrder[level] == effortOrder[f.Usual] && level < f.Usual
		if level != "" && (g.Turns > most || g.Turns == most && lower) {
			f.Usual, most = level, g.Turns
		}
	}
	if h := by["high"]; h != nil && h.Turns > 0 {
		if recorded(h) {
			f.HighMean = h.Think / h.ThinkN
		} else {
			f.HighMean = h.Output / h.Turns
		}
	}

	for _, g := range by {
		f.ByLevel = append(f.ByLevel, *g)
	}
	sortBy(f.ByLevel, func(g EffortLevel) (float64, string) { return float64(-effortOrder[g.Level]), g.Level })
	return f
}
