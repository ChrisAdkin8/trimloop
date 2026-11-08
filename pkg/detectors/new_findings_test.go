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
	// Actual: 103k written at $5/M = $0.515. With the cache still there:
	// 102k read at $0.20/M and 1k written at $5/M = $0.0254.
	if want := 0.515 - 0.0254; !near(f.Saving, want) {
		t.Errorf("saving: got %v, want %v", f.Saving, want)
	}
	// On the hour's cache the wait turn writes its 1k at $8/M ($0.0284), but
	// the session's other 208k of writes cost $3/M more ($0.624).
	if want := (0.515 - 0.0284) - 0.624; !near(f.HourNet, want) {
		t.Errorf("hour net: got %v, want %v", f.HourNet, want)
	}

	// The hour against 5 minutes: a warm turn after 30 minutes would have
	// been written again on the 5-minute cache.
	main := event.Session{ID: "m", Kind: "main", Turns: []event.Turn{
		pricedTurn(m, event.Usage{CacheWrite1h: 100_000}, 0),                                // $0.80; short: $0.50
		pricedTurn(m, event.Usage{CacheRead: 100_000, CacheWrite1h: 1_000}, 30*time.Minute), // $0.028; short: 101k written, $0.505
		pricedTurn(m, event.Usage{CacheRead: 101_000}, 7*time.Minute),                       // $0.0202; short: rewritten only past 5 min, $0.505
	}}
	g := FindCacheLife([]event.Session{main}, 5)
	if g.MainHour != 1 || g.SubShort != 0 {
		t.Errorf("lifetimes: got %+v", g)
	}
	if f := FindCacheLife([]event.Session{sub, main}, 5); f.SubShort != 1 || f.MainHour != 1 || f.MainShort != 0 {
		t.Errorf("lifetimes by kind: got %+v", f)
	}
	if g.Waits != 0 || !near(g.HourActual, 0.8482) || !near(g.HourAsShort, 1.51) || !near(g.HourAsShortLow, 1.0252) {
		t.Errorf("hour comparison: got actual %v, short %v (low %v); want 0.8482, 1.51, 1.0252",
			g.HourActual, g.HourAsShort, g.HourAsShortLow)
	}

	// A model change or a compaction is not a wait that lost the cache.
	changed := sub
	changed.Turns = append([]event.Turn(nil), sub.Turns...)
	changed.Turns[2].Model = "claude-sonnet-5-5"
	changed.Turns[2].Cost, _ = price.Cost("claude-sonnet-5-5", changed.Turns[2].Usage, price.Options{})
	if f := FindCacheLife([]event.Session{changed}, 5); f.Waits != 0 {
		t.Errorf("model change: got %d waits, want 0", f.Waits)
	}
	compacted := sub
	compacted.Turns = append([]event.Turn(nil), sub.Turns...)
	compacted.Turns[2].CompactedBefore = true
	if f := FindCacheLife([]event.Session{compacted}, 5); f.Waits != 0 {
		t.Errorf("compaction: got %d waits, want 0", f.Waits)
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
	s := event.Session{Turns: []event.Turn{turn("max", 5000), turn("xhigh", 1000), turn("medium", 500)}}
	for range 15 {
		s.Turns = append(s.Turns, turn("high", 1000), turn("high", 3000))
	}
	f := FindEffort(spread(s))
	// High's mean is 2,000. Max's mean is 5,000: 3,000 more over 1 turn at
	// $20/M is $0.06. Xhigh's mean is 1,000, under high's: nothing.
	if f.HighMean != 2000 || f.Turns != 2 || !near(f.Saving, 0.06) {
		t.Errorf("got %+v", f)
	}
	if f.Usual != "high" || len(f.ByLevel) != 4 || f.ByLevel[0].Level != "medium" || f.ByLevel[3].Level != "max" {
		t.Errorf("levels: got %+v", f.ByLevel)
	}
}

// The weekly figures for P02, P14 and main-session models add up to the
// findings.
func TestFindWeeklyNewFigures(t *testing.T) {
	mon := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	at := func(x event.Turn, d time.Duration) event.Turn { x.Time = mon.Add(d); return x }
	m := "claude-opus-5-5"
	sub := event.Session{ID: "m/a", Kind: "subagent", Turns: []event.Turn{
		at(pricedTurn(m, event.Usage{CacheWrite5m: 100_000}, 0), 0),
		at(pricedTurn(m, event.Usage{CacheWrite5m: 103_000}, 20*time.Minute), 20*time.Minute),
	}}
	maxTurn := at(pricedTurn("claude-opus-5", event.Usage{CacheRead: 1000, Output: 1000}, 0), time.Hour)
	maxTurn.Effort = "max"
	main := event.Session{ID: "m", Kind: "main", Turns: []event.Turn{maxTurn}}
	sessions := []event.Session{sub, main}
	w := FindWeekly(sessions, 200_000, time.Hour, time.UTC)
	if len(w) != 1 {
		t.Fatalf("got %+v", w)
	}
	if want := FindCacheLife(sessions, 1).Saving; !near(w[0].ShortWaits, want) || want == 0 {
		t.Errorf("short waits: got %v, want %v", w[0].ShortWaits, want)
	}
	if !near(w[0].TopEffort, maxTurn.Cost.Output) || !near(w[0].OlderModels, maxTurn.Cost.Total()) {
		t.Errorf("effort and models: got %+v", w[0])
	}
}

