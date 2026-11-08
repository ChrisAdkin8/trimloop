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

	// Actions are every recommendation, largest expected saving first.
	// Savings can overlap (see each one's overlap), so they are not added up.
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
	PauseFix    *fixes.Recommendation  `json:"p09_fix,omitempty"`

	Reconciliation detectors.Reconciliation  `json:"reconciliation"`
	Standing       detectors.StandingContext `json:"p07_standing_context"`
	StandingFixes  []fixes.Recommendation    `json:"p07_fixes"`

	// Weeks are the figures week by week, oldest first, including weeks
	// kept from earlier runs (see package history).
	Weeks []detectors.Week `json:"weeks"`

	Period Period `json:"period"`

	// Labels name the sessions the tables show: title, project and date.
	// Titles summarise conversations, so they are left out when
	// Settings.Titles is false.
	Labels map[string]Label `json:"labels"`
}

// Period is the time the logs cover.
type Period struct {
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
	Days    int       `json:"days"`
	PerWeek float64   `json:"per_week"` // spend per week over the period; 0 if under a week
}

// Label names a session for people.
type Label struct {
	Title    string    `json:"title,omitempty"`
	Project  string    `json:"project,omitempty"`
	Start    time.Time `json:"start"`
	SubAgent bool      `json:"sub_agent,omitempty"` // a sub-agent of the titled session
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
	ContextThreshold int            // P19: input tokens per turn
	PauseGap         time.Duration  // P09: longer than the longest cache lifetime
	UncompactedLimit int            // sessions past this many tokens that never compacted
	ResultTarget     int            // P04: tokens per tool result; the excess is saving
	TopSessions      int            // sessions listed per finding
	TopGroups        int            // tools, shapes, projects listed
	TopFixes         int            // kinds of tool noise with fixes
	MinFixSaving     float64        // dollars; smaller savings get no recommendation
	CompactTo        int            // tokens a compaction is assumed to leave, when the logs have none
	Titles           bool           // show session titles, which summarise conversations
	Location         *time.Location // for week boundaries
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
		CompactTo:        30_000,
		Titles:           true,
		Location:         time.Local,
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
	after := r.Growth.MedianAfter
	if after == 0 {
		after = s.CompactTo
	}
	likely, upTo := detectors.LikelyCompactionSaving(sessions, s.ContextThreshold, after)
	if f, ok := fixes.ForCompaction(r.Growth, s.ContextThreshold, upTo, likely, after, s.MinFixSaving); ok {
		r.CompactFix = &f
	}
	fresh := detectors.TypicalStart(sessions)
	if f, ok := fixes.ForPauses(r.Pause, detectors.LikelyPauseSaving(sessions, s.PauseGap, fresh), fresh, s.MinFixSaving); ok {
		r.PauseFix = &f
	}

	r.Reconciliation = detectors.FindReconciliation(sessions)
	r.Standing = detectors.FindStandingContext(sessions)
	r.StandingFixes = fixes.ForStandingContext(r.Standing, s.MinFixSaving)
	r.Actions = append(r.Actions, r.StandingFixes...)
	loc := s.Location
	if loc == nil {
		loc = time.UTC
	}
	r.Weeks = detectors.FindWeekly(sessions, s.ContextThreshold, s.PauseGap, loc)

	for _, f := range []*fixes.Recommendation{r.CompactFix, r.PauseFix} {
		if f != nil {
			r.Actions = append(r.Actions, *f)
		}
	}
	if r.SubAgentFix != nil {
		r.Actions = append(r.Actions, *r.SubAgentFix)
	}
	r.Actions = append(r.Actions, r.ToolFixes...)
	sort.SliceStable(r.Actions, func(i, j int) bool { return r.Actions[i].Expected() > r.Actions[j].Expected() })
	r.Period = period(sessions, r.Total)
	r.Labels = labels(sessions, r, s.Titles)
	return r
}

// period works out the time the sessions cover and the spend per week.
func period(sessions []event.Session, total float64) Period {
	var p Period
	for _, s := range sessions {
		if t := s.Start(); !t.IsZero() && (p.From.IsZero() || t.Before(p.From)) {
			p.From = t
		}
		if t := s.End(); t.After(p.To) {
			p.To = t
		}
	}
	if p.From.IsZero() {
		return p
	}
	p.Days = int(p.To.Sub(p.From).Hours()/24) + 1
	if p.Days >= 7 {
		p.PerWeek = total / (float64(p.Days) / 7)
	}
	return p
}

// labels names every session the report's tables show. A sub-agent is named
// after the session that started it.
func labels(sessions []event.Session, r Results, titles bool) map[string]Label {
	want := map[string]bool{}
	for _, s := range r.Context.Top {
		want[s.SessionID] = true
	}
	for _, s := range r.Pause.Top {
		want[s.SessionID] = true
	}
	for _, c := range r.Growth.Top {
		want[c.ID] = true
	}
	byID := map[string]event.Session{}
	child := map[string]event.Session{} // a conversation's first sub-agent, for one whose main log is gone
	for _, s := range sessions {
		byID[s.ID] = s
		if c, ok := child[s.ParentID]; s.ParentID != "" && (!ok || s.Start().Before(c.Start())) {
			child[s.ParentID] = s
		}
	}
	out := map[string]Label{}
	for id := range want {
		s, ok := byID[id]
		if !ok {
			// Only the conversation's sub-agents are left: name it by the
			// first of them, with no title.
			if c, ok := child[id]; ok {
				out[id] = Label{Project: c.Project, Start: c.Start()}
			}
			continue
		}
		l := Label{Project: s.Project, Start: s.Start()}
		named := s
		if s.Kind == "subagent" {
			l.SubAgent = true
			if p, ok := byID[s.ParentID]; ok {
				named = p
			}
		}
		if titles {
			l.Title = named.Title
		}
		out[id] = l
	}
	return out
}
