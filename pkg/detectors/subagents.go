package detectors

import (
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// SubAgents is P13, model over-provisioning, in reduced fidelity for
// sub-agents (ADR 0031): sub-agents that run on an expensive model, and what
// the same turns would cost on a cheaper one. Whether the cheaper model does
// the work as well is not known from the logs, so Saving is untested for
// quality.
type SubAgents struct {
	Runs  int     `json:"runs"`  // sub-agent sessions
	Turns int     `json:"turns"` // their turns
	Cost  float64 `json:"cost"`  // their spend

	// Saving is what their turns would have cost less on the cheaper model
	// price.Cheaper suggests, counting only turns that have one.
	Saving float64 `json:"saving"`

	// Costly is their spend on a model that has a cheaper choice, in agent
	// types that may move to it.
	Costly float64 `json:"costly"`

	// Cache use, sub-agents against main sessions: cache writes as a share of
	// spend, and how many times each written token was read again.
	WriteShare        float64 `json:"write_share"`
	MainWriteShare    float64 `json:"main_write_share"`
	ReadsPerWrite     float64 `json:"reads_per_write"`
	MainReadsPerWrite float64 `json:"main_reads_per_write"`

	LargeAbove int     `json:"large_above"` // context tokens above which a run counts as large
	Large      int     `json:"large"`       // runs whose context passed LargeAbove
	LargeCost  float64 `json:"large_cost"`  // what those runs cost

	ByType []AgentGroup `json:"by_type"` // by agent type, largest spend first
}

// CostlyShare is the share of sub-agent spend, 0 to 1, on a model that has a
// cheaper choice.
func (f SubAgents) CostlyShare() float64 {
	if f.Cost <= 0 {
		return 0
	}
	return f.Costly / f.Cost
}

// AgentGroup is the spend of one agent type.
type AgentGroup struct {
	Type        string  `json:"type"`
	Runs        int     `json:"runs"`
	Turns       int     `json:"turns"`
	Cost        float64 `json:"cost"`
	Saving      float64 `json:"saving"`
	Model       string  `json:"model"`       // the model that cost most in this group
	Alternative string  `json:"alternative"` // the cheaper model suggested for it; "" if none
}

// sharesCache lists agent types that continue the main conversation and read
// its cache. On another model they would lose that cache, so no cheaper
// model is suggested for them.
var sharesCache = map[string]bool{"fork": true}

// FindSubAgents reports sub-agent spend by agent type and model, and the
// saving from a cheaper model. largeAbove is the context, in tokens, above
// which a run counts as large.
func FindSubAgents(sessions []event.Session, largeAbove int) SubAgents {
	f := SubAgents{LargeAbove: largeAbove}
	groups := map[string]*AgentGroup{}
	modelCost := map[string]map[string]float64{} // agent type -> model -> cost
	var sub, main cacheUse

	for _, s := range sessions {
		if s.Kind != "subagent" {
			for _, t := range s.Turns {
				main.add(t)
			}
			continue
		}
		typ := s.AgentType
		if typ == "" {
			typ = "unknown"
		}
		g, ok := groups[typ]
		if !ok {
			g = &AgentGroup{Type: typ}
			groups[typ] = g
			modelCost[typ] = map[string]float64{}
		}
		g.Runs++
		f.Runs++

		cost, peak := 0.0, 0
		for _, t := range s.Turns {
			sub.add(t)
			cost += t.Cost.Total()
			peak = max(peak, t.Context())
			g.Turns++
			f.Turns++
			modelCost[typ][t.Model] += t.Cost.Total()
			if _, ok := price.Cheaper(t.Model); ok && !sharesCache[typ] {
				f.Costly += t.Cost.Total()
			}
			if saving := cheaperSaving(t); saving > 0 && !sharesCache[typ] {
				g.Saving += saving
				f.Saving += saving
			}
		}
		g.Cost += cost
		f.Cost += cost
		if peak > largeAbove {
			f.Large++
			f.LargeCost += cost
		}
	}

	f.WriteShare, f.ReadsPerWrite = sub.shares()
	f.MainWriteShare, f.MainReadsPerWrite = main.shares()
	for typ, g := range groups {
		if names := largest(modelCost[typ], 1); len(names) > 0 {
			g.Model = names[0]
			if !sharesCache[typ] {
				g.Alternative, _ = price.Cheaper(g.Model)
			}
		}
	}
	f.ByType = make([]AgentGroup, 0, len(groups))
	for _, g := range groups {
		f.ByType = append(f.ByType, *g)
	}
	sortBy(f.ByType, func(g AgentGroup) (float64, string) { return g.Cost, g.Type })
	return f
}

// cheaperSaving is what turn t would have cost less on the cheaper model, or
// 0 if there is none or it has no price for the turn's options.
func cheaperSaving(t event.Turn) float64 {
	alt, ok := price.Cheaper(t.Model)
	if !ok || !t.Priced {
		return 0
	}
	c, ok := price.Cost(alt, t.Usage, price.Options{Fast: t.Fast, USOnly: t.USOnly})
	if !ok {
		return 0
	}
	return t.Cost.Total() - c.Total()
}

// cacheUse adds up spend, cache-write spend and cache tokens.
type cacheUse struct {
	cost, writeCost   float64
	readTok, writeTok int
}

func (c *cacheUse) add(t event.Turn) {
	c.cost += t.Cost.Total()
	c.writeCost += t.Cost.CacheWrites()
	c.readTok += t.Usage.CacheRead
	c.writeTok += t.Usage.CacheWrite5m + t.Usage.CacheWrite1h
}

// shares returns cache writes as a share of spend (0 to 1) and reads per
// written token.
func (c cacheUse) shares() (writeShare, readsPerWrite float64) {
	if c.cost > 0 {
		writeShare = c.writeCost / c.cost
	}
	if c.writeTok > 0 {
		readsPerWrite = float64(c.readTok) / float64(c.writeTok)
	}
	return writeShare, readsPerWrite
}
