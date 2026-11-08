package detectors

import (
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// MainModels is P13 for main sessions (ADR 0042): spend by model, and what
// the same turns would have cost on a newer model in the same line
// (price.Successor: same tokenizer, no dearer) or one tier down for routine
// work (price.LowerTier: a trade-off, untested for quality). Sub-agents are
// left out: P13's sub-agent finding covers them.
type MainModels struct {
	ByModel []ModelSpend `json:"by_model"` // largest spend first

	Newer      float64 `json:"newer"`       // saving on the newer models
	LowerTier  float64 `json:"lower_tier"`  // saving one tier down
	NewerTurns int     `json:"newer_turns"` // turns on a model with a newer one
	TierTurns  int     `json:"tier_turns"`  // turns on a model with a tier down
}

// ModelSpend is one model's main-session spend and what moving would save.
type ModelSpend struct {
	Model     string  `json:"model"`
	Turns     int     `json:"turns"`
	Cost      float64 `json:"cost"`
	Newer     string  `json:"newer,omitempty"` // the newer model, if any
	NewerSave float64 `json:"newer_save,omitempty"`
	Tier      string  `json:"tier,omitempty"` // the model one tier down, if any
	TierSave  float64 `json:"tier_save,omitempty"`
}

// FindMainModels reports main-session spend by model and the two savings.
func FindMainModels(sessions []event.Session) MainModels {
	var f MainModels
	by := map[string]*ModelSpend{}
	for _, s := range sessions {
		if s.Kind == "subagent" {
			continue
		}
		for _, t := range s.Turns {
			if !t.Priced || t.Model == "" {
				continue
			}
			g, ok := by[t.Model]
			if !ok {
				g = &ModelSpend{Model: t.Model}
				g.Newer, _ = price.Successor(t.Model)
				g.Tier, _ = price.LowerTier(t.Model)
				by[t.Model] = g
			}
			g.Turns++
			g.Cost += t.Cost.Total()
			opts := price.Options{Fast: t.Fast, USOnly: t.USOnly}
			if g.Newer != "" {
				if c, ok := price.Cost(g.Newer, t.Usage, opts); ok && c.Total() < t.Cost.Total() {
					g.NewerSave += t.Cost.Total() - c.Total()
					f.Newer += t.Cost.Total() - c.Total()
					f.NewerTurns++
				}
			}
			if g.Tier != "" {
				if c, ok := price.Cost(g.Tier, t.Usage, opts); ok && c.Total() < t.Cost.Total() {
					g.TierSave += t.Cost.Total() - c.Total()
					f.LowerTier += t.Cost.Total() - c.Total()
					f.TierTurns++
				}
			}
		}
	}
	f.ByModel = make([]ModelSpend, 0, len(by))
	for _, g := range by {
		f.ByModel = append(f.ByModel, *g)
	}
	sortBy(f.ByModel, func(g ModelSpend) (float64, string) { return g.Cost, g.Model })
	return f
}
