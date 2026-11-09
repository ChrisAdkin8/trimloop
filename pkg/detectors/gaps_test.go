package detectors

import (
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// gturn is a priced Opus or Sonnet turn reading ctx tokens and writing 1,000
// for the hour.
func gturn(model string, at time.Time, idle time.Duration, read, write int) event.Turn {
	t := event.Turn{Model: model, Time: at, IdleBefore: idle, Priced: true,
		Usage: event.Usage{CacheRead: read, CacheWrite1h: write, Output: 100}}
	t.Cost.CacheRead = float64(read) * 0.2 / 1e6
	t.Cost.CacheWrite1h = float64(write) * 8 / 1e6
	return t
}

func TestFindModelSwitches(t *testing.T) {
	t0 := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	opus, sonnet := "claude-opus-5-5", "claude-sonnet-5-5"
	s := []event.Session{
		{ID: "a", Kind: event.MainSession, Turns: []event.Turn{
			gturn(opus, t0, 0, 99_000, 1000),
			gturn(sonnet, t0.Add(time.Minute), time.Minute, 0, 101_000), // a switch: wrote it all again
		}},
		{ID: "b", Kind: event.MainSession, Turns: []event.Turn{
			gturn(opus, t0, 0, 99_000, 1000),
			gturn(sonnet, t0.Add(2*time.Hour), 2*time.Hour, 0, 101_000), // cache had expired anyway
		}},
		{ID: "c", Kind: event.MainSession, Turns: []event.Turn{
			gturn(opus, t0, 0, 9_000, 1000),
			gturn(sonnet, t0.Add(time.Minute), time.Minute, 0, 11_000), // too small to count
		}},
	}
	compacted := gturn(sonnet, t0.Add(time.Minute), time.Minute, 0, 101_000)
	compacted.CompactedBefore = true
	s = append(s, event.Session{ID: "d", Kind: event.MainSession, Turns: []event.Turn{gturn(opus, t0, 0, 99_000, 1000), compacted}})
	f := FindModelSwitches(s, 3)
	if f.Switches != 1 || f.Sessions != 1 || f.Saving <= 0 || f.Saving >= f.Cost || f.Top[0].SessionID != "a" {
		t.Errorf("got %+v; want one switch, in session a, saving part of its cost", f)
	}
}

func TestFindFailures(t *testing.T) {
	s := []event.Session{{ID: "a", Failures: []event.Failure{
		{Kind: "sleep", Context: 200_000, Model: "claude-opus-5-5"},
		{Kind: "sleep", Context: 300_000, Model: "claude-opus-5-5"},
		{Kind: "network", Context: 100_000, Model: "claude-opus-5-5"},
		{Kind: "other"}, // before any turn: nothing re-sent
	}}, {ID: "b"}}
	f := FindFailures(s)
	if f.Sleep != 2 || f.Network != 1 || f.Other != 1 || f.Sessions != 1 {
		t.Errorf("counts: got %+v", f)
	}
	// Opus 5.5 reads at $0.20 a million: 500k tokens is $0.10, 600k all.
	if f.SleepResent < 0.0999 || f.SleepResent > 0.1001 || f.Resent < 0.1199 || f.Resent > 0.1201 || f.MedianContext != 300_000 {
		t.Errorf("costs: got %+v", f)
	}
}

func TestFindWhen(t *testing.T) {
	mon := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC) // a Monday
	sat := time.Date(2026, 11, 7, 20, 0, 0, 0, time.UTC)
	priced := func(at time.Time, c float64) event.Turn {
		return event.Turn{Time: at, Priced: true, Cost: event.Cost{Output: c}}
	}
	s := []event.Session{
		{ID: "a", Kind: event.MainSession, Turns: []event.Turn{priced(mon, 1), priced(mon.Add(2*time.Hour), 1)}},
		{ID: "b", Kind: event.MainSession, Turns: []event.Turn{priced(mon.Add(time.Hour), 1), priced(mon.Add(3*time.Hour), 1)}},
		{ID: "c", Kind: event.MainSession, Turns: []event.Turn{priced(mon.Add(3*time.Hour), 1), priced(mon.Add(4*time.Hour), 1)}}, // starts as b ends
		{ID: "d", Kind: event.MainSession, Turns: []event.Turn{priced(sat, 4)}},
	}
	w := FindWhen(s, time.UTC)
	if w.ByWeekday[0] != 6 || w.ByWeekday[5] != 4 || w.WeekendShare != 0.4 || w.ByHour[20] != 4 || w.MaxConcurrent != 2 {
		t.Errorf("got %+v; want Monday 6, Saturday 4, weekends 40%%, two at once", w)
	}
}

