package detectors

import (
	"sort"
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

	// Behind counts sessions whose record does not cover them: work went on
	// after the record was last written (still running, or ended without
	// writing it), or began before the earliest recorded run. They are left
	// out of the comparison.
	Behind     int     `json:"behind"`
	BehindCost float64 `json:"behind_cost"`

	// Unrecorded counts conversations with no harness record (older
	// versions, runs that ended early, or sub-agents whose main session's
	// log is gone), and what the logs show for them.
	Unrecorded     int     `json:"unrecorded"`
	UnrecordedCost float64 `json:"unrecorded_cost"`

	// Unpriced counts sessions with turns the price catalogue has no price
	// for, or whose record says the harness could not price some usage.
	// One side of the comparison would be incomplete, so they are left out.
	Unpriced int `json:"unpriced"`

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
	last := map[string]time.Time{}  // latest turn of each conversation
	first := map[string]time.Time{} // earliest turn of each conversation
	unpriced := map[string]bool{}   // conversations with a turn that has no price
	oursModel := map[string]map[string]*ModelGap{}
	for _, s := range sessions {
		id := s.Conversation()
		if oursModel[id] == nil {
			oursModel[id] = map[string]*ModelGap{}
		}
		if e := s.End(); e.After(last[id]) {
			last[id] = e
		}
		if b := s.Start(); !b.IsZero() && (first[id].IsZero() || b.Before(first[id])) {
			first[id] = b
		}
		for _, t := range s.Turns {
			if !t.Priced {
				unpriced[id] = true
			}
			ours[id] += t.Cost.Total()
			m := modelGroup(t.Model)
			g := oursModel[id][m]
			if g == nil {
				g = &ModelGap{Model: m}
				oursModel[id][m] = g
			}
			g.Ours += t.Cost.Total()
			g.OursUncached += t.Usage.InputUncached
		}
	}

	byModel := map[string]*ModelGap{}
	for _, s := range sessions {
		if s.Kind != event.MainSession {
			continue
		}
		mine := ours[s.ID]
		if s.Harness == nil {
			f.Unrecorded++
			f.UnrecordedCost += mine
			continue
		}
		if unpriced[s.ID] || s.Harness.UnknownCost {
			f.Unpriced++
			continue
		}
		at, start := s.Harness.At, s.Harness.Start
		if !at.IsZero() && last[s.ID].Sub(at) > staleAfter ||
			!start.IsZero() && !first[s.ID].IsZero() && start.Sub(first[s.ID]) > staleAfter {
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
	// Sub-agents whose main session's log is gone have no record either.
	for _, s := range sessions {
		if s.Kind == event.MainSession {
			delete(ours, s.ID)
		}
	}
	orphans := make([]string, 0, len(ours))
	for id := range ours {
		orphans = append(orphans, id)
	}
	sort.Strings(orphans) // a fixed order keeps the sum the same on every run
	for _, id := range orphans {
		f.Unrecorded++
		f.UnrecordedCost += ours[id]
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
	model = modelGroup(model)
	g, ok := m[model]
	if !ok {
		g = &ModelGap{Model: model}
		m[model] = g
	}
	return g
}
