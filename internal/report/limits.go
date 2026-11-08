package report

import (
	"fmt"
	"io"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/fixes"
)

// Plan is the user's Claude plan, as Claude Code's account file says.
type Plan struct {
	Name         string `json:"name,omitempty"` // "Max 5×"; "" if not known
	Subscription bool   `json:"subscription"`   // a flat fee, not paid per token
}

// The least a weekly-limit rate is worked out from (ADR 0048): list-price
// spend, and percentage points the limit rose, over all windows measured.
const (
	minLimitSpend = 20.0
	minLimitRise  = 5.0
)

// AddLimits adds the plan and the status-line readings of usage limits to
// r, and gives each action its saving as a share of a week's limit when
// that can be measured (ADR 0048).
func AddLimits(r *Results, sessions []event.Session, plan Plan, readings []detectors.LimitReading) {
	r.Plan = plan
	r.Limits = detectors.FindLimits(sessions, readings, minLimitSpend, minLimitRise)
	rate := r.Limits.SevenDay.PerDollar
	weeks := max(float64(r.Period.Days)/7, 1)
	for i := range r.Actions {
		a := &r.Actions[i]
		a.WeeklyLimit = 0
		if rate > 0 {
			saving := a.Likely
			if saving == 0 {
				saving = a.Saving
			}
			a.WeeklyLimit = min(saving/weeks*rate, 100)
		}
	}
}

// Subscriber reports whether the user pays a flat fee with usage limits:
// the account file says so, or Claude Code reported limits.
func (r Results) Subscriber() bool {
	return r.Plan.Subscription || r.Limits.Hits > 0 || r.Limits.Readings > 0
}

// PlanName is "the Max 5× plan", or "a subscription" if the plan's name is
// not known.
func (r Results) PlanName() string {
	if r.Plan.Name == "" {
		return "a subscription"
	}
	return "the " + r.Plan.Name + " plan"
}

// LimitHitsText says how often a usage limit stopped work.
func (r Results) LimitHitsText() string {
	l := r.Limits
	if l.Hits == 0 {
		return "No usage limit was reached in this period."
	}
	s := fmt.Sprintf("Claude Code stopped %s because a usage limit was reached", count(l.Hits, "time"))
	var parts []string
	if l.Session > 0 {
		parts = append(parts, fmt.Sprintf("%d at the 5-hour limit", l.Session))
	}
	if l.Weekly > 0 {
		parts = append(parts, fmt.Sprintf("%d at the weekly limit", l.Weekly))
	}
	if len(parts) > 0 && (len(parts) > 1 || l.Session+l.Weekly < l.Hits) {
		s += " (" + joinAnd(parts) + ")"
	}
	if !l.Last.IsZero() {
		s += ", most recently on " + l.Last.Local().Format("2 Jan")
	}
	return s + "."
}

// LimitRateText says how much of the weekly limit list-price spend used, or
// how to measure it.
func (r Results) LimitRateText() string {
	l := r.Limits
	if rate := l.SevenDay.PerDollar; rate > 0 {
		return fmt.Sprintf("Each $10 of work at list prices used about %.1f%% of your weekly limit (measured from %s of status-line readings; "+
			"an overestimate if you also use Claude chat, which shares the limit).", 10*rate, count(l.SevenDay.Windows, "week"))
	}
	if l.Readings > 0 {
		return "Not enough status-line readings yet to show savings as a share of your weekly limit."
	}
	return "To see savings as a share of your weekly limit, add trimloop's status line (trimloop setup): " +
		"it keeps the limit readings Claude Code shows it."
}

// limitLines prints, for a subscriber, what the figures mean on a flat fee,
// how often a limit was reached, and what a dollar of work uses.
func limitLines(w io.Writer, r Results) {
	if !r.Subscriber() {
		return
	}
	fmt.Fprintf(w, "You are on %s, a flat fee: these figures are list prices, a measure of how much of\n", r.PlanName())
	fmt.Fprintln(w, "your usage limits work takes, not money you would get back.")
	fmt.Fprintln(w, r.LimitHitsText())
	fmt.Fprintln(w, r.LimitRateText())
}

// limitText is ", about N% of your weekly limit each week" for an action
// with a measured share, else "".
func limitText(a fixes.Recommendation) string {
	if a.WeeklyLimit <= 0 {
		return ""
	}
	if a.WeeklyLimit < 1 {
		return "; under 1% of your weekly limit each week"
	}
	return fmt.Sprintf("; about %.0f%% of your weekly limit each week", a.WeeklyLimit)
}

// joinAnd joins "a", "b" and "c" as "a, b and c".
func joinAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	s := parts[0]
	for _, p := range parts[1 : len(parts)-1] {
		s += ", " + p
	}
	return s + " and " + parts[len(parts)-1]
}
