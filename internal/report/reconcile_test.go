package report

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// recordedSession is a main session on Opus 5.5 whose record says recorded,
// with cachedRecorded tokens read from cache and outRecorded written, while
// its one logged turn cost logged, read cachedLogged and wrote outLogged.
func recordedSession(id, title string, recorded, logged float64, cachedRecorded, cachedLogged, outRecorded, outLogged int) event.Session {
	return event.Session{ID: id, Kind: event.MainSession, Project: "api", Title: title,
		Harness: &event.HarnessRecord{Cost: recorded, At: t0.Add(time.Hour), ByModel: map[string]event.ModelRecord{
			"claude-opus-5-5": {Cost: recorded, CacheRead: cachedRecorded, Output: outRecorded}}},
		Turns: []event.Turn{{Model: "claude-opus-5-5", Time: t0, Priced: true,
			Usage: event.Usage{CacheRead: cachedLogged, Output: outLogged}, Cost: event.Cost{CacheRead: logged}}}}
}

// spreadSessions are ten sessions with a gap of 10 cents each: a record 20 %
// over the logs on cached work and level on output, as summaries after a
// break leave it.
func spreadSessions() []event.Session {
	var out []event.Session
	for i := range 10 {
		out = append(out, recordedSession(fmt.Sprintf("s%d", i), "", 1.10, 1, 1_200_000, 1_000_000, 1_000, 1_000))
	}
	return out
}

// concentratedSessions add three sessions with large gaps to the spread
// ones, two of them equal, and two left out of the comparison that would
// otherwise top the list.
func concentratedSessions() []event.Session {
	late := recordedSession("late", "Left running", 500, 1, 0, 0, 0, 0)
	late.Harness.At = t0.Add(-time.Hour) // work went on after the record
	unpriced := recordedSession("unpriced", "Unpriced", 400, 1, 0, 0, 0, 0)
	unpriced.Turns[0].Priced = false
	return append(spreadSessions(),
		recordedSession("big", "Fix the parser", 90, 30, 125_000_000, 88_000_000, 716_000, 140_000),
		event.Session{ID: "big/a", Kind: event.SubAgentSession, ParentID: "big", Project: "api",
			Turns: []event.Turn{{Model: "claude-opus-5-5", Time: t0, Priced: true}}},
		recordedSession("tie-b", "Second tie", 20, 10, 0, 0, 0, 0),
		recordedSession("tie-a", "First tie", 20, 10, 0, 0, 0, 0),
		late, unpriced,
	)
}

// reconcileOutputs renders the summary, the text report's reconciliation
// section and the web page's.
func reconcileOutputs(t *testing.T, sessions []event.Session, titles bool) (summary, text, page string) {
	t.Helper()
	s := DefaultSettings()
	s.Titles = titles
	r := Analyse(sessions, Read{Logs: len(sessions)}, Local{}, s, t0)
	var a, b, c bytes.Buffer
	if err := WriteSummary(&a, r, "", 3); err != nil {
		t.Fatal(err)
	}
	if err := WriteText(&b, r); err != nil {
		t.Fatal(err)
	}
	if err := WriteHTML(&c, r); err != nil {
		t.Fatal(err)
	}
	text = b.String()
	text = text[strings.Index(text, "CHECKED AGAINST"):strings.Index(text, "WEEK BY WEEK")]
	page = c.String()
	page = page[strings.Index(page, `<h2 id="checks">`):]
	page = page[:strings.Index(page, "</section>")]
	return a.String(), text, page
}

