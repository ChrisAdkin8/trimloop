package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

var t0 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

func TestThousands(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567", -1234: "-1,234"} {
		if got := thousands(n); got != want {
			t.Errorf("thousands(%d) = %q, want %q", n, got, want)
		}
	}
	if got := dollars(1234.5); got != "$1,234.50" {
		t.Errorf("dollars(1234.5) = %q, want $1,234.50", got)
	}
}

func TestDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		time.Hour: "1 hour", 2 * time.Hour: "2 hours", 90 * time.Minute: "90 minutes", 90 * time.Second: "1m30s",
	} {
		if got := duration(d); got != want {
			t.Errorf("duration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestReportRuns(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteText(&buf, Analyse(nil, Read{Logs: 1}, DefaultSettings(), t0)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"COST BY TOKEN CLASS", "WHERE THE SPEND WENT", "P19 ", "P10)", "P09 ", "P04 ", "P13 "} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report has no %q", want)
		}
	}
}

// failingWriter fails every write, like a closed pipe or a full disk.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestReportReturnsWriteError(t *testing.T) {
	err := WriteText(failingWriter{}, Analyse(nil, Read{Logs: 1}, DefaultSettings(), t0))
	if err == nil {
		t.Fatal("got no error from a writer that always fails")
	}
}

func TestPrintable(t *testing.T) {
	for in, want := range map[string]string{
		"sed -n":               "sed -n",
		"evil\x1b]0;title\x07": "evil?]0;title?",
		"a\nb\tc\x7f":          "a?b?c?",
		"bad\xffutf8":          "bad?utf8",
		"café":                 "café",
	} {
		if got := printable(in); got != want {
			t.Errorf("printable(%q) = %q, want %q", in, got, want)
		}
	}
}

// sample is a small set of sessions with a sub-agent, a large context and a
// name that tries to inject markup.
func sample() []event.Session {
	turn := func(ctx int, cost float64, project string) event.Turn {
		return event.Turn{Model: "claude-opus-5-5", Usage: event.Usage{CacheRead: ctx},
			Cost: event.Cost{CacheRead: cost}, Priced: true, Project: project}
	}
	return []event.Session{
		{ID: "m1", Kind: "main", Project: "<script>alert(1)</script>", Turns: []event.Turn{
			turn(300_000, 40, "<script>alert(1)</script>"), turn(600_000, 60, "<script>alert(1)</script>")}},
		{ID: "m1/a", Kind: "subagent", ParentID: "m1", AgentType: "general-purpose", Turns: []event.Turn{turn(10_000, 10, "web")}},
	}
}

func TestJSON(t *testing.T) {
	r := Analyse(sample(), Read{Logs: 2}, DefaultSettings(), t0)
	var buf bytes.Buffer
	if err := WriteJSON(&buf, r); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"format", "total", "actions", "attribution", "p19_context", "p19_growth",
		"p09_pauses", "p04_tool_output", "p13_sub_agents"} {
		if _, ok := got[key]; !ok {
			t.Errorf("JSON has no %q", key)
		}
	}
	if got["format"] != FormatVersion || got["total"] != 110.0 {
		t.Errorf("got format %v total %v", got["format"], got["total"])
	}
}

func TestAnalyseOrdersActions(t *testing.T) {
	r := Analyse(sample(), Read{}, DefaultSettings(), t0)
	if r.Read.Sessions != 2 || r.Read.Turns != 3 {
		t.Errorf("read: got %+v", r.Read)
	}
	for i := 1; i < len(r.Actions); i++ {
		if r.Actions[i].Saving > r.Actions[i-1].Saving {
			t.Errorf("actions out of order: %v before %v", r.Actions[i-1].Saving, r.Actions[i].Saving)
		}
	}
}

func TestHTML(t *testing.T) {
	r := Analyse(sample(), Read{Logs: 2}, DefaultSettings(), t0)
	var buf bytes.Buffer
	if err := WriteHTML(&buf, r); err != nil {
		t.Fatal(err)
	}
	page := buf.String()
	for _, want := range []string{"<title>Trimloop report</title>", "What to do first", "Where the money went",
		"Long sessions", "Sub-agents", "Words used in this report", "What every request carries", "general-purpose", "&lt;script&gt;alert(1)&lt;/script&gt;",
		"default-src 'none'"} {
		if !strings.Contains(page, want) {
			t.Errorf("page has no %q", want)
		}
	}
	if strings.Contains(page, "<script>alert(1)") {
		t.Error("a name from the logs was not escaped")
	}
	for _, bad := range []string{"http://", "https://", "<link", "src=\"", "#ZgotmplZ"} {
		if strings.Contains(page, bad) {
			t.Errorf("page contains %q: it must be self-contained and every value valid", bad)
		}
	}
}

func TestHTMLWithNoSessions(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHTML(&buf, Analyse(nil, Read{}, DefaultSettings(), t0)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Nothing found was worth changing") {
		t.Error("an empty report should say there is nothing to do")
	}
}

func TestSummary(t *testing.T) {
	r := Analyse(sample(), Read{Logs: 2}, DefaultSettings(), t0)
	var buf bytes.Buffer
	if err := WriteSummary(&buf, r, "out.html", 1); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Full report: out.html") || strings.Count(out, "\n  2. ") != 0 || !strings.Contains(out, "Spent $110 ") {
		t.Errorf("got %q", out)
	}
	if len(r.Actions) > 0 && !strings.Contains(out, "1. ") {
		t.Errorf("summary has no first action: %q", out)
	}
}

