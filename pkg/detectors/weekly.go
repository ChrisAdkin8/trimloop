package detectors

import (
	"sort"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// Week is one calendar week's figures, so a change can be compared before
// and after. It holds numbers only, no names, so it can be kept between runs
// (ADR 0033).
type Week struct {
	Week  string  `json:"week"` // the Monday it starts, YYYY-MM-DD, local time
	Spend float64 `json:"spend"`
	Turns int     `json:"turns"`

	LargeContext float64 `json:"large_context"` // input cost of turns over the P19 threshold
	SubAgents    float64 `json:"sub_agents"`    // spend in sub-agents
	PauseRewrite float64 `json:"pause_rewrite"` // cache re-writes after long breaks the user took (P09)
	ToolOutput   float64 `json:"tool_output"`   // carried cost of tool output added that week (P04)
	FileViews    int     `json:"file_views"`
	Rereads      int     `json:"rereads"`
	Compactions  int     `json:"compactions"`

	// Added in analysis version 5, so the P02, P14 and P13 main-session
	// cards can be checked week by week (ADR 0043).
	ShortWaits  float64 `json:"short_waits"`  // what a 5-minute cache expiring after 5-60 minute waits cost (P02)
	TopEffort   float64 `json:"top_effort"`   // output cost of xhigh and max turns (P14)
	OlderModels float64 `json:"older_models"` // main-session spend on a model with a newer, cheaper version or a cheaper tier (P13)

	// Added in analysis version 7 (ADR 0048): requests refused because a
	// subscription usage limit was reached.
	LimitHits int `json:"limit_hits"`

	Prices string `json:"prices,omitempty"` // price catalogue the figures used
	// Analysis is the AnalysisVersion that worked the figures out.
	Analysis int `json:"analysis,omitempty"`
	// Kept is true when the week's figures came from the history file: its
	// logs are gone, or partly gone and the stored copy was fuller (the
	// harness deletes old logs). Shown, not stored.
	Kept bool `json:"kept,omitempty"`
	// KeptWhy says why a kept week's figures came from the history file:
	// "logs deleted", or "logs partly deleted, stored copy fuller". Shown,
	// not stored.
	KeptWhy string `json:"kept_why,omitempty"`
}

// AnalysisVersion changes whenever a fix changes how the weekly figures are
// worked out, so figures kept from an older version can be replaced.
// Version 2 stopped counting made-up replies as turns and copied lines twice;
// version 3 counts only breaks the user took in PauseRewrite; version 4
// prices carried tool output as a write on rebuilt-cache turns, leaves
// sub-agent pauses out of PauseRewrite, counts refusals as the user's and
// compactions after a session's last reply (ADR 0039); version 5 adds the
// short-wait, top-effort and older-model figures (ADR 0043); version 6
// prices short waits against the cache still there (ADR 0044); version 7
// adds limit hits (ADR 0048); version 8 counts a re-read only for the same
// part of an unchanged file (ADR 0053); version 9 follows a cd before a
// shell view to the file it names, and counts every file a view names.
const AnalysisVersion = 9

// The analysis version in which each later weekly figure was first
// measured. A week stored by an earlier version does not have it; reports
// show a dash rather than a zero.
const (
	NewFindingsSince = 5 // ShortWaits, TopEffort, OlderModels (ADR 0043)
	LimitHitsSince   = 7 // LimitHits (ADR 0048)
)

// HasNewFindings reports whether the week measured the P02, P14 and P13
// main-session figures.
func (k Week) HasNewFindings() bool { return k.Analysis >= NewFindingsSince }

// HasLimitHits reports whether the week counted usage-limit hits.
func (k Week) HasLimitHits() bool { return k.Analysis >= LimitHitsSince }

// TurnsCountedSince is the first AnalysisVersion that counts turns as this
// one does, so turn counts from it on can be compared.
const TurnsCountedSince = 2

// WeekOf returns the Monday that starts t's week, as YYYY-MM-DD in t's
// location.
func WeekOf(t time.Time) string {
	d := (int(t.Weekday()) + 6) % 7 // days since Monday
	return t.AddDate(0, 0, -d).Format("2006-01-02")
}

// FindWeekly works out each week's figures from the turns' times, oldest
// week first. Turns with no time are left out.
func FindWeekly(sessions []event.Session, threshold int, pauseGap time.Duration, loc *time.Location) []Week {
	weeks := map[string]*Week{}
	get := func(t time.Time) *Week {
		k := WeekOf(t.In(loc))
		w, ok := weeks[k]
		if !ok {
			w = &Week{Week: k, Analysis: AnalysisVersion}
			weeks[k] = w
		}
		return w
	}
	for _, s := range sessions {
		for _, sw := range shortWaits(s) {
			if t := s.Turns[sw.turn]; !t.Time.IsZero() {
				get(t.Time).ShortWaits += sw.saving
			}
		}
		for _, t := range s.Turns {
			if t.Time.IsZero() {
				continue
			}
			w := get(t.Time)
			if t.Priced && (t.Effort == "xhigh" || t.Effort == "max") {
				w.TopEffort += t.Cost.Output
			}
			if t.Priced && s.Kind != event.SubAgentSession {
				_, newer := price.Successor(t.Model)
				_, tier := price.LowerTier(t.Model)
				if newer || tier {
					w.OlderModels += t.Cost.Total()
				}
			}
			w.Spend += t.Cost.Total()
			w.Turns++
			if t.Context() > threshold && t.Priced {
				w.LargeContext += t.Cost.Input()
			}
			if s.Kind == event.SubAgentSession {
				w.SubAgents += t.Cost.Total()
			}
			if userBreak(s, t, pauseGap) {
				w.PauseRewrite += t.Cost.CacheWrites()
			}
			if t.CompactedBefore {
				w.Compactions++
			}
		}
		for _, h := range s.LimitHits {
			if !h.At.IsZero() {
				get(h.At).LimitHits++
			}
		}
		for _, c := range s.Unreplied {
			at := c.At
			if at.IsZero() && len(s.Turns) > 0 {
				at = s.Turns[len(s.Turns)-1].Time // not known: the last reply's week
			}
			if !at.IsZero() {
				get(at).Compactions++
			}
		}
		for _, cc := range carry(s) {
			if cc.at.IsZero() {
				continue
			}
			w := get(cc.at)
			w.ToolOutput += cc.carried
			if cc.call.FileOp == event.FileView {
				w.FileViews++
			}
			if cc.reread {
				w.Rereads++
			}
		}
	}
	out := make([]Week, 0, len(weeks))
	for _, w := range weeks {
		out = append(out, *w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Week < out[j].Week })
	return out
}