// When a level writes as much per turn as high on average, there is
// nothing to save, however much single turns vary.
func TestFindEffortUsesAverages(t *testing.T) {
	turn := func(effort string, out int) event.Turn {
		x := pricedTurn("claude-opus-5-5", event.Usage{Output: out}, 0)
		x.Effort = effort
		return x
	}
	s := event.Session{Turns: []event.Turn{turn("max", 1000), turn("max", 3000)}}
	for range 15 {
		s.Turns = append(s.Turns, turn("high", 1000), turn("high", 3000))
	}
	if f := FindEffort(spread(s)); f.Saving != 0 {
		t.Errorf("got %v, want 0: max writes as much as high on average", f.Saving)
	}
}

// Where turns record thinking tokens, P14 compares thinking, which is what
// the effort level controls, not all output.
func TestFindEffortUsesThinking(t *testing.T) {
	turn := func(effort string, out, think int) event.Turn {
		x := pricedTurn("claude-opus-5-5", event.Usage{Output: out, Reasoning: &think}, 0)
		x.Effort = effort
		return x
	}
	// High thinks 400 of 1,000; max thinks 2,400 of 3,000: 2,000 more per
	// turn at $20/M is $0.04, though its output is also 2,000 more.
	s := event.Session{Turns: []event.Turn{turn("max", 3000, 2400)}}
	for range 30 {
		s.Turns = append(s.Turns, turn("high", 1000, 400))
	}
	f := FindEffort(spread(s))
	if !f.Thinking || f.HighMean != 400 || !near(f.Saving, 0.04) {
		t.Errorf("got %+v", f)
	}
}

// Models think different amounts at the same level: each xhigh or max turn
// is compared with its own model's high turns, and a model with no high
// turns is left out.
func TestFindEffortWithinModel(t *testing.T) {
	turn := func(model, effort string, out, think int) event.Turn {
		x := pricedTurn(model, event.Usage{Output: out, Reasoning: &think}, 0)
		x.Effort = effort
		return x
	}
	s := event.Session{Turns: []event.Turn{
		turn("claude-fable-5-1", "xhigh", 2000, 1500), // thinks as much as Fable's high: nothing
		turn("claude-opus-5-5", "max", 2000, 1400),    // 1,000 more than Opus's high at $20/M: $0.02
		turn("claude-sonnet-5-5", "max", 2000, 1400),  // no Sonnet high turns: unmatched
		turn("claude-haiku-5-5", "max", 2000, 1400),   // one Haiku high turn: too few, unmatched
		turn("claude-haiku-5-5", "high", 2000, 100),
	}}
	for range 30 {
		s.Turns = append(s.Turns, turn("claude-opus-5-5", "high", 1000, 400), turn("claude-fable-5-1", "high", 2000, 1500))
	}
	f := FindEffort(spread(s))
	if !near(f.Saving, 0.02) || f.Turns != 4 || f.Unmatched != 2 || !f.Thinking {
		t.Errorf("got saving %v, %d turns, %d unmatched; want 0.02, 4, 2", f.Saving, f.Turns, f.Unmatched)
	}
}

// spread splits a session's turns over three sessions, round robin, so a
// high-effort baseline comes from enough sessions (minHighSessions).
func spread(s event.Session) []event.Session {
	out := []event.Session{{ID: "s1"}, {ID: "s2"}, {ID: "s3"}}
	for i, t := range s.Turns {
		out[i%3].Turns = append(out[i%3].Turns, t)
	}
	return out
}

// A baseline from one session is too easily swung: left out.
func TestFindEffortNeedsSeveralSessions(t *testing.T) {
	turn := func(effort string, out int) event.Turn {
		x := pricedTurn("claude-opus-5-5", event.Usage{Output: out}, 0)
		x.Effort = effort
		return x
	}
	s := event.Session{ID: "one", Turns: []event.Turn{turn("max", 5000)}}
	for range 40 {
		s.Turns = append(s.Turns, turn("high", 1000))
	}
	if f := FindEffort([]event.Session{s}); f.Unmatched != 1 || f.Saving != 0 || len(f.TopModels) != 1 {
		t.Errorf("got %+v, want the max turn unmatched", f)
	}
}

// A session that wrote with both lifetimes is counted as on the hour, and
// its waits do not count towards putting 5-minute sessions on the hour.
func TestFindCacheLifeMixedSession(t *testing.T) {
	m := "claude-opus-5-5"
	mixed := event.Session{ID: "x", Kind: "main", Turns: []event.Turn{
		pricedTurn(m, event.Usage{CacheWrite5m: 100_000}, 0),
		pricedTurn(m, event.Usage{CacheWrite5m: 101_000}, 6*time.Minute), // expired
		pricedTurn(m, event.Usage{CacheRead: 101_000, CacheWrite1h: 1_000}, time.Minute),
	}}
	f := FindCacheLife([]event.Session{mixed}, 5)
	if f.Waits != 1 || f.MainHour != 1 || f.MainShort != 0 || f.HourNet != 0 {
		t.Errorf("got waits %d, hour %d, short %d, net %v; want 1, 1, 0, 0", f.Waits, f.MainHour, f.MainShort, f.HourNet)
	}
}
