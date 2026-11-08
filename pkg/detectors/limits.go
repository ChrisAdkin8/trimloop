package detectors

import (
	"sort"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// LimitWindow is one usage limit as Claude Code reported it to the status
// line: how much of it was used, and when it resets.
type LimitWindow struct {
	Used     float64 `json:"used"`      // percent of the limit used, 0–100
	ResetsAt int64   `json:"resets_at"` // when the limit resets, Unix seconds
}

// LimitReading is one status-line reading of a subscriber's usage limits
// (ADR 0048). A limit not reported is nil.
type LimitReading struct {
	At       time.Time    `json:"time"`
	FiveHour *LimitWindow `json:"five_hour,omitempty"`
	SevenDay *LimitWindow `json:"seven_day,omitempty"`
}

// LimitRate is how much of one limit list-price spend used, measured
// between readings within the same limit window.
type LimitRate struct {
	Rise    float64 `json:"rise"`    // percentage points the limit rose, summed over windows
	Spend   float64 `json:"spend"`   // list-price spend in the logs over the same spans
	Windows int     `json:"windows"` // limit windows with at least two readings
	// PerDollar is the percentage points of the limit each dollar of
	// list-price spend used; 0 until enough was measured to say.
	PerDollar float64 `json:"per_dollar"`
}

// Limits is what the logs and the status-line readings say about a
// subscriber's usage limits.
type Limits struct {
	Hits    int       `json:"hits"`    // requests refused because a limit was reached
	Session int       `json:"session"` // of which the 5-hour limit
	Weekly  int       `json:"weekly"`  // of which the weekly limit
	Last    time.Time `json:"last,omitempty"`

	Readings int       `json:"readings"` // status-line readings in the period
	FiveHour LimitRate `json:"five_hour"`
	SevenDay LimitRate `json:"seven_day"`
}

// FindLimits counts limit hits in sessions and measures, from readings,
// how much of each limit a dollar of list-price spend used. Within each
// window (readings with the same reset time) it takes the rise from the
// first reading to the last against the spend logged between them; a rate
// is given only once the windows add up to at least minSpend and minRise
// (ADR 0048). Readings outside the sessions' period are ignored.
func FindLimits(sessions []event.Session, readings []LimitReading, minSpend, minRise float64) Limits {
	var l Limits
	type spent struct {
		at   time.Time
		cost float64
	}
	var turns []spent
	var from, to time.Time
	for _, s := range sessions {
		for _, h := range s.LimitHits {
			l.Hits++
			switch h.Limit {
			case "session":
				l.Session++
			case "weekly":
				l.Weekly++
			}
			if h.At.After(l.Last) {
				l.Last = h.At
			}
		}
		for _, t := range s.Turns {
			if t.Time.IsZero() {
				continue
			}
			turns = append(turns, spent{t.Time, t.Cost.Total()})
			if from.IsZero() || t.Time.Before(from) {
				from = t.Time
			}
			if t.Time.After(to) {
				to = t.Time
			}
		}
	}
	sort.Slice(turns, func(i, j int) bool { return turns[i].at.Before(turns[j].at) })
	// spendBetween is the list-price spend of turns in (a, b].
	spendBetween := func(a, b time.Time) float64 {
		i := sort.Search(len(turns), func(i int) bool { return turns[i].at.After(a) })
		var sum float64
		for ; i < len(turns) && !turns[i].at.After(b); i++ {
			sum += turns[i].cost
		}
		return sum
	}

	var inPeriod []LimitReading
	for _, r := range readings {
		if !from.IsZero() && !r.At.Before(from) && !r.At.After(to.Add(time.Hour)) {
			inPeriod = append(inPeriod, r)
		}
	}
	l.Readings = len(inPeriod)
	sort.Slice(inPeriod, func(i, j int) bool { return inPeriod[i].At.Before(inPeriod[j].At) })

	rate := func(window func(LimitReading) *LimitWindow) LimitRate {
		type span struct {
			reset       int64
			first, last LimitReading
			n           int
		}
		var spans []*span
		for _, r := range inPeriod {
			w := window(r)
			if w == nil {
				continue
			}
			var sp *span
			for _, s := range spans {
				if SameWindow(s.reset, w.ResetsAt) {
					sp = s
					break
				}
			}
			if sp == nil {
				sp = &span{reset: w.ResetsAt, first: r}
				spans = append(spans, sp)
			}
			sp.last = r
			sp.n++
		}
		var lr LimitRate
		for _, sp := range spans {
			rise := window(sp.last).Used - window(sp.first).Used
			if sp.n < 2 || rise < 0 {
				continue
			}
			lr.Windows++
			lr.Rise += rise
			lr.Spend += spendBetween(sp.first.At, sp.last.At)
		}
		if lr.Spend >= minSpend && lr.Rise >= minRise {
			lr.PerDollar = lr.Rise / lr.Spend
		}
		return lr
	}
	l.FiveHour = rate(func(r LimitReading) *LimitWindow { return r.FiveHour })
	l.SevenDay = rate(func(r LimitReading) *LimitWindow { return r.SevenDay })
	return l
}

// SameWindow reports whether two reset times name the same limit window:
// Claude Code's reset times for one window can differ by a few seconds.
func SameWindow(a, b int64) bool {
	d := a - b
	return d >= -60 && d <= 60
}
