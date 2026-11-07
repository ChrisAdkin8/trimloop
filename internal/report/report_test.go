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
		"Where spend piles up", "Sub-agents", "general-purpose", "&lt;script&gt;alert(1)&lt;/script&gt;",
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
	if !strings.Contains(buf.String(), "No recommendations") {
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
	if !strings.Contains(out, "Full report: out.html") || strings.Count(out, "\n  2. ") != 0 {
		t.Errorf("got %q", out)
	}
	if len(r.Actions) > 0 && !strings.Contains(out, "1. ") {
		t.Errorf("summary has no first action: %q", out)
	}
}
