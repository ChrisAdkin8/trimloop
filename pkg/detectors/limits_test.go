package detectors

import (
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
	reset := int64(1_800_000_029) // 1 s before a bucket edge in the old rule
	readings := []LimitReading{
		{At: t0.Add(time.Minute), SevenDay: &LimitWindow{Used: 10, ResetsAt: reset}},
		{At: t0.Add(5*time.Hour + time.Minute), SevenDay: &LimitWindow{Used: 16, ResetsAt: reset + 2}}, // same window, 2 s later
	}
	if r := FindLimits(s, readings, 1, 1).SevenDay; r.Windows != 1 || r.Rise != 6 {
		t.Errorf("reset times 2 s apart: got %+v, want one window rising 6", r)
	}
}
