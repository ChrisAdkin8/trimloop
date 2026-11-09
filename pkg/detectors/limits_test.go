package detectors

import (
	"sort"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

func TestFindLimitsCountsHits(t *testing.T) {
	t0 := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	s := []event.Session{
		{ID: "a", Kind: "main", Turns: []event.Turn{{Time: t0}}, LimitHits: []event.LimitHit{{At: t0, Limit: "session"}, {At: t0.Add(time.Hour), Limit: "weekly"}}},
		{ID: "b", Kind: "main", LimitHits: []event.LimitHit{{At: t0.Add(2 * time.Hour), Limit: "other"}}}, // refused at its first message
	}
	l := FindLimits(s, nil, 20, 5)
	if l.Hits != 3 || l.Session != 1 || l.Weekly != 1 || !l.Last.Equal(t0.Add(2*time.Hour)) {
		t.Errorf("hits: got %+v", l)
	}
	if l.SevenDay.PerDollar != 0 || l.Readings != 0 {
		t.Errorf("no readings: got %+v", l)
	}
}

func TestFindLimitsRate(t *testing.T) {
	t0 := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	week1, week2 := t0.Add(5*24*time.Hour).Unix(), t0.Add(12*24*time.Hour).Unix()
	var turns []event.Turn
	for h := range 300 { // $1 an hour for 300 hours
		turns = append(turns, event.Turn{Time: t0.Add(time.Duration(h) * time.Hour), Cost: event.Cost{Output: 1}})
	}
	s := []event.Session{{ID: "a", Kind: "main", Turns: turns}}
	read := func(h int, used float64, reset int64) LimitReading {
		return LimitReading{At: t0.Add(time.Duration(h)*time.Hour + time.Minute), SevenDay: &LimitWindow{Used: used, ResetsAt: reset},
			FiveHour: &LimitWindow{Used: 50, ResetsAt: reset}}
	}
	readings := []LimitReading{
		read(10, 10, week1), read(20, 13, week1), read(40, 19, week1), // +9 points over 30 hours ($30)
		read(130, 2, week2+20), read(150, 8, week2), // +6 over 20 hours ($20); reset times 20 s apart: one window
		read(400, 90, week2), // after the logs end: ignored
	}
	l := FindLimits(s, readings, 20, 5)
	r := l.SevenDay
	if r.Windows != 2 || r.Rise != 15 || r.Spend != 50 || r.PerDollar != 0.3 {
		t.Errorf("weekly rate: got %+v, want 2 windows, rise 15, spend 50, 0.3 a dollar", r)
	}
	if l.Readings != 5 {
		t.Errorf("readings in the period: got %d, want 5", l.Readings)
	}
	if f := l.FiveHour; f.Rise != 0 || f.PerDollar != 0 {
		t.Errorf("five-hour limit did not move: got %+v", f)
	}
	if l := FindLimits(s, readings, 100, 5); l.SevenDay.PerDollar != 0 {
		t.Error("under the least spend: want no rate")
	}
	if l := FindLimits(s, readings, 20, 16); l.SevenDay.PerDollar != 0 {
		t.Error("under the least rise: want no rate")
	}
}

func TestFindLimitsSkipsAReset(t *testing.T) {
	t0 := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	s := []event.Session{{ID: "a", Kind: "main", Turns: []event.Turn{{Time: t0}, {Time: t0.Add(10 * time.Hour), Cost: event.Cost{Output: 30}}}}}
	reset := t0.Add(48 * time.Hour).Unix()
	readings := []LimitReading{
		{At: t0.Add(time.Hour), SevenDay: &LimitWindow{Used: 40, ResetsAt: reset}},
		{At: t0.Add(9 * time.Hour), SevenDay: &LimitWindow{Used: 10, ResetsAt: reset}}, // fell: not a rise
	}
	if r := FindLimits(s, readings, 1, 1).SevenDay; r.Windows != 0 || r.Rise != 0 {
		t.Errorf("a falling reading: got %+v, want no window", r)
	}
}

func TestFindLimitsWindowAcrossAMinuteEdge(t *testing.T) {
	t0 := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	s := []event.Session{{ID: "a", Kind: event.MainSession, Turns: []event.Turn{{Time: t0}, {Time: t0.Add(5 * time.Hour), Cost: event.Cost{Output: 30}}}}}
	reset := t0.Add(72*time.Hour).Unix() + 29 // 1 s before a bucket edge in the old rule; within 8 days of the readings
	readings := []LimitReading{
		{At: t0.Add(time.Minute), SevenDay: &LimitWindow{Used: 10, ResetsAt: reset}},
		{At: t0.Add(5*time.Hour + time.Minute), SevenDay: &LimitWindow{Used: 16, ResetsAt: reset + 2}}, // same window, 2 s later
	}
	if r := FindLimits(s, readings, 1, 1).SevenDay; r.Windows != 1 || r.Rise != 6 {
		t.Errorf("reset times 2 s apart: got %+v, want one window rising 6", r)
	}
}

// Readings stored unchecked by 0.1.0 (a reset time in the year 9999 or in
// milliseconds, or long before the reading) would join into one window
// spanning the genuine ones; they are left out of the rate.
func TestFindLimitsIgnoresImplausibleResets(t *testing.T) {
	t0 := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	var turns []event.Turn
	for h := range 300 { // $1 an hour for 300 hours
		turns = append(turns, event.Turn{Time: t0.Add(time.Duration(h) * time.Hour), Cost: event.Cost{Output: 1}})
	}
	s := []event.Session{{ID: "a", Kind: event.MainSession, Turns: turns}}
	week1, week2 := t0.Add(5*24*time.Hour).Unix(), t0.Add(12*24*time.Hour).Unix()
	read := func(h int, used float64, reset int64) LimitReading {
		return LimitReading{At: t0.Add(time.Duration(h)*time.Hour + time.Minute), SevenDay: &LimitWindow{Used: used, ResetsAt: reset}}
	}
	genuine := []LimitReading{
		read(10, 10, week1), read(40, 19, week1), // +9 over $30
		read(130, 2, week2), read(150, 8, week2), // +6 over $20
	}
	y9999 := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC).Unix()
	stored := append([]LimitReading{
		read(5, 1, y9999), read(20, 3, y9999), read(140, 60, y9999), read(200, 99, y9999), // one bogus window over both
		read(30, 5, week1*1000), read(35, 50, week1*1000), // milliseconds
		read(100, 0, t0.Unix()), read(120, 40, t0.Unix()), // reset days before the reading
	}, genuine...)
	sort.Slice(stored, func(i, j int) bool { return stored[i].At.Before(stored[j].At) })

	want := FindLimits(s, genuine, 20, 5).SevenDay
	if want.Windows != 2 || want.PerDollar != 0.3 {
		t.Fatalf("genuine windows alone: got %+v, want 2 windows at 0.3 a dollar", want)
	}
	if got := FindLimits(s, stored, 20, 5).SevenDay; got != want {
		t.Errorf("with implausible readings: got %+v, want %+v", got, want)
	}

	// The bounds: a day before the reading, eight days after.
	at := t0
	for _, c := range []struct {
		reset int64
		ok    bool
	}{
		{at.Add(-ResetsAtPast).Unix(), true},
		{at.Add(-ResetsAtPast).Unix() - 1, false},
		{at.Add(ResetsAtFuture).Unix(), true},
		{at.Add(ResetsAtFuture).Unix() + 1, false},
		{y9999, false},
	} {
		if got := PlausibleReset(c.reset, at); got != c.ok {
			t.Errorf("PlausibleReset(%d): got %v, want %v", c.reset, got, c.ok)
		}
	}
}
