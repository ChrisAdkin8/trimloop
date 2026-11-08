package report

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/fixes"
)

//go:embed report.html.tmpl
var htmlSource string

var htmlPage = template.Must(template.New("report").Funcs(template.FuncMap{
	"dollars":   money,
	"thousands": thousands,
	"short":     approx,
	"pct":       func(part, whole float64) string { return fmt.Sprintf("%.0f%%", pct(part, whole)) },
	"pct1":      func(part, whole float64) string { return fmt.Sprintf("%.1f%%", pct(part, whole)) },
	"share":     func(x float64) string { return fmt.Sprintf("%.0f%%", 100*x) },
	"times":     func(x float64) string { return fmt.Sprintf("%.0f", x) },
	// width is a bar's width in percent of the largest value, at least 0.5
	// so that small values still show.
	"width": func(v, maxv float64) string {
		if maxv <= 0 {
			return "0"
		}
		return fmt.Sprintf("%.1f", max(0.5, 100*v/maxv))
	},
	"date": func(t time.Time) string { return t.Format("2 January 2006, 15:04 UTC") },
	"day":  func(t time.Time) string { return t.Local().Format("2 Jan 2006") },
	// weekStart turns "2026-11-02" into "2 Nov 2026".
	"weekStart": func(w string) string {
		if t, err := time.Parse("2006-01-02", w); err == nil {
			return t.Format("2 Jan 2006")
		}
		return w
	},
	"server":   serverName,
	"agent":    agentName,
	"model":    modelName,
	"ctxname":  contextName,
	"duration": func(d time.Duration) string { return duration(d.Round(time.Second)) },
	"join":     strings.Join,
	"add":      func(a, b int) int { return a + b },
	"kind":     func(k fixes.Kind) string { return string(k) },
	"w":        func(x float64) string { return fmt.Sprintf("%.1f", max(0, x)) },
	"p0":       func(x float64) string { return fmt.Sprintf("%.0f%%", x) },
	"float":    func(n int) float64 { return float64(n) },
	"neg":      func(x float64) float64 { return -x },
	"sub":      func(a, b float64) float64 { return a - b },
	"div":      func(a, b int) int { return a / max(1, b) },
	"plural":   count,
	// args packs values for a sub-template, which reads them with index.
	"args": func(v ...any) []any { return v },
}).Parse(htmlSource))

// WriteHTML writes r as one self-contained HTML page: styles, charts and
// script are inline, and the page loads nothing from the network, so it
// works offline and the data stays on the machine. html/template escapes
// every name taken from the logs.
func WriteHTML(out io.Writer, r Results) error {
	return htmlPage.Execute(out, newView(r))
}

// view is what the page template sees: the results, plus values worked out
// in Go so the template stays simple.
type view struct {
	Results
	First []fixes.Recommendation // the top actions, shown as cards
	More  []fixes.Recommendation // the rest

	Sessions []sessionRow // P19 and P09's top sessions, each once

	MaxProject, MaxBand, MaxTool, MaxAgent, MaxWeek, MaxPart float64

	ThisWeek string // the week the report was written in, which is not over

	// StandingGap is true when the system prompt and tools were recorded
	// for noticeably fewer than all turns.
	StandingGap bool

	Summary summaryView // the two bars at the top of the page

	// Largest is the kind of token that took the most spend.
	Largest *costPart

	// SubCostly is the share of sub-agent spend on a model that has a
	// cheaper choice, 0 to 1.
	SubCostly float64

	Busiest    detectors.Week // the week with the most spend
	TopProject *detectors.Spend
	TopFive    float64 // share of spend in the five costliest sessions, 0 to 1
}

// costPart is one kind of token and its share of spend.
type costPart struct {
	What  string
	Share float64
}

// sessionRow joins one session's P19 and P09 figures.
type sessionRow struct {
	ID          string
	Peak        int
	ContextSave float64 // P19
	PauseSave   float64 // P09
	Pauses      int
}

// Name returns the label for a session id, with the id as a last resort.
func (v view) Name(id string) Label { return v.Labels[id] }

// Older reports whether a week's figures came from another price list or an
// older analysis.
func (v view) Older(k detectors.Week) bool { return olderFigures(k, v.Prices) }

// Untitled is how a session with no title is shown.
const Untitled = "Untitled session"

func newView(r Results) view {
	v := view{Results: r}
	n := min(3, len(r.Actions))
	v.First, v.More = r.Actions[:n], r.Actions[n:]

	rows := map[string]*sessionRow{}
	var order []string
	row := func(s detectors.SessionStat) *sessionRow {
		x, ok := rows[s.SessionID]
		if !ok {
			x = &sessionRow{ID: s.SessionID}
			rows[s.SessionID] = x
			order = append(order, s.SessionID)
		}
		x.Peak = max(x.Peak, s.Peak)
		return x
	}
	for _, s := range r.Context.Top {
		row(s).ContextSave = s.Saving
	}
	for _, s := range r.Pause.Top {
		x := row(s)
		x.PauseSave, x.Pauses = s.Saving, s.Turns
	}
	for _, id := range order {
		v.Sessions = append(v.Sessions, *rows[id])
	}

	for _, p := range r.Attribution.ByProject {
		v.MaxProject = max(v.MaxProject, p.Cost)
	}
	for _, b := range r.Growth.Bands {
		v.MaxBand = max(v.MaxBand, b.MeanCost())
	}
	for _, g := range r.Tools.ByTool {
		v.MaxTool = max(v.MaxTool, g.Carried)
	}
	for _, g := range r.SubAgents.ByType {
		v.MaxAgent = max(v.MaxAgent, g.Cost)
	}
	for _, w := range r.Weeks {
		v.MaxWeek = max(v.MaxWeek, w.Spend)
	}
	for _, p := range r.Standing.Parts {
		v.MaxPart = max(v.MaxPart, p.Cost)
	}
	v.ThisWeek = detectors.WeekOf(r.Generated.Local())
	v.StandingGap = r.Standing.Turns > 0 && float64(r.Standing.Covered) < 0.99*float64(r.Standing.Turns)
	v.Summary = newSummary(r)
	for _, w := range r.Weeks {
		if w.Spend > v.Busiest.Spend {
			v.Busiest = w
		}
	}
	if r.Total > 0 {
		parts := []costPart{
			{"re-reading the session from the cache, Claude's cheaper saved copy of it", r.Cost.CacheRead},
			{"saving the session to the cache", r.Cost.CacheWrite1h + r.Cost.CacheWrite5m},
			{"Claude's replies", r.Cost.Output},
			{"new input sent without the cache", r.Cost.InputUncached},
		}
		big := parts[0]
		for _, p := range parts[1:] {
			if p.Share > big.Share {
				big = p
			}
		}
		big.Share /= r.Total
		v.Largest = &big
	}
	v.SubCostly = r.SubAgents.CostlyShare()
	if len(r.Attribution.ByProject) > 0 {
		v.TopProject = &r.Attribution.ByProject[0]
	}
	for _, s := range r.Growth.TopShares {
		if s.N == 5 {
			v.TopFive = s.Share
		}
	}
	return v
}

// approx writes a token count briefly: 50000 -> "50k", 967437 -> "967k",
// 13259634 -> "13.3M".
func approx(n int) string {
	switch {
	case n >= 999_500: // would round to "1000k"
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%dk", (n+500)/1000)
	}
	return fmt.Sprint(n)
}
