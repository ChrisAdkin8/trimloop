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
	"dollars":   dollars,
	"thousands": thousands,
	"short":     short,
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
	"date":     func(t time.Time) string { return t.Format("2 January 2006, 15:04 UTC") },
	"duration": func(d time.Duration) string { return duration(d.Round(time.Second)) },
	"join":     strings.Join,
	"add":      func(a, b int) int { return a + b },
	"kind":     func(k fixes.Kind) string { return string(k) },
	"float":    func(n int) float64 { return float64(n) },
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

	MaxProject, MaxBand, MaxTool, MaxAgent float64
}

// sessionRow joins one session's P19 and P09 figures.
type sessionRow struct {
	ID          string
	Peak        int
	ContextSave float64 // P19
	PauseSave   float64 // P09
	Pauses      int
}

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
	return v
}
