package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/fixes"
)

func TestAddLimitsSharesOfTheWeek(t *testing.T) {
	r := Results{Period: Period{Days: 14}, Actions: []fixes.Recommendation{
		{Title: "a", Saving: 100, Likely: 60, Estimated: true}, // $30 a week likely
		{Title: "b", Saving: 2},                                // $1 a week at most
		{Title: "c", Saving: 100, Estimated: true},             // replayed: saves nothing
	}}
	t0 := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	var turns []event.Turn
	for h := range 100 {
		turns = append(turns, event.Turn{Time: t0.Add(time.Duration(h) * time.Hour), Cost: event.Cost{Output: 1}})
	}
	reset := t0.Add(6 * 24 * time.Hour).Unix()
	readings := []detectors.LimitReading{
		{At: t0.Add(time.Minute), SevenDay: &detectors.LimitWindow{Used: 10, ResetsAt: reset}},
		{At: t0.Add(50*time.Hour + time.Minute), SevenDay: &detectors.LimitWindow{Used: 20, ResetsAt: reset}}, // +10 points for $50
	}
	AddLimits(&r, []event.Session{{ID: "s", Kind: "main", Turns: turns}}, Plan{Name: "Max 5×", Subscription: true}, readings)
	if got := r.Actions[0].WeeklyLimit; got != 6 { // $30 a week at 0.2 points a dollar
		t.Errorf("share for a: got %v, want 6", got)
	}
	if got := limitText(r.Actions[1]); got != "; under 1% of your weekly limit each week" {
		t.Errorf("small share: got %q", got)
	}
	// An estimate of nothing is not replaced by the upper bound.
	if got := r.Actions[2].WeeklyLimit; got != 0 {
		t.Errorf("share for c, estimated to save nothing: got %v, want 0", got)
	}
	if got := savingText(r.Actions[2], 1000); !strings.HasPrefix(got, "likely nothing") {
		t.Errorf("c: got %q, want it to say the likely saving is nothing", got)
	}
	var out bytes.Buffer
	if err := WriteSummary(&out, r, "", 3); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"the Max 5× plan, a flat fee", "No usage limit was reached", "Each $10 of work at list prices used about 2.0% of your weekly limit",
		"about 6% of your weekly limit each week"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("summary has no %q:\n%s", want, out.String())
		}
	}
}

func TestLimitsOnlyForSubscribers(t *testing.T) {
	r := Results{Actions: []fixes.Recommendation{{Title: "a", Saving: 10}}}
	AddLimits(&r, sample(), Plan{}, nil)
	var out bytes.Buffer
	if err := WriteSummary(&out, r, "", 3); err != nil {
		t.Fatal(err)
	}
	if r.Subscriber() || strings.Contains(out.String(), "flat fee") || strings.Contains(out.String(), "weekly limit") {
		t.Errorf("API user: subscriber %v, summary:\n%s", r.Subscriber(), out.String())
	}
	// Limit hits alone mark a subscriber, even with no account file.
	s := sample()
	s[0].LimitHits = []event.LimitHit{{At: time.Date(2026, 10, 9, 21, 0, 0, 0, time.UTC), Limit: "session"}}
	AddLimits(&r, s, Plan{}, nil)
	if !r.Subscriber() || r.LimitHitsText() != "Claude Code stopped 1 time because a usage limit was reached, most recently on 9 Oct." {
		t.Errorf("hits: subscriber %v, %q", r.Subscriber(), r.LimitHitsText())
	}
	if !strings.Contains(r.LimitRateText(), "trimloop setup") {
		t.Errorf("no readings: got %q", r.LimitRateText())
	}
}

func TestAddLocalSetsEverythingFromOutsideTheLogs(t *testing.T) {
	var r Results
	AddLocal(&r, sample(), Local{Version: "v1.2.3", Plan: Plan{Name: "Pro", Subscription: true}, Retention: Retention{Days: 365, Set: true}})
	if r.Version != "v1.2.3" || r.Plan.Name != "Pro" || !r.Subscriber() || r.Retention.Days != 365 || r.ShortRetention() {
		t.Errorf("got version %q, plan %+v, retention %+v", r.Version, r.Plan, r.Retention)
	}
}