func TestApprox(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 50_000: "50k", 967_437: "967k", 13_259_634: "13.3M"} {
		if got := approx(n); got != want {
			t.Errorf("approx(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestNames(t *testing.T) {
	for in, want := range map[string]string{
		"1a59c906-04da-521d-bda7-7f71b9f9e01c": "unnamed connector (1a59c906…)",
		"ccd_turn":                             "Claude app: task list and wrap-up",
		"ccd_other":                            "Claude app (ccd_other)",
		"plugin:terraform:terraform":           "terraform (from plugin terraform)",
		"github":                               "github",
	} {
		if got := serverName(in); got != want {
			t.Errorf("serverName(%q) = %q, want %q", in, got, want)
		}
	}
	if agentName("unknown") != "type not recorded" || agentName("Plan") != "Plan" || modelName("claude-opus-5-5") != "Opus 5.5" {
		t.Error("agent or model names wrong")
	}
}

// Sessions in the tables are named by title and date, and titles can be
// left out.
func TestLabels(t *testing.T) {
	sessions := sample()
	sessions[0].Title = "Fix the scoring"
	r := Analyse(sessions, Read{}, DefaultSettings(), t0)
	if l := r.Labels["m1"]; l.Title != "Fix the scoring" || l.SubAgent {
		t.Errorf("main: got %+v", l)
	}
	settings := DefaultSettings()
	settings.Titles = false
	if l := Analyse(sessions, Read{}, settings, t0).Labels["m1"]; l.Title != "" {
		t.Errorf("titles off: got %+v", l)
	}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Fix the scoring") {
		t.Error("page does not show the session title")
	}
}

func TestPeriod(t *testing.T) {
	s := []event.Session{{Turns: []event.Turn{{Time: t0, Cost: event.Cost{Output: 7}}, {Time: t0.Add(13 * 24 * time.Hour)}}}}
	p := Analyse(s, Read{}, DefaultSettings(), t0).Period
	if p.Days != 14 || !p.From.Equal(t0) || p.PerWeek != 3.5 {
		t.Errorf("got %+v, want 14 days at $3.50 a week", p)
	}
}

func TestSummaryWithoutFile(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteSummary(&buf, Analyse(sample(), Read{}, DefaultSettings(), t0), "", 3); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "--format html") {
		t.Errorf("summary does not say how to get the full report: %q", buf.String())
	}
}

func TestMoney(t *testing.T) {
	for v, want := range map[float64]string{0: "$0", 9.994: "$9.99", 10: "$10", 1234.5: "$1,235"} {
		if got := money(v); got != want {
			t.Errorf("money(%v) = %q, want %q", v, got, want)
		}
	}
}

func TestTextHasNewSections(t *testing.T) {
	sessions := sample()
	sessions[0].Harness = &event.HarnessRecord{Cost: 105, ByModel: map[string]event.ModelRecord{"claude-opus-5-5": {Cost: 105}}}
	sessions[0].Turns[0].Time = t0
	r := Analyse(sessions, Read{}, DefaultSettings(), t0)
	var buf bytes.Buffer
	if err := WriteText(&buf, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CHECKED AGAINST CLAUDE CODE'S OWN RECORDS", "WEEK BY WEEK", "P07 WHAT EVERY REQUEST CARRIES"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("text has no %q", want)
		}
	}
	var page bytes.Buffer
	if err := WriteHTML(&page, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Checking the totals", "Week by week", "(so far)"} {
		if !strings.Contains(page.String(), want) {
			t.Errorf("page has no %q", want)
		}
	}
}

func TestContextName(t *testing.T) {
	for in, want := range map[string]string{
		"skill_listing": "List of available skills", "mcp__github": "MCP server github: tool definitions",
		"Project CLAUDE.md": "Project CLAUDE.md", "some_new_kind": "some new kind",
	} {
		if got := contextName(in); got != want {
			t.Errorf("contextName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The summary at the top shows spend against turns by conversation size, and
// what filled the conversation.
func TestHTMLSummary(t *testing.T) {
	r := Analyse(sample(), Read{}, DefaultSettings(), t0)
	v := newSummary(r)
	if len(v.Bands) != 4 || v.Bands[0].Label != "Under 50k" || v.Bands[3].Label != "Over 500k" {
		t.Fatalf("bands: got %+v", v.Bands)
	}
	// sample(): 300k ($40), 600k ($60), 10k ($10) turns; total $110.
	if !near(v.LargeSpendShare, 100*100.0/110) || !near(v.LargeTurnShare, 100*2.0/3) {
		t.Errorf("large shares: got %v of spend, %v of turns", v.LargeSpendShare, v.LargeTurnShare)
	}
	if v.SmallCost != "$10" || v.LargeCost != "$40–$60" {
		t.Errorf("cost ranges: got %q and %q", v.SmallCost, v.LargeCost)
	}
	sum := 0.0
	for _, p := range v.Fill {
		sum += p.Cost
	}
	if !near(sum, v.Input) {
		t.Errorf("the parts should add up to what was sent: %v against %v", sum, v.Input)
	}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, r); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "ZgotmplZ") {
		t.Error("the template blanked a value it considered unsafe")
	}
	for _, want := range []string{"91% of your spend came from turns over 200k tokens", "paid to send the conversation to Claude",
		"Tool output", `class="size4"`, `class="kind2"`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("page has no %q", want)
		}
	}
}

func near(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }

func TestCostRange(t *testing.T) {
	if costRange(nil) != "" || costRange([]float64{0.061, 0.0612}) != "$0.06" || costRange([]float64{0.15, 0.28}) != "$0.15–$0.28" {
		t.Error("costRange is wrong")
	}
}
