package detectors

import (
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// Reconciliation compares the costs worked out from the logs with the
// harness's own records (ADR 0028). The harness also pays for requests that
// never appear in its logs, such as session titles, summaries after a break
// and web searches; the gap between the two is that unobserved spend.
type Reconciliation struct {
	Sessions int     `json:"sessions"` // main sessions with a complete harness record
	Harness  float64 `json:"harness"`  // what the harness recorded for them
	Ours     float64 `json:"ours"`     // what the logs show for them
	Gap      float64 `json:"gap"`      // Harness minus Ours: spend the logs do not show

	// Behind counts sessions that kept working after their record was last
	// written (still running, or ended without writing it), with what the
	// logs show for them. They are left out of the comparison.
	Behind     int     `json:"behind"`
	BehindCost float64 `json:"behind_cost"`

	// Unrecorded counts main sessions with no harness record (older
	// versions, or runs that ended early), and what the logs show for them.
	Unrecorded     int     `json:"unrecorded"`
	UnrecordedCost float64 `json:"unrecorded_cost"`

	WebSearches   int        `json:"web_searches"`    // in the compared sessions
	WebSearchCost float64    `json:"web_search_cost"` // at the catalogue's per-request price
	ByModel       []ModelGap `json:"by_model"`        // largest gap first
}

// ModelGap is the comparison for one model.
type ModelGap struct {
	Model   string  `json:"model"`
	Harness float64 `json:"harness"`
	Ours    float64 `json:"ours"`
	Gap     float64 `json:"gap"`

	// Uncached input tokens: background requests mostly show here, since
	// they are not cached.
	HarnessUncached int `json:"harness_uncached"`
	OursUncached    int `json:"ours_uncached"`
}

// staleAfter is how long a session may go on after its record was last
// written before the record counts as out of date.
const staleAfter = 10 * time.Minute

// FindReconciliation compares each main session's harness record with the
// cost of the session and its subagents as the logs show it.
func FindReconciliation(sessions []event.Session) Reconciliation {
	var f Reconciliation
	ours := map[string]float64{}
	last := map[string]time.Time{} // latest turn of each conversation
	oursModel := map[string]map[string]*ModelGap{}
	for _, s := range sessions {
		id := s.Conversation()
		if oursModel[id] == nil {
			oursModel[id] = map[string]*ModelGap{}
		}
		if e := s.End(); e.After(last[id]) {
			last[id] = e
		}
		for _, t := range s.Turns {
			ours[id] += t.Cost.Total()
			g := oursModel[id][t.Model]
			if g == nil {
				g = &ModelGap{Model: t.Model}
				oursModel[id][t.Model] = g
			}
			g.Ours += t.Cost.Total()
			g.OursUncached += t.Usage.InputUncached
		}
	}

	byModel := map[string]*ModelGap{}
	for _, s := range sessions {
		if s.Kind != "main" {
			continue
		}
		mine := ours[s.ID]
		if s.Harness == nil {
			f.Unrecorded++
			f.UnrecordedCost += mine
			continue
		}
		if at := s.Harness.At; !at.IsZero() && last[s.ID].Sub(at) > staleAfter {
			f.Behind++
			f.BehindCost += mine
			continue
		}
		f.Sessions++
		f.Harness += s.Harness.Cost
		f.Ours += mine
		for model, r := range s.Harness.ByModel {
			g := gapFor(byModel, model)
			g.Harness += r.Cost
			g.HarnessUncached += r.InputUncached
			f.WebSearches += r.WebSearches
		}
		for model, o := range oursModel[s.ID] {
			g := gapFor(byModel, model)
			g.Ours += o.Ours
			g.OursUncached += o.OursUncached
		}
	}
	f.Gap = f.Harness - f.Ours
	f.WebSearchCost = float64(f.WebSearches) * price.WebSearch
	for _, g := range byModel {
		g.Gap = g.Harness - g.Ours
		if g.Harness > 0 || g.Ours > 0 {
			f.ByModel = append(f.ByModel, *g)
		}
	}
	sortBy(f.ByModel, func(g ModelGap) (float64, string) { return g.Gap, g.Model })
	return f
}

func gapFor(m map[string]*ModelGap, model string) *ModelGap {
	g, ok := m[model]
	if !ok {
		g = &ModelGap{Model: model}
		m[model] = g
	}
	return g
}