// A small gap spread over every session: background requests, no word of
// concentration or of missing requests.
func TestReconciliationSpreadGap(t *testing.T) {
	summary, text, page := reconcileOutputs(t, spreadSessions(), true)
	want := "Claude Code's own records show $1.00 (10%) more than its logs: requests the logs do not record.\n" +
		"The findings below come from the logs, so they leave it out.\n"
	if !strings.Contains(summary, want) {
		t.Errorf("summary: want %q in\n%s", want, summary)
	}
	for _, want := range []string{
		"  $1.00 (10.0%) was spent on requests the logs do not record. Part of it is\n" +
			"  background requests: session titles, summaries after a break, compactions and\n" +
			"  web searches.\n" +
			"  The findings in this report are worked out from the logs, so they leave this spend out.\n",
		"    12.0M / 10.0M     0 / 0             10k / 10k       0 / 0           Opus 5.5\n",
		"  Sessions with the largest gaps ($0.30 of the $1.00):\n",
		"    $1.10      $1.00      $0.10                   0  api                  s0\n" +
			"    $1.10      $1.00      $0.10                   0  api                  s1\n" +
			"    $1.10      $1.00      $0.10                   0  api                  s2\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text: want %q in\n%s", want, text)
		}
	}
	if strings.Contains(text, "cached work") || strings.Contains(page, "cached work") {
		t.Error("a record over the logs on cached work alone says requests are missing")
	}
	if want := "requests the logs don't record. The findings on this page come from the logs, so they leave it out.</p>"; !strings.Contains(page, want) {
		t.Errorf("page: want %q in\n%s", want, page)
	}
}

// Most of the gap in a few sessions: the summary says how much, the rows
// list them largest first with ties by id, sessions left out of the
// comparison are not listed, and the record's extra cached work and output are named.
func TestReconciliationConcentratedGap(t *testing.T) {
	summary, text, page := reconcileOutputs(t, concentratedSessions(), true)
	want := "Claude Code's own records show $81 (135%) more than its logs: requests the logs do not record.\n" +
		"$80 of it is in 3 sessions. The findings below come from the logs, so they leave it out.\n"
	if !strings.Contains(summary, want) {
		t.Errorf("summary: want %q in\n%s", want, summary)
	}
	for _, want := range []string{
		"  The record also counts clearly more cached work and output than the logs hold\n" +
			"  for Opus 5.5:\n" +
			"  some of the sessions' own requests are missing from the logs, and why is not known.\n",
		"    137.0M / 98.0M    0 / 0             726k / 150k     0 / 0           Opus 5.5\n",
		"  Sessions with the largest gaps ($80 of the $81):\n" +
			"    recorded   in logs    gap        sub-agent logs  project              session\n" +
			"    $90        $30        $60                     1  api                  big  Fix the parser\n" +
			"    $20        $10        $10                     0  api                  tie-a  First tie\n" +
			"    $20        $10        $10                     0  api                  tie-b  Second tie\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text: want %q in\n%s", want, text)
		}
	}
	for _, out := range []string{text, page} {
		if strings.Contains(out, "Left running") || strings.Contains(out, "Unpriced") {
			t.Errorf("a session left out of the comparison is listed:\n%s", out)
		}
	}
	for _, want := range []string{
		"requests the logs don't record. $80 of it is in 3 sessions. The findings on this page come from the logs",
		"than the logs hold for Opus 5.5: some of the sessions' own requests are missing from the logs, and why is not known.",
		"<td class=\"n\">137.0M / 98.0M</td><td class=\"n\">0 / 0</td><td class=\"n\">726k / 150k</td>",
		"Together $80 of the $81.",
		`Fix the parser<span class="id" title="Session big">`,
		"<td class=\"name\">api</td><td class=\"n\">$90</td><td class=\"n\">$30</td><td class=\"n\">$60</td><td class=\"n\">1</td>",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page: want %q in\n%s", want, page)
		}
	}
	if a, b := strings.Index(page, "First tie"), strings.Index(page, "Second tie"); a < 0 || b < a {
		t.Errorf("page: ties not in id order:\n%s", page)
	}
}

// --no-titles: the rows keep the session id in the text, as its other
// tables do, and name no title; the page shows neither.
func TestReconciliationNoTitles(t *testing.T) {
	_, text, page := reconcileOutputs(t, concentratedSessions(), false)
	if want := "    $90        $30        $60                     1  api                  big\n"; !strings.Contains(text, want) {
		t.Errorf("text: want %q in\n%s", want, text)
	}
	for _, out := range []string{text, page} {
		for _, title := range []string{"Fix the parser", "First tie", "Second tie"} {
			if strings.Contains(out, title) {
				t.Errorf("--no-titles shows %q:\n%s", title, out)
			}
		}
	}
	if strings.Contains(page, `title="Session big"`) {
		t.Errorf("--no-titles: the page shows a session id:\n%s", page)
	}
}
