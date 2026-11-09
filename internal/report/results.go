// Package report runs every finding over a set of sessions and renders the
// results as text, JSON or a self-contained HTML page. All three formats show
// the same Results.
package report

import (
	"fmt"
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
	Prices    string    `json:"price_catalogue"`            // price catalogue version
	Version   string    `json:"trimloop_version,omitempty"` // the build that made the report

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

	CacheLife    detectors.CacheLife    `json:"p02_cache_life"`
	CacheLifeFix *fixes.Recommendation  `json:"p02_fix,omitempty"`
	MainModels   detectors.MainModels   `json:"p13_main_models"`
	ModelFixes   []fixes.Recommendation `json:"p13_main_fixes"`
	Effort       detectors.Effort       `json:"p14_effort"`
	EffortFix    *fixes.Recommendation  `json:"p14_fix,omitempty"`

	// Plan and Limits are the user's plan and usage limits (ADR 0048).
	Plan   Plan             `json:"plan"`
	Limits detectors.Limits `json:"usage_limits"`

	// From the second gap analysis (ADR 0050).
	Switches     detectors.ModelSwitches `json:"p01_model_switches"`
	SwitchFix    *fixes.Recommendation   `json:"p01_fix,omitempty"`
	Failures     detectors.Failures      `json:"p12_failures"`
	FailureFix   *fixes.Recommendation   `json:"p12_fix,omitempty"`
	ToolInputs   detectors.ToolInputs    `json:"tool_inputs"`
	AppServers   []detectors.ContextPart `json:"p07_unused_app_servers"`
	AppServerFix *fixes.Recommendation   `json:"p07_app_server_fix,omitempty"`
	StartCost    float64                 `json:"start_cost"` // median cost of a new main session's first turn
	When         detectors.When          `json:"when"`
	Retention    Retention               `json:"retention"`

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

	// HideIDs is true under --no-titles: the web page then shows no session
	// or connector identifiers either, and the text no connector
	// identifiers (it keeps session ids, to find a session by). Not in the
	// JSON, which is for scripts.
	HideIDs bool `json:"-"`
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
	// Since, if set, is the --since date (YYYY-MM-DD): only main sessions
	// that started on or after it were analysed, with their sub-agents.
	Since string `json:"since,omitempty"`
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
	CompactTo        int            // context after a compaction, when the logs show none
	SummaryTo        int            // tokens a compaction's summary writes, when the logs show none
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
		CompactTo:        80_000,
		SummaryTo:        25_000, // the measured median is 22-29k (docs/log-signals.md section 8)
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
	r.Limits = detectors.FindLimits(sessions, nil, minLimitSpend, minLimitRise) // AddLimits adds readings
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
	// What the first request after a compaction sends, measured; the
	// harness's own "after" figure counts only the summary.
	after, summary := r.Growth.MedianResume, r.Growth.MedianAfter
	if after == 0 {
		after = s.CompactTo
	}
	if summary == 0 {
		summary = s.SummaryTo // not the whole size after: the summary is only part of it
	}
	summary = min(summary, after)
	likely, upTo := detectors.LikelyCompactionSaving(sessions, s.ContextThreshold, after, summary)
	if f, ok := fixes.ForCompaction(r.Growth, s.ContextThreshold, upTo, likely, after, s.MinFixSaving); ok {
		// The likely saving rests on the size after a compaction, measured
		// on few compactions: show what its quartiles give.
		if lo, hi := r.Growth.ResumeLow, r.Growth.ResumeHigh; lo > 0 && hi > lo {
			high, _ := detectors.LikelyCompactionSaving(sessions, s.ContextThreshold, lo, min(summary, lo))
			low, _ := detectors.LikelyCompactionSaving(sessions, s.ContextThreshold, hi, min(summary, hi))
			// The range is capped at "up to", which is the replay with every
			// compaction free: at the low quartile the replay can pass it.
			f.LikelyLow, f.LikelyHigh = min(low, likely), min(max(high, likely), upTo)
			f.LikelyNote += fmt.Sprintf(" The size after a compaction varies (the middle half of %d measured "+
				"runs from %s to %s tokens): at %s the likely saving is %s, at %s it is %s.",
				r.Growth.Resumes, approx(lo), approx(hi), approx(lo), money(high), approx(hi), money(low))
		}
		r.CompactFix = &f
	}
	fresh := detectors.TypicalStart(sessions)
	r.StartCost = detectors.StartCost(sessions)
	if f, ok := fixes.ForPauses(r.Pause, detectors.LikelyPauseSaving(sessions, s.PauseGap, fresh), fresh, r.StartCost, s.MinFixSaving); ok {
		r.PauseFix = &f
	}

	r.CacheLife = detectors.FindCacheLife(sessions, s.TopSessions)
	if f, ok := fixes.ForCacheLife(r.CacheLife, s.MinFixSaving); ok {
		r.CacheLifeFix = &f
	}
	r.MainModels = detectors.FindMainModels(sessions)
	r.ModelFixes = fixes.ForMainModels(r.MainModels, s.MinFixSaving)
	r.Actions = append(r.Actions, r.ModelFixes...)
	r.Effort = detectors.FindEffort(sessions)
	r.Effort.ThinkingCost, r.Effort.OutputCost = detectors.ThinkingCost(sessions)
	if f, ok := fixes.ForEffort(r.Effort, s.MinFixSaving); ok {
		r.EffortFix = &f
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

	r.Switches = detectors.FindModelSwitches(sessions, s.TopSessions)
	if f, ok := fixes.ForModelSwitches(r.Switches, s.MinFixSaving); ok {
		r.SwitchFix = &f
	}
	r.Failures = detectors.FindFailures(sessions)
	if f, ok := fixes.ForFailures(r.Failures, s.MinFixSaving); ok {
		r.FailureFix = &f
	}
	r.ToolInputs = detectors.FindToolInputs(sessions, s.TopGroups)
	r.AppServers = detectors.UnusedAppServers(r.Standing, sessions)
	if f, ok := fixes.ForAppServers(r.AppServers, s.MinFixSaving); ok {
		r.AppServerFix = &f
	}
	r.When = detectors.FindWhen(sessions, loc)

	for _, f := range []*fixes.Recommendation{r.CompactFix, r.PauseFix, r.CacheLifeFix, r.EffortFix,
		r.SwitchFix, r.FailureFix, r.AppServerFix} {
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
	r.HideIDs = !s.Titles
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
	for _, s := range r.CacheLife.Top {
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
		if s.Kind == event.SubAgentSession {
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

// Retention is how long Claude Code keeps session logs (ADR 0050): its
// cleanupPeriodDays setting, or the default when it is not set.
type Retention struct {
	Days int  `json:"days"`
	Set  bool `json:"set"`
}

// ShortRetention reports whether logs are kept for under 90 days, which
// limits what a report can see.
func (r Results) ShortRetention() bool { return RetentionTip(r.Retention) != "" }

// RetentionTip is the tip to show when Claude Code keeps logs for under 90
// days, or "" (the report and setup --check both use it). Claude Code
// deletes a log once it has gone unused that long, so long-running sessions
// stay in reports longer than that.
func RetentionTip(r Retention) string {
	switch {
	case r.Days >= 90:
		return ""
	case r.Set && r.Days <= 0:
		return "cleanupPeriodDays is 0, so Claude Code may keep no session logs at all. " +
			`Set "cleanupPeriodDays": 365 in Claude Code's settings to keep a year.`
	case r.Days <= 0:
		return ""
	}
	how := "by default"
	if r.Set {
		how = "as cleanupPeriodDays is set"
	}
	return fmt.Sprintf("Claude Code deletes session logs once they have gone unused for %d days (%s), so older work "+
		`drops out of reports. Set "cleanupPeriodDays": 365 in Claude Code's settings to keep a year.`, r.Days, how)
}
