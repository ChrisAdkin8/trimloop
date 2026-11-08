package detectors

import (
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

func pricedTurn(model string, u event.Usage, idle time.Duration) event.Turn {
	c, _ := price.Cost(model, u, price.Options{})
	return event.Turn{Model: model, Usage: u, Cost: c, Priced: true, IdleBefore: idle}
}

// A sub-agent on the 5-minute cache that waits 20 minutes saves its whole
// history again; the hour's cache would have read it.
func TestFindCacheLife(t *testing.T) {
	m := "claude-opus-5-5"
	sub := event.Session{ID: "m/a", Kind: "subagent", Turns: []event.Turn{
		pricedTurn(m, event.Usage{CacheWrite5m: 100_000, Output: 100}, 0),
		pricedTurn(m, event.Usage{CacheRead: 100_000, CacheWrite5m: 2_000, Output: 100}, time.Minute), // warm
		pricedTurn(m, event.Usage{CacheWrite5m: 103_000, Output: 100}, 20*time.Minute),                // expired
		pricedTurn(m, event.Usage{CacheRead: 103_000, CacheWrite5m: 1_000, Output: 100}, 2*time.Hour), // read: not cold
		pricedTurn(m, event.Usage{CacheWrite5m: 105_000, Output: 100}, 2*time.Hour),                   // over an hour: P09's
	}}
	f := FindCacheLife([]event.Session{sub}, 5)
	if f.Waits != 1 || f.SubAgent != 1 || f.Sessions != 1 {
		t.Fatalf("got %+v, want one wait", f)
	}
	// Actual: 103k written at $5/M = $0.515; on the hour: 102k read at $0.20/M
	// and 1k written at $8/M = $0.0284. Output is the same either way.
	if want := 0.515 - 0.0284; !near(f.Saving, want) {
		t.Errorf("saving: got %v, want %v", f.Saving, want)
	}

	// The hour against 5 minutes: a warm turn after 30 minutes would have
	// been written again on the 5-minute cache.
	main := event.Session{ID: "m", Kind: "main", Turns: []event.Turn{
		pricedTurn(m, event.Usage{CacheWrite1h: 100_000}, 0),
		pricedTurn(m, event.Usage{CacheRead: 100_000, CacheWrite1h: 1_000}, 30*time.Minute),
	}}
	g := FindCacheLife([]event.Session{main}, 5)
	if g.Waits != 0 || g.HourAsShort <= g.HourActual {
		t.Errorf("hour comparison: got %+v, want the hour cheaper", g)
	}
}

func TestFindMainModels(t *testing.T) {
	u := event.Usage{CacheRead: 1_000_000, Output: 10_000}
	sessions := []event.Session{
		{Kind: "main", Turns: []event.Turn{pricedTurn("claude-opus-5", u, 0), pricedTurn("claude-fable-5-1", u, 0)}},
		{Kind: "subagent", Turns: []event.Turn{pricedTurn("claude-opus-5", u, 0)}}, // P13's sub-agent finding covers it
	}
	f := FindMainModels(sessions)
	// Opus 5 → 5.5: reads $0.50 → $0.20, output $0.25 → $0.20: $0.35 less.
	if !near(f.Newer, 0.35) || f.NewerTurns != 1 {
		t.Errorf("newer: got %v over %d turns", f.Newer, f.NewerTurns)
	}
	// Fable 5.1 → Opus 5.5: reads $0.25 → $0.20, output $0.50 → $0.20: $0.35 less.
	if !near(f.LowerTier, 0.35) || f.TierTurns != 1 || len(f.ByModel) != 2 {
		t.Errorf("tier: got %+v", f)
	}
}

func TestFindEffort(t *testing.T) {
	m := "claude-opus-5-5"
	turn := func(effort string, out int) event.Turn {
		x := pricedTurn(m, event.Usage{Output: out}, 0)
		x.Effort = effort
		return x
	}
	s := event.Session{Turns: []event.Turn{turn("high", 1000), turn("high", 3000), turn("max", 5000), turn("xhigh", 1000), turn("medium", 500)}}
	f := FindEffort([]event.Session{s})
	// High's mean is 2,000: the max turn's 3,000 extra at $20/M is $0.06; the
	// xhigh turn is under it.
	if f.HighMean != 2000 || f.Turns != 2 || !near(f.Saving, 0.06) {
		t.Errorf("got %+v", f)
	}
	if len(f.ByLevel) != 4 || f.ByLevel[0].Level != "medium" || f.ByLevel[3].Level != "max" {
		t.Errorf("levels: got %+v", f.ByLevel)
	}
}
