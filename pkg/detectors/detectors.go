// Package detectors finds waste patterns in sessions.
//
// The detectors work from usage, timestamps, tool results and the harness's
// own records (ADRs 0027, 0028). They are plain functions over finished
// sessions, not yet the incremental Detector interface in docs/analyzer.md
// §4.
package detectors

import (
	"sort"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// SessionStat is one session's share of a finding.
type SessionStat struct {
	SessionID string  `json:"session_id"`
	Turns     int     `json:"turns"`  // turns in the session that the finding applies to
	Peak      int     `json:"peak"`   // largest context (input tokens) among those turns
	Cost      float64 `json:"cost"`   // what those turns cost, for the part the finding covers
	Saving    float64 `json:"saving"` // estimated saving, an upper bound
}

// top returns the n sessions with the largest saving, largest first.
func top(stats map[string]*SessionStat, n int) []SessionStat {
	out := make([]SessionStat, 0, len(stats))
	for _, s := range stats {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Saving != out[j].Saving {
			return out[i].Saving > out[j].Saving
		}
		return out[i].SessionID < out[j].SessionID // stable order for ties
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// stat returns the entry for id, creating it if needed.
func stat(stats map[string]*SessionStat, id string) *SessionStat {
	s, ok := stats[id]
	if !ok {
		s = &SessionStat{SessionID: id}
		stats[id] = s
	}
	return s
}

// TotalCost is the cost of every priced turn in sessions.
func TotalCost(sessions []event.Session) event.Cost {
	var c event.Cost
	for _, s := range sessions {
		for _, t := range s.Turns {
			c = c.Add(t.Cost)
		}
	}
	return c
}

// sortBy sorts items by key, largest value first; ties go by name, so the
// order is the same every run.
func sortBy[T any](items []T, key func(T) (value float64, name string)) {
	sort.Slice(items, func(i, j int) bool {
		a, an := key(items[i])
		b, bn := key(items[j])
		if a != b {
			return a > b
		}
		return an < bn
	})
}
