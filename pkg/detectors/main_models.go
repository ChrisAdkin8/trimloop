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
		if s.Kind == event.SubAgentSession {
			continue
		}
		for _, t := range s.Turns {
			if !t.Priced || t.Model == "" {
				continue
			}
			m := modelGroup(t.Model)
			g, ok := by[m]
			if !ok {
				g = &ModelSpend{Model: m}
				g.Newer, _ = price.Successor(m)
				g.Tier, _ = price.LowerTier(m)
				by[m] = g
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

// modelGroup is the name turns on one model are grouped under for display
// and suggestions, so that one model is one row and one name. A provider or
// dated id ("claude-opus-5@20260101", "us.anthropic.claude-fable-5-1-v1:0")
// is grouped under the catalogue key it resolves to (price.Key), and a dated
// key the catalogue also holds without its date ("claude-haiku-4-5-20251001")
// under the dateless key, which has the same price. An id the catalogue does
// not hold is kept as written.
func modelGroup(model string) string {
	k, ok := price.Key(model)
	if !ok {
		return model
	}
	if i := len(k) - 9; i > 0 && k[i] == '-' && allDigits(k[i+1:]) {
		if d, ok := price.Key(k[:i]); ok && d == k[:i] {
			return d
		}
	}
	return k
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
