package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/fixes"
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
	if err := WriteText(&buf, Analyse(nil, Read{Logs: 1}, Local{}, DefaultSettings(), t0)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"COST BY TOKEN CLASS", "WHERE THE SPEND WENT", "LONG SESSIONS (P19)", "P10)", "LONG BREAKS (P09)", "TOOL OUTPUT (P04)", "SUB-AGENTS ON THE MAIN MODEL (P13)"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report has no %q", want)
		}
	}
}

// failingWriter fails every write, like a closed pipe or a full disk.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestReportReturnsWriteError(t *testing.T) {
	err := WriteText(failingWriter{}, Analyse(nil, Read{Logs: 1}, Local{}, DefaultSettings(), t0))
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
	r := Analyse(sample(), Read{Logs: 2}, Local{}, DefaultSettings(), t0)
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
	r := Analyse(sample(), Read{}, Local{}, DefaultSettings(), t0)
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
	r := Analyse(sample(), Read{Logs: 2}, Local{}, DefaultSettings(), t0)
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
	if err := WriteHTML(&buf, Analyse(nil, Read{}, Local{}, DefaultSettings(), t0)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Nothing found was worth changing") {
		t.Error("an empty report should say there is nothing to do")
	}
}

func TestSummary(t *testing.T) {
	r := Analyse(sample(), Read{Logs: 2}, Local{}, DefaultSettings(), t0)
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
	for n, want := range map[int]string{0: "0", 999: "999", 50_000: "50k", 967_437: "967k", 999_499: "999k", 999_500: "1.0M", 13_259_634: "13.3M"} {
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
	r := Analyse(sessions, Read{}, Local{}, DefaultSettings(), t0)
	if l := r.Labels["m1"]; l.Title != "Fix the scoring" || l.SubAgent {
		t.Errorf("main: got %+v", l)
	}
	settings := DefaultSettings()
	settings.Titles = false
	if l := Analyse(sessions, Read{}, Local{}, settings, t0).Labels["m1"]; l.Title != "" {
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
	p := Analyse(s, Read{}, Local{}, DefaultSettings(), t0).Period
	if p.Days != 14 || !p.From.Equal(t0) || p.PerWeek != 3.5 {
		t.Errorf("got %+v, want 14 days at $3.50 a week", p)
	}
}

func TestSummaryWithoutFile(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteSummary(&buf, Analyse(sample(), Read{}, Local{}, DefaultSettings(), t0), "", 3); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "--format html") {
		t.Errorf("summary does not say how to get the full report: %q", buf.String())
	}
}

func TestMoney(t *testing.T) {
	for v, want := range map[float64]string{-6.08: "-$6.08", -1234.5: "-$1,235", 0: "$0", 9.994: "$9.99", 10: "$10", 1234.5: "$1,235"} {
		if got := money(v); got != want {
			t.Errorf("money(%v) = %q, want %q", v, got, want)
		}
	}
}

func TestTextHasNewSections(t *testing.T) {
	sessions := sample()
	sessions[0].Harness = &event.HarnessRecord{Cost: 105, ByModel: map[string]event.ModelRecord{"claude-opus-5-5": {Cost: 105}}}
	sessions[0].Turns[0].Time = t0
	r := Analyse(sessions, Read{}, Local{}, DefaultSettings(), t0)
	var buf bytes.Buffer
	if err := WriteText(&buf, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CHECKED AGAINST CLAUDE CODE'S OWN RECORDS", "WEEK BY WEEK", "WHAT EVERY REQUEST CARRIES (P07)"} {
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
		"github (instructions)": "MCP server github: instructions",
	} {
		if got := contextName(in, false); got != want {
			t.Errorf("contextName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The web page's headline share of large turns is the text report's P19
// figure and the weekly table's: input only, and only turns strictly over
// the threshold.
func TestLargeShareMatchesP19(t *testing.T) {
	turn := func(ctx int, output float64) event.Turn {
		return event.Turn{Model: "claude-opus-5-5", Time: t0, Usage: event.Usage{CacheRead: ctx},
			Cost: event.Cost{CacheRead: float64(ctx) / 1e6, Output: output}, Priced: true}
	}
	s := []event.Session{{ID: "m", Kind: "main", Turns: []event.Turn{
		turn(10_000, 0), turn(300_000, 4), turn(300_000, 4), turn(200_000, 0)}}}
	r := Analyse(s, Read{}, Local{}, DefaultSettings(), t0)
	v := newSummary(r)
	if want := pct(0.6, r.Total); !near(v.LargeSpendShare, want) || !near(v.LargeTurnShare, 50) {
		t.Errorf("got %v%% of spend, %v%% of turns; want %v%%, 50%%", v.LargeSpendShare, v.LargeTurnShare, want)
	}
	var large float64
	for _, w := range r.Weeks {
		large += w.LargeContext
	}
	if !near(large, r.Context.Cost) {
		t.Errorf("weekly large-turn cost %v, P19 %v", large, r.Context.Cost)
	}
}

// The headline's share of large turns counts the same turns as the bars,
// priced or not.
func TestLargeTurnShareCountsAllTurns(t *testing.T) {
	small := event.Turn{Model: "claude-opus-5-5", Time: t0, Usage: event.Usage{CacheRead: 10_000}, Cost: event.Cost{CacheRead: 0.01}, Priced: true}
	large := event.Turn{Model: "mystery", Time: t0, Usage: event.Usage{CacheRead: 300_000}}
	s := []event.Session{{ID: "m", Kind: "main", Turns: []event.Turn{small, small, large, large}}}
	v := newSummary(Analyse(s, Read{}, Local{}, DefaultSettings(), t0))
	bars := 0.0
	for _, b := range v.Bands[2:] {
		bars += b.TurnShare
	}
	if !near(v.LargeTurnShare, 50) || !near(bars, 50) {
		t.Errorf("headline %v%% of turns, bars %v%%; want 50%% both", v.LargeTurnShare, bars)
	}
}

// The summary at the top shows spend against turns by conversation size, and
// what filled the conversation.
func TestHTMLSummary(t *testing.T) {
	r := Analyse(sample(), Read{}, Local{}, DefaultSettings(), t0)
	v := newSummary(r)
	if len(v.Bands) != 4 || v.Bands[0].Label != "Up to 50k" || v.Bands[3].Label != "Over 500k" {
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
	for _, want := range []string{"91% of your spend went on sending turns over 200k tokens", "paid to send the session to Claude",
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

func TestDollarsNegative(t *testing.T) {
	if got := dollars(-6.08); got != "-$6.08" {
		t.Errorf("dollars(-6.08) = %q", got)
	}
}

// When no session can be compared, the text says why.
func TestReconcileSectionGivesReasons(t *testing.T) {
	var b strings.Builder
	reconcileSection(&b, detectors.Reconciliation{Unpriced: 1, Behind: 2}, 10, nil)
	out := b.String()
	if strings.Contains(out, "No session has a cost record") || !strings.Contains(out, "no known price") ||
		!strings.Contains(out, "2 sessions have a record that does not cover all of them (they began") {
		t.Errorf("got %q", out)
	}
	b.Reset()
	reconcileSection(&b, detectors.Reconciliation{Behind: 1}, 10, nil)
	if out := b.String(); !strings.Contains(out, "1 session has a record that does not cover all of it (it began") {
		t.Errorf("one session: got %q", out)
	}
	// The web page says the same, in the singular and the plural.
	for n, want := range map[int]string{1: "1 session has a record that does not cover all of it (it began",
		2: "2 sessions have a record that does not cover all of them (they began"} {
		r := Analyse(sample(), Read{}, Local{}, DefaultSettings(), t0)
		r.Reconciliation.Sessions, r.Reconciliation.Behind = 1, n
		var page bytes.Buffer
		if err := WriteHTML(&page, r); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(page.String(), want) {
			t.Errorf("web page with %d left out has no %q", n, want)
		}
	}
}

func TestOlderFigures(t *testing.T) {
	now := detectors.AnalysisVersion
	for _, tc := range []struct {
		w    detectors.Week
		want bool
	}{
		{detectors.Week{Analysis: now, Prices: "p"}, false},
		{detectors.Week{Analysis: now}, false},           // no prices recorded: not a difference
		{detectors.Week{Analysis: 0, Prices: "p"}, true}, // stored before versions were recorded
		{detectors.Week{Analysis: now, Prices: "old"}, true},
	} {
		if got := olderFigures(tc.w, "p"); got != tc.want {
			t.Errorf("%+v: got %v, want %v", tc.w, got, tc.want)
		}
	}
}

// The likely compaction saving comes with a range from the quartiles of the
// size after a compaction: a smaller size saves more, and likely lies inside.
func TestCompactionLikelyRange(t *testing.T) {
	// History at $5 per million tokens (as on a turn that rebuilt its cache),
	// so a compaction pays for itself within a few turns.
	turn := func(ctx int) event.Turn {
		return event.Turn{Model: "claude-opus-5-5", Usage: event.Usage{CacheRead: ctx},
			Cost: event.Cost{CacheRead: 5 * float64(ctx) / 1e6}, Priced: true}
	}
	var sessions []event.Session
	// Four sessions compacted once each, sending 40k to 100k afterwards.
	for i, after := range []int{40_000, 60_000, 80_000, 100_000} {
		resumed := turn(after)
		resumed.CompactedBefore = true
		resumed.Compaction = event.Compaction{Before: 400_000, After: 20_000}
		sessions = append(sessions, event.Session{ID: fmt.Sprint("c", i), Kind: "main",
			Turns: []event.Turn{turn(390_000), resumed}})
	}
	// One long session that never compacted.
	var long []event.Turn
	for ctx := 100_000; ctx <= 900_000; ctx += 50_000 {
		long = append(long, turn(ctx))
	}
	sessions = append(sessions, event.Session{ID: "long", Kind: "main", Turns: long})

	r := Analyse(sessions, Read{}, Local{}, DefaultSettings(), t0)
	f := r.CompactFix
	if f == nil {
		t.Fatal("no compaction card")
	}
	if !(f.LikelyLow <= f.Likely && f.Likely <= f.LikelyHigh && f.LikelyLow < f.LikelyHigh) {
		t.Errorf("range: got %v ≤ %v ≤ %v", f.LikelyLow, f.Likely, f.LikelyHigh)
	}
	g := r.Growth
	if g.ResumeLow != 60_000 || g.ResumeHigh != 100_000 || g.Resumes != 4 {
		t.Errorf("quartiles: got %d–%d of %d", g.ResumeLow, g.ResumeHigh, g.Resumes)
	}
	// The smaller size pairs with the larger saving.
	if want := "at 60k the likely saving is " + money(f.LikelyHigh); !strings.Contains(f.LikelyNote, want) {
		t.Errorf("note %q has no %q", f.LikelyNote, want)
	}
	if !f.Estimated || f.LikelyHigh > f.Saving {
		t.Errorf("got estimated=%v, high %v above up to %v", f.Estimated, f.LikelyHigh, f.Saving)
	}
}

// The likely range never passes "up to", even when the replay at the low
// quartile of the size after a compaction would.
func TestCompactionLikelyRangeCapped(t *testing.T) {
	turn := func(ctx int) event.Turn {
		return event.Turn{Model: "claude-opus-5-5", Usage: event.Usage{CacheRead: ctx},
			Cost: event.Cost{CacheRead: 5 * float64(ctx) / 1e6}, Priced: true}
	}
	var sessions []event.Session
	for i, after := range []int{20_000, 40_000, 60_000, 80_000} {
		resumed := turn(after)
		resumed.CompactedBefore = true
		resumed.Compaction = event.Compaction{Before: 400_000, After: 10_000}
		sessions = append(sessions, event.Session{ID: fmt.Sprint("c", i), Kind: "main",
			Turns: []event.Turn{turn(390_000), resumed}})
	}
	long := []event.Turn{turn(199_000)}
	for range 600 {
		long = append(long, turn(201_000))
	}
	sessions = append(sessions, event.Session{ID: "long", Kind: "main", Turns: long})

	f := Analyse(sessions, Read{}, Local{}, DefaultSettings(), t0).CompactFix
	if f == nil {
		t.Fatal("no compaction card")
	}
	if f.LikelyHigh > f.Saving || f.Likely > f.Saving {
		t.Errorf("likely %v (%v–%v) passes up to %v", f.Likely, f.LikelyLow, f.LikelyHigh, f.Saving)
	}
}

// With no compaction in the logs, the summary request is priced at the
// measured typical summary, not at the whole size after a compaction.
func TestCompactionSummaryWhenNoneMeasured(t *testing.T) {
	var long []event.Turn
	for ctx := 100_000; ctx <= 900_000; ctx += 50_000 {
		long = append(long, event.Turn{Model: "claude-opus-5-5", Usage: event.Usage{CacheRead: ctx},
			Cost: event.Cost{CacheRead: 5 * float64(ctx) / 1e6}, Priced: true})
	}
	sessions := []event.Session{{ID: "long", Kind: "main", Turns: long}}
	whole := DefaultSettings()
	whole.SummaryTo = whole.CompactTo // the old assumption: the summary is the whole 80k
	a, b := Analyse(sessions, Read{}, Local{}, DefaultSettings(), t0).CompactFix, Analyse(sessions, Read{}, Local{}, whole, t0).CompactFix
	if a == nil || b == nil || a.Likely <= b.Likely {
		t.Errorf("likely with a typical summary %+v should exceed likely with an 80k summary %+v", a, b)
	}
}

func TestOlderReason(t *testing.T) {
	now := detectors.AnalysisVersion
	for _, c := range []struct {
		w    detectors.Week
		want string
	}{
		{detectors.Week{Analysis: now, Prices: "p"}, ""},
		{detectors.Week{Analysis: now, Prices: "q"}, "older prices: q"},
		{detectors.Week{Analysis: 4, Prices: "p"}, fmt.Sprintf("older method: analysis 4, now %d", now)},
		{detectors.Week{Analysis: 0}, "older method"},
		{detectors.Week{Analysis: 4, Prices: "q"}, "older method and prices"},
	} {
		if got := olderReason(c.w, "p"); got != c.want {
			t.Errorf("olderReason(%+v) = %q, want %q", c.w, got, c.want)
		}
	}
}

func TestTextShowsGapSections(t *testing.T) {
	r := Analyse(sample(), Read{Logs: 1}, Local{}, DefaultSettings(), t0)
	r.Retention = Retention{Days: 30}
	r.Failures = detectors.Failures{Sleep: 4, Resent: 2}
	var out bytes.Buffer
	if err := WriteText(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"gone unused for 30 days", "REPLIES CUT OFF (P12)", "4 when the computer slept", "WHEN YOU WORK"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("text has no %q", want)
		}
	}
	r.Retention = Retention{Days: 365, Set: true}
	out.Reset()
	_ = WriteText(&out, r)
	if strings.Contains(out.String(), "gone unused for") {
		t.Error("a year's retention: no tip")
	}
}

func TestRetentionTip(t *testing.T) {
	for _, c := range []struct {
		r    Retention
		want string
	}{
		{Retention{Days: 30}, "gone unused for 30 days (by default)"},
		{Retention{Days: 45, Set: true}, "as cleanupPeriodDays is set"},
		{Retention{Days: 0, Set: true}, "cleanupPeriodDays is 0"},
		{Retention{Days: 90, Set: true}, ""},
		{Retention{}, ""}, // not read: no tip
	} {
		got := RetentionTip(c.r)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("RetentionTip(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
}

// The page renders whole, with and without --no-titles, when MCP servers
// (here a connector known only by an id) are in the spend tables; under
// --no-titles it shows no session or connector ids.
func TestHTMLHidesIDsUnderNoTitles(t *testing.T) {
	for _, titles := range []bool{true, false} {
		s := DefaultSettings()
		s.Titles = titles
		r := Analyse(sample(), Read{Logs: 1}, Local{}, s, t0)
		const id = "1a59c906-04da-521d-bda7-7f71b9f9e01c"
		r.Attribution.ByMCPServer = []detectors.Spend{{Name: id, Cost: 3, Turns: 1}}
		// Every other place a connector's name reaches the page or the text.
		r.Tools.ByTool = append(r.Tools.ByTool, detectors.Group{Name: "mcp__" + id, Carried: 2, Calls: 1})
		r.ToolInputs.Carried = 2
		r.ToolInputs.ByTool = []detectors.Group{{Name: "mcp__" + id, Carried: 2, Calls: 1}}
		r.Standing.Parts = append(r.Standing.Parts,
			detectors.ContextPart{Name: "mcp__" + id, Cost: 1, Tokens: 10, Times: 1},
			detectors.ContextPart{Name: id + " (instructions)", Cost: 1, Tokens: 10, Times: 1})
		card := fixes.Recommendation{Pattern: "P04", Title: "Trim MCP tool results", Saving: 1e6,
			Examples: []string{"mcp__" + id + "__search"}}
		r.Actions = append([]fixes.Recommendation{card}, r.Actions...)
		r.ToolFixes = append(r.ToolFixes, card)
		var out, text bytes.Buffer
		if err := WriteHTML(&out, r); err != nil {
			t.Fatalf("titles %v: %v", titles, err)
		}
		if err := WriteText(&text, r); err != nil {
			t.Fatalf("titles %v: %v", titles, err)
		}
		page := out.String()
		// The footer says what the page shows.
		for footer, want := range map[string]bool{
			"It shows project names and session titles": titles,
			"session titles and ids are left out":       !titles,
		} {
			if strings.Contains(page, footer) != want {
				t.Errorf("titles %v: footer %q shown %v, want %v", titles, footer, !want, want)
			}
		}
		hasID := strings.Contains(page, "1a59c906") || strings.Contains(page, `title="Session m1"`)
		if hasID == !titles || !strings.Contains(page, "unnamed connector") || !strings.Contains(page, "</html>") {
			t.Errorf("titles %v: ids shown %v; page ends %q", titles, hasID, page[len(page)-40:])
		}
		// The whole identifier is never shown; under --no-titles no part is.
		for name, got := range map[string]string{"page": page, "text": text.String()} {
			if strings.Contains(got, id) || (!titles && strings.Contains(got, "1a59c906")) {
				t.Errorf("titles %v: the %s shows the connector's id", titles, name)
			}
		}
	}
}

// The compaction card's two counts are of the same sessions: main sessions
// with a priced turn, the ones its replay changes. A sub-agent or an
// unpriced turn past 500k is left out of both, though the growth figures
// count it.
func TestCompactionCardCountsMainPricedSessions(t *testing.T) {
	turn := func(ctx int, priced bool) event.Turn {
		return event.Turn{Model: "claude-opus-5-5", Usage: event.Usage{CacheRead: ctx},
			Cost: event.Cost{CacheRead: 5 * float64(ctx) / 1e6}, Priced: priced}
	}
	grow := func(top int, priced bool) []event.Turn {
		var ts []event.Turn
		for ctx := 100_000; ctx <= top; ctx += 50_000 {
			ts = append(ts, turn(ctx, priced))
		}
		return ts
	}
	compacted := grow(600_000, true)
	compacted[5] = turn(50_000, true) // the first request after a compaction
	compacted[5].CompactedBefore = true
	compacted[5].Compaction = event.Compaction{Before: 350_000, After: 20_000}
	sessions := []event.Session{
		{ID: "long", Kind: event.MainSession, Turns: grow(900_000, true)},           // over both
		{ID: "mid", Kind: event.MainSession, Turns: grow(300_000, true)},            // over 200k only
		{ID: "done", Kind: event.MainSession, Turns: compacted},                     // compacted
		{ID: "long/agent", Kind: event.SubAgentSession, Turns: grow(900_000, true)}, // a sub-agent
		{ID: "unpriced", Kind: event.MainSession, Turns: grow(900_000, false)},      // no price
	}
	r := Analyse(sessions, Read{}, Local{}, DefaultSettings(), t0)
	if r.CompactFix == nil {
		t.Fatal("no compaction card")
	}
	got := r.CompactFix.Finding
	for _, want := range []string{"3 sessions passed 200,000 tokens", "1 session went past 500,000 tokens without compacting"} {
		if !strings.Contains(got, want) {
			t.Errorf("finding %q has no %q", got, want)
		}
	}
	if r.Growth.Uncompacted != 3 {
		t.Errorf("growth: got %d never compacted, want 3 (every session)", r.Growth.Uncompacted)
	}
}

// A conversation whose main log is gone is named after its first sub-agent
// with a time; one with no time never counts as first.
func TestLabelsFirstSubAgentHasATime(t *testing.T) {
	timed := sample()[0]
	timed.ID, timed.Kind, timed.ParentID, timed.Project = "gone/a", event.SubAgentSession, "gone", "api"
	timed.Turns = append([]event.Turn(nil), timed.Turns...)
	for i := range timed.Turns {
		timed.Turns[i].Time = t0.Add(time.Duration(i) * time.Minute)
	}
	untimed := timed
	untimed.ID, untimed.Project = "gone/b", "web"
	untimed.Turns = append([]event.Turn(nil), timed.Turns...)
	for i := range untimed.Turns {
		untimed.Turns[i].Time = time.Time{}
	}
	for _, order := range [][]event.Session{{untimed, timed}, {timed, untimed}} {
		r := Results{Growth: detectors.Growth{Top: []detectors.ConversationStat{{ID: "gone"}}}}
		if l := labels(order, r, true)["gone"]; l.Project != "api" || l.Start.IsZero() {
			t.Errorf("got %+v, want the timed sub-agent's project and start", l)
		}
	}
}

// Counts of one read as one in both formats: "1 time", "1 session", never
// "1 times" or "1 was a breaks".
func TestCountsOfOne(t *testing.T) {
	r := Analyse(sample(), Read{Logs: 1}, Local{}, DefaultSettings(), t0)
	r.Pause.Pauses, r.Pause.Sessions, r.Pause.Saving, r.Pause.Cost = 1, 1, 1, 1
	r.Growth.Compactions, r.CacheLife.Waits, r.Switches.Switches = 1, 1, 1
	r.Reconciliation.Sessions, r.SubAgents.Runs, r.Period.Days, r.Read.Unpriced = 1, 1, 1, 1
	r.Reconciliation.Unrecorded, r.Reconciliation.Unpriced, r.Reconciliation.WebSearches = 1, 1, 1
	r.Read.Failed, r.Read.BadLines, r.Tools.Calls, r.Tools.Persisted = 1, 1, 1, 1
	bad := regexp.MustCompile(`\b1 (times|sessions|days|turns|runs|logs|lines|results|tool results|switches|web searches)\b|was a breaks|search[s]\b`) // the last: "search" with a plain s added
	var text, page bytes.Buffer
	if err := WriteText(&text, r); err != nil {
		t.Fatal(err)
	}
	if err := WriteHTML(&page, r); err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]string{"text": text.String(), "page": page.String()} {
		if m := bad.FindAllString(out, -1); len(m) > 0 {
			t.Errorf("%s says %q", name, m)
		}
	}
	if !strings.Contains(text.String(), "1 time, work carried on") || !strings.Contains(page.String(), "1 time in 1 session, work carried on") {
		t.Error("the long-break sentences do not say 1 time")
	}
	if !strings.Contains(page.String(), "1 turn used a model with no known price and is left out.") {
		t.Error(`the page does not say "1 turn ... is left out"`)
	}
}

// Nouns that end in a hissing sound take "es", not a plain "s".
func TestCountPlurals(t *testing.T) {
	for _, c := range []struct {
		n          int
		noun, want string
	}{
		{1, "web search", "1 web search"},
		{133, "web search", "133 web searches"},
		{2, "switch", "2 switches"},
		{2000, "log", "2,000 logs"},
		{0, "session", "0 sessions"},
	} {
		if got := count(c.n, c.noun); got != c.want {
			t.Errorf("count(%d, %q) = %q, want %q", c.n, c.noun, got, c.want)
		}
	}
}
