package detectors

import (
	"math"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// turn makes a priced turn with the given context (all cache reads, $1 per
// 1,000 tokens, so cost = context/1000), cache-write cost and idle time.
func turn(context int, writeCost float64, idle time.Duration, first bool) event.Turn {
	return event.Turn{
		Usage:      event.Usage{CacheRead: context},
		Cost:       event.Cost{CacheRead: float64(context) / 1000, CacheWrite1h: writeCost},
		IdleBefore: idle,
		First:      first,
		Priced:     true,
	}
}

func TestFindContextSize(t *testing.T) {
	sessions := []event.Session{
		{ID: "a", Turns: []event.Turn{
			turn(100_000, 0, 0, true),            // at threshold: not counted
			turn(150_000, 0, time.Second, false), // 50k over
			turn(300_000, 0, time.Second, false), // 200k over
		}},
		{ID: "b", Turns: []event.Turn{
			turn(200_000, 0, 0, true), // 100k over
		}},
		{ID: "c", Turns: []event.Turn{
			turn(500_000, 0, 0, true), // over, but not priced: skipped
		}},
	}
	sessions[2].Turns[0].Priced = false

	f := FindContextSize(sessions, 100_000, 10)

	if f.Turns != 3 || f.Sessions != 2 {
		t.Errorf("got %d turns in %d sessions, want 3 in 2", f.Turns, f.Sessions)
	}
	// Input cost of counted turns: 150 + 300 + 200.
	if !near(f.Cost, 650) {
		t.Errorf("cost: got %v, want 650", f.Cost)
	}
	// Saving = cost of tokens over 100k: 50 + 200 + 100.
	if !near(f.Saving, 350) {
		t.Errorf("saving: got %v, want 350", f.Saving)
	}
	if len(f.Top) != 2 || f.Top[0].SessionID != "a" || !near(f.Top[0].Saving, 250) || f.Top[0].Peak != 300_000 {
		t.Errorf("top: got %+v, want a first with saving 250 and peak 300000", f.Top)
	}
}

func TestFindLongPauses(t *testing.T) {
	sessions := []event.Session{
		{ID: "a", Turns: []event.Turn{
			turn(1000, 5, 0, true),                      // first turn: not a pause
			turn(1000, 0.1, 2*time.Hour, false),         // counted
			turn(1000, 3, time.Hour, false),             // exactly at gap: not counted
			turn(1000, 0, 3*time.Hour, false),           // no cache write: not counted
			turn(1000, 2, time.Hour+time.Second, false), // counted
		}},
		{ID: "b", Turns: []event.Turn{
			turn(9000, 7, 0, true),
			turn(9000, 7, 5*time.Hour, false), // counted
		}},
	}

	f := FindLongPauses(sessions, time.Hour, 1)

	if f.Pauses != 3 || f.Sessions != 2 {
		t.Errorf("got %d pauses in %d sessions, want 3 in 2", f.Pauses, f.Sessions)
	}
	if !near(f.Cost, 9.1) || !near(f.Saving, 9.1) {
		t.Errorf("cost/saving: got %v/%v, want 9.1", f.Cost, f.Saving)
	}
	if len(f.Top) != 1 || f.Top[0].SessionID != "b" || f.Top[0].Peak != 9000 {
		t.Errorf("top 1: got %+v, want b with peak 9000", f.Top)
	}
}

func TestTotalCost(t *testing.T) {
	got := TotalCost([]event.Session{
		{Turns: []event.Turn{turn(1000, 2, 0, true)}},
		{Turns: []event.Turn{turn(3000, 0, 0, true)}},
	})
	if !near(got.Total(), 6) {
		t.Errorf("got %v, want 6", got.Total())
	}
}
