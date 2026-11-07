// Package report runs every finding over a set of sessions and renders the
// results as text, JSON or a self-contained HTML page. All three formats show
// the same Results.
package report

import (
	"sort"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/fixes"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// FormatVersion identifies the JSON layout. It changes when a field is
// removed or changes meaning; adding a field does not change it.
const FormatVersion = "trimloop-report/1"

// Results is everything a report shows.
type Results struct {
	Format    string    `json:"format"`
	Generated time.Time `json:"generated"`
	Prices    string    `json:"price_catalogue"` // price catalogue version

	Read  Read       `json:"read"`
	Cost  event.Cost `json:"cost"`  // by token class
	Total float64    `json:"total"` // all spend

	// Actions are every recommendation, largest saving first. Savings can
	// overlap (see each one's overlap), so they are not added up.
	Actions []fixes.Recommendation `json:"actions"`

	Attribution detectors.Attribution  `json:"attribution"`
	Context     detectors.ContextSize  `json:"p19_context"`
	Growth      detectors.Growth       `json:"p19_growth"`
	Pause       detectors.LongPause    `json:"p09_pauses"`
	Tools       detectors.ToolOutput   `json:"p04_tool_output"`
	ToolFixes   []fixes.Recommendation `json:"p04_fixes"`
	SubAgents   detectors.SubAgents    `json:"p13_sub_agents"`
	SubAgentFix *fixes.Recommendation  `json:"p13_fix,omitempty"`
	CompactFix  *fixes.Recommendation  `json:"p19_compaction_fix,omitempty"`
}

// Read counts what was read.
type Read struct {
	Logs     int `json:"logs"`
	Failed   int `json:"failed"`    // logs that could not be read to the end
	BadLines int `json:"bad_lines"` // lines that could not be parsed
	Sessions int `json:"sessions"`
	Turns    int `json:"turns"`
	Unpriced int `json:"unpriced"` // turns with no price, left out of costs
}

// Settings are the thresholds the findings use.
type Settings struct {
	ContextThreshold int           // P19: input tokens per turn
	PauseGap         time.Duration // P09: longer than the longest cache lifetime
	UncompactedLimit int           // sessions past this many tokens that never compacted
	ResultTarget     int           // P04: tokens per tool result; the excess is saving
	TopSessions      int           // sessions listed per finding
	TopGroups        int           // tools, shapes, projects listed
	TopFixes         int           // kinds of tool noise with fixes
	MinFixSaving     float64       // dollars; smaller savings get no recommendation
}

// DefaultSettings are the thresholds the report uses unless told otherwise.
func DefaultSettings() Settings {
	return Settings{
		ContextThreshold: 200_000,
		PauseGap:         time.Hour,
		UncompactedLimit: 500_000,
		ResultTarget:     2_000,
		TopSessions:      5,
		TopGroups:        10,
		TopFixes:         4,
		MinFixSaving:     1.0,
	}
}

// Analyse runs every finding over sessions. read gives the log counts; the
// session and turn counts are filled in here.
func Analyse(sessions []event.Session, read Read, s Settings, now time.Time) Results {
	r := Results{Format: FormatVersion, Generated: now.UTC(), Prices: price.Version, Read: read}
	r.Read.Sessions = len(sessions)
	r.Read.Turns, r.Read.Unpriced = 0, 0
	for _, sess := range sessions {
		for _, t := range sess.Turns {
			r.Read.Turns++
			if !t.Priced {
				r.Read.Unpriced++
			}
		}
	}
	r.Cost = detectors.TotalCost(sessions)
	r.Total = r.Cost.Total()

	r.Context = detectors.FindContextSize(sessions, s.ContextThreshold, s.TopSessions)
	r.Pause = detectors.FindLongPauses(sessions, s.PauseGap, s.TopSessions)
	r.Tools = detectors.FindToolOutput(sessions, s.ResultTarget, s.TopGroups)
	r.ToolFixes = fixes.ForToolOutput(r.Tools, s.TopFixes, s.MinFixSaving)
	r.SubAgents = detectors.FindSubAgents(sessions, s.ContextThreshold)
	r.Growth = detectors.FindGrowth(sessions, s.UncompactedLimit, s.TopSessions)
	r.Attribution = detectors.FindAttribution(sessions, s.TopGroups)
	if f, ok := fixes.ForSubAgents(r.SubAgents, s.MinFixSaving); ok {
		r.SubAgentFix = &f
	}
	if f, ok := fixes.ForCompaction(r.Growth, s.ContextThreshold, r.Context.Saving, s.MinFixSaving); ok {
		r.CompactFix = &f
	}

	if r.CompactFix != nil {
		r.Actions = append(r.Actions, *r.CompactFix)
	}
	if r.SubAgentFix != nil {
		r.Actions = append(r.Actions, *r.SubAgentFix)
	}
	r.Actions = append(r.Actions, r.ToolFixes...)
	sort.SliceStable(r.Actions, func(i, j int) bool { return r.Actions[i].Saving > r.Actions[j].Saving })
	return r
}
