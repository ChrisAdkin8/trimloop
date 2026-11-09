package detectors

import (
	"sort"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// When is when the spend happened, in local time (ADR 0050): by hour of the
// day, by day of the week, and how many main sessions ran at once.
type When struct {
	ByHour        [24]float64 `json:"by_hour"`    // spend by hour of the day, 0–23
	ByWeekday     [7]float64  `json:"by_weekday"` // spend by day, Monday first
	WeekendShare  float64     `json:"weekend_share"`
	MaxConcurrent int         `json:"max_concurrent"` // most main sessions running at once
}

// FindWhen sorts each priced turn's cost into its hour and day, and counts
// the most main sessions whose first and last turns overlap.
func FindWhen(sessions []event.Session, loc *time.Location) When {
	var w When
	total := 0.0
	type edge struct {
		at    time.Time
		start int // +1 at a start, -1 at an end
	}
	var edges []edge
	for _, s := range sessions {
		for _, t := range s.Turns {
			if t.Time.IsZero() || !t.Priced {
				continue
			}
			lt := t.Time.In(loc)
			c := t.Cost.Total()
			w.ByHour[lt.Hour()] += c
			w.ByWeekday[(int(lt.Weekday())+6)%7] += c
			total += c
		}
		if s.Kind == event.MainSession {
			if a, b := s.Start(), s.End(); !a.IsZero() && b.After(a) {
				edges = append(edges, edge{a, 1}, edge{b, -1})
			}
		}
	}
	if total > 0 {
		w.WeekendShare = (w.ByWeekday[5] + w.ByWeekday[6]) / total
	}
	// Ends before starts at the same moment: back-to-back is not at once.
	sort.Slice(edges, func(i, j int) bool {
		if !edges[i].at.Equal(edges[j].at) {
			return edges[i].at.Before(edges[j].at)
		}
		return edges[i].start < edges[j].start
	})
	n := 0
	for _, e := range edges {
		n += e.start
		w.MaxConcurrent = max(w.MaxConcurrent, n)
	}
	return w
}

// ThinkingCost is the output cost of thinking: each turn's output cost times
// its thinking share of output, where the harness logged thinking tokens.
// It returns the thinking cost and the output cost of all turns.
func ThinkingCost(sessions []event.Session) (thinking, output float64) {
	for _, s := range sessions {
		for _, t := range s.Turns {
			output += t.Cost.Output
			if r := t.Usage.Reasoning; r != nil && t.Usage.Output > 0 {
				thinking += t.Cost.Output * float64(min(*r, t.Usage.Output)) / float64(t.Usage.Output)
			}
		}
	}
	return thinking, output
}

// StartCost is the median cost of a main session's first turn: what
// starting a new session costs (ADR 0050). It returns 0 if there are none.
func StartCost(sessions []event.Session) float64 {
	var costs []float64
	for _, s := range sessions {
		if s.Kind == event.MainSession && len(s.Turns) > 0 && s.Turns[0].Priced {
			costs = append(costs, s.Turns[0].Cost.Total())
		}
	}
	return median(costs)
}