func TestThinkingAndStartCost(t *testing.T) {
	think := 300
	s := []event.Session{
		{ID: "a", Kind: event.MainSession, Turns: []event.Turn{
			{Priced: true, Usage: event.Usage{Output: 1000, Reasoning: &think}, Cost: event.Cost{Output: 1, CacheWrite1h: 0.05}},
			{Priced: true, Usage: event.Usage{Output: 1000}, Cost: event.Cost{Output: 1}},
		}},
		{ID: "b", Kind: event.MainSession, Turns: []event.Turn{{Priced: true, Cost: event.Cost{CacheWrite1h: 0.10}}}},
		{ID: "c", Kind: event.MainSession, Turns: []event.Turn{{Priced: true, Cost: event.Cost{CacheWrite1h: 0.20}}}},
		{ID: "c/x", Kind: event.SubAgentSession, Turns: []event.Turn{{Priced: true, Cost: event.Cost{CacheWrite1h: 9}}}},
	}
	th, out := ThinkingCost(s)
	if th != 0.3 || out != 2 {
		t.Errorf("thinking: got %v of %v, want 0.3 of 2", th, out)
	}
	if got := StartCost(s); got < 0.199 || got > 0.201 { // first turns 1.05, 0.10, 0.20; sub-agents left out
		t.Errorf("start cost: got %v, want the median 0.20", got)
	}
}

func TestUnusedAppServers(t *testing.T) {
	f := StandingContext{Parts: []ContextPart{
		{Name: "mcp__claude-artifacts", Source: SourceHarness, Cost: 10},
		{Name: "mcp__Claude_Browser", Source: SourceHarness, Cost: 7},
		{Name: "mcp__ccd_session", Source: SourceHarness, Cost: 2},
		{Name: "mcp__github", Source: SourceMCP, Cost: 5}, // the user's own: the "remove" card's
		{Name: "Built-in tool definitions", Source: SourceHarness, Cost: 100},
	}}
	s := []event.Session{{Turns: []event.Turn{{ToolCalls: []event.ToolCall{{Tool: "mcp__Claude_Browser__navigate"}}}}}}
	got := UnusedAppServers(f, s)
	if len(got) != 2 || got[0].Name != "mcp__claude-artifacts" || got[1].Name != "mcp__ccd_session" {
		t.Errorf("got %+v; want artifacts then ccd_session, costliest first", got)
	}
}

func TestFindToolInputs(t *testing.T) {
	call := func(tool string, bytes int) event.ToolCall { return event.ToolCall{Tool: tool, InputBytes: bytes} }
	read := func(ctx int) event.Turn {
		return event.Turn{Model: "claude-opus-5-5", Priced: true, Usage: event.Usage{CacheRead: ctx}, Cost: event.Cost{CacheRead: float64(ctx) * 0.2 / 1e6}}
	}
	t1 := read(10_000)
	t1.ToolCalls = []event.ToolCall{call("Bash", 4000), call("Write", 8000)}
	compacted := read(5_000)
	compacted.CompactedBefore = true
	s := []event.Session{{ID: "a", Turns: []event.Turn{t1, read(13_000), read(13_500), compacted, read(6_000)}}}
	f := FindToolInputs(s, 5)
	// 3,000 tokens, read on two turns at $0.20 a million, then cleared.
	if f.Calls != 2 || f.Tokens != 3000 || f.Carried < 0.00119 || f.Carried > 0.00121 || f.ByTool[0].Name != "Write" {
		t.Errorf("got %+v; want 2 calls, 3,000 tokens carried twice, Write first", f)
	}
}
