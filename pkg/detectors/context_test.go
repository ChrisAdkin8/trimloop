package detectors

import (
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

func TestFindReconciliation(t *testing.T) {
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	turn := func(model string, cost float64, when time.Time) event.Turn {
		return event.Turn{Model: model, Cost: event.Cost{Output: cost}, Priced: true, Time: when}
	}
	record := func(cost float64, written time.Time, searches int) *event.HarnessRecord {
		return &event.HarnessRecord{Cost: cost, At: written, ByModel: map[string]event.ModelRecord{
			"opus": {Cost: cost - 1}, "haiku": {Cost: 1, InputUncached: 500, WebSearches: searches}}}
	}
	sessions := []event.Session{
		// Complete: the record covers the main session and its sub-agent.
		{ID: "a", Kind: "main", Harness: record(12, at.Add(time.Hour), 100), Turns: []event.Turn{turn("opus", 8, at)}},
		{ID: "a/x", Kind: "subagent", ParentID: "a", Turns: []event.Turn{turn("opus", 2, at)}},
		// Stale: work went on long after the record.
		{ID: "b", Kind: "main", Harness: record(1, at, 0), Turns: []event.Turn{turn("opus", 9, at.Add(time.Hour))}},
		// No record.
		{ID: "c", Kind: "main", Turns: []event.Turn{turn("opus", 4, at)}},
		// A record whose earliest run began after the session did: partial.
		{ID: "e", Kind: "main", Harness: &event.HarnessRecord{Cost: 2, At: at.Add(time.Hour), Start: at.Add(30 * time.Minute)},
			Turns: []event.Turn{turn("opus", 9, at), turn("opus", 1, at.Add(time.Hour))}},
		// A turn with no price: the logs would show it as free.
		{ID: "d", Kind: "main", Harness: record(30, at.Add(time.Hour), 0), Turns: []event.Turn{{Model: "new-model", Time: at}}},
		// Sub-agents whose main session's log is gone: no record.
		{ID: "gone/x", Kind: "subagent", ParentID: "gone", Turns: []event.Turn{turn("opus", 3, at)}},
		// The harness could not price some of its usage: its total is short.
		{ID: "u", Kind: "main", Harness: &event.HarnessRecord{Cost: 1, At: at.Add(time.Hour), UnknownCost: true},
			Turns: []event.Turn{turn("opus", 5, at)}},
	}
	f := FindReconciliation(sessions)
	if f.Sessions != 1 || f.Harness != 12 || f.Ours != 10 || f.Gap != 2 || f.Unpriced != 2 {
		t.Errorf("compared: got %+v", f)
	}
	if f.Behind != 2 || f.BehindCost != 19 || f.Unrecorded != 2 || f.UnrecordedCost != 7 {
		t.Errorf("left out: got behind %d ($%v), unrecorded %d ($%v)", f.Behind, f.BehindCost, f.Unrecorded, f.UnrecordedCost)
	}
	if f.WebSearches != 100 || !near(f.WebSearchCost, 1) {
		t.Errorf("web searches: got %d ($%v)", f.WebSearches, f.WebSearchCost)
	}
	if len(f.ByModel) != 2 || f.ByModel[0].Model != "haiku" || f.ByModel[0].Gap != 1 || f.ByModel[0].HarnessUncached != 500 {
		t.Errorf("by model: got %+v", f.ByModel)
	}
	if want := (SessionGap{ID: "a", Harness: 12, Ours: 10, Gap: 2, SubAgentLogs: 1}); len(f.LargestGaps) != 1 || f.LargestGaps[0] != want {
		t.Errorf("largest gaps: got %+v, want %+v", f.LargestGaps, want)
	}
}

// The sessions with the largest gaps: compared sessions only, largest
// first, ties by id, at most three, with their sub-agent logs counted.
func TestReconciliationLargestGaps(t *testing.T) {
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	main := func(id string, recorded, logged float64) event.Session {
		return event.Session{ID: id, Kind: event.MainSession, Harness: &event.HarnessRecord{Cost: recorded, At: at},
			Turns: []event.Turn{{Model: "opus", Cost: event.Cost{Output: logged}, Priced: true, Time: at}}}
	}
	sub := func(id, parent string) event.Session {
		return event.Session{ID: id, Kind: event.SubAgentSession, ParentID: parent,
			Turns: []event.Turn{{Model: "opus", Priced: true, Time: at}}}
	}
	behind := main("behind", 100, 1)
	behind.Turns[0].Time = at.Add(time.Hour)
	unpriced := main("unpriced", 100, 1)
	unpriced.Turns[0].Priced = false
	f := FindReconciliation([]event.Session{
		main("small", 3, 2), main("tie-b", 12, 2), main("tie-a", 12, 2), main("big", 50, 10),
		sub("big/1", "big"), sub("big/2", "big"), sub("tie-a/1", "tie-a"),
		main("even", 5, 5), main("over", 1, 4), main("rounding", 2.004, 2), behind, unpriced,
		{ID: "unrecorded", Kind: event.MainSession, Turns: []event.Turn{{Cost: event.Cost{Output: 1}, Priced: true, Time: at}}},
	})
	want := []SessionGap{
		{ID: "big", Harness: 50, Ours: 10, Gap: 40, SubAgentLogs: 2},
		{ID: "tie-a", Harness: 12, Ours: 2, Gap: 10, SubAgentLogs: 1},
		{ID: "tie-b", Harness: 12, Ours: 2, Gap: 10},
	}
	if len(f.LargestGaps) != len(want) {
		t.Fatalf("got %+v, want %+v", f.LargestGaps, want)
	}
	for i := range want {
		if f.LargestGaps[i] != want[i] {
			t.Errorf("row %d: got %+v, want %+v", i, f.LargestGaps[i], want[i])
		}
	}
	if held, ok := f.Concentrated(); held != 60 || !ok {
		t.Errorf("concentrated: got %v %v, want 60 true", held, ok)
	}
}

// The record's cached tokens against the logs': only a clear margin over
// the logs says requests are missing from them.
func TestMissingCachedWork(t *testing.T) {
	for _, c := range []struct {
		harnessRead, harnessWrite, oursRead, oursWrite int
		want                                           bool
	}{
		{110, 0, 100, 0, false}, // 10 % more: within the margin
		{100, 11, 100, 0, true}, // 11 % more, counting writes
		{147, 0, 88, 7, true},
		{5000, 0, 0, 0, false}, // the logs hold no cached work: background only
		{90, 0, 100, 0, false},
	} {
		g := ModelGap{HarnessCacheRead: c.harnessRead, HarnessCacheWrite: c.harnessWrite, OursCacheRead: c.oursRead, OursCacheWrite: c.oursWrite}
		if got := g.MissingCachedWork(); got != c.want {
			t.Errorf("%+v: got %v, want %v", c, got, c.want)
		}
	}
}

func TestWeekOf(t *testing.T) {
	for in, want := range map[string]string{
		"2026-11-02": "2026-11-02", // Monday
		"2026-11-08": "2026-11-02", // Sunday
		"2026-11-04": "2026-11-02",
	} {
		d, _ := time.Parse("2006-01-02", in)
		if got := WeekOf(d.Add(15 * time.Hour)); got != want {
			t.Errorf("WeekOf(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestFindWeekly(t *testing.T) {
	mon := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	big := event.Turn{Time: mon, Usage: event.Usage{CacheRead: 300_000}, Cost: event.Cost{CacheRead: 3}, Priced: true}
	paused := event.Turn{Time: mon.Add(8 * 24 * time.Hour), IdleBefore: 2 * time.Hour, Usage: event.Usage{CacheWrite1h: 10},
		Cost: event.Cost{CacheWrite1h: 5}, Priced: true, CompactedBefore: true, Prompted: true}
	sessions := []event.Session{
		{ID: "m", Kind: "main", Turns: []event.Turn{big, paused, {}}}, // the last has no time
		{ID: "m/a", Kind: "subagent", ParentID: "m", Turns: []event.Turn{{Time: mon, Cost: event.Cost{Output: 1}, Priced: true, First: true}}},
	}
	weeks := FindWeekly(sessions, 200_000, time.Hour, time.UTC)
	if len(weeks) != 2 || weeks[0].Week != "2026-11-02" || weeks[1].Week != "2026-11-09" {
		t.Fatalf("got %+v", weeks)
	}
	w := weeks[0]
	if w.Turns != 2 || w.Spend != 4 || w.LargeContext != 3 || w.SubAgents != 1 {
		t.Errorf("first week: got %+v", w)
	}
	if w2 := weeks[1]; w2.PauseRewrite != 5 || w2.Compactions != 1 {
		t.Errorf("second week: got %+v", w2)
	}
	// A compaction after the last reply counts in that reply's week.
	sessions[0].Unreplied = []event.Compaction{{Before: 300_000, After: 20_000}}
	sessions[0].Turns = sessions[0].Turns[:2]
	if w2 := FindWeekly(sessions, 200_000, time.Hour, time.UTC)[1]; w2.Compactions != 2 {
		t.Errorf("trailing compaction: got %d compactions, want 2", w2.Compactions)
	}
}

func TestFindStandingContext(t *testing.T) {
	st := &event.Standing{SystemBytes: 4000, Tools: []event.ToolDef{{Name: "Bash", Bytes: 4000}, {Name: "mcp__gh__issue", Bytes: 4000}}}
	// Reads $1 per million tokens, writes $10 per million.
	turn := func(injected ...event.Injected) event.Turn {
		return event.Turn{Usage: event.Usage{CacheRead: 1_000_000, CacheWrite1h: 1_000_000},
			Cost: event.Cost{CacheRead: 1, CacheWrite1h: 10}, Priced: true, Standing: st, Injected: injected}
	}
	skills := event.Injected{Kind: "skill_listing", Bytes: 4_000_000} // 1M tokens
	compacted := turn()
	compacted.CompactedBefore = true
	s := event.Session{Turns: []event.Turn{turn(skills), turn(), compacted}}
	f := FindStandingContext([]event.Session{s})

	// 1,000 tokens each of system prompt, built-in and MCP tools: written on
	// the first turn ($0.01 each), read on the next two ($0.001 each).
	if f.Covered != 3 || !near(f.SystemPrompt, 0.012) || !near(f.ToolDefinitions, 0.024) {
		t.Errorf("standing: got %+v", f)
	}
	// The skill list: written on arrival ($10), read once ($1), dropped at
	// the compaction.
	if !near(f.Injected, 11) {
		t.Errorf("injected: got %v, want 11", f.Injected)
	}
	if f.Parts[0].Name != "skill_listing" || f.Parts[0].Source != SourceSkills || f.Parts[0].Tokens != 1_000_000 {
		t.Errorf("largest part: got %+v", f.Parts[0])
	}
	sources := map[string]string{}
	for _, p := range f.Parts {
		sources[p.Name] = p.Source
	}
	if sources["mcp__gh"] != SourceMCP || sources["Built-in tool definitions"] != SourceHarness || sources["System prompt"] != SourceHarness {
		t.Errorf("sources: got %v", sources)
	}
}

// A second record of the same system prompt and tools is not a change.
func TestStandingRecordedAgainIsNotRewritten(t *testing.T) {
	a := &event.Standing{SystemBytes: 4_000_000}
	b := &event.Standing{SystemBytes: 4_000_000} // same content, new record
	turn := func(st *event.Standing) event.Turn {
		return event.Turn{Usage: event.Usage{CacheRead: 1_000_000, CacheWrite1h: 1_000_000},
			Cost: event.Cost{CacheRead: 1, CacheWrite1h: 10}, Priced: true, Standing: st}
	}
	f := FindStandingContext([]event.Session{{Turns: []event.Turn{turn(a), turn(b)}}})
	if !near(f.SystemPrompt, 11) { // written once ($10), then read ($1)
		t.Errorf("got %v, want 11", f.SystemPrompt)
	}
	if sameStanding(a, &event.Standing{SystemBytes: 1}) || !sameStanding(nil, nil) || sameStanding(a, nil) {
		t.Error("sameStanding is wrong")
	}
}

func TestStandingSources(t *testing.T) {
	for group, want := range map[string]string{
		"mcp__gh":                       SourceMCP,
		"mcp__plugin_engineering_slack": SourceSkills,
		"mcp__ccd_session":              SourceHarness,
		"mcp__Claude_Browser":           SourceHarness,
		"mcp__visualize":                SourceHarness,
		"Built-in tool definitions":     SourceHarness,
	} {
		if got := mcpSource(group); got != want {
			t.Errorf("mcpSource(%q) = %q, want %q", group, got, want)
		}
	}
	for server, want := range map[string]string{
		"plugin:terraform:terraform": SourceSkills,
		"claude-artifacts":           SourceHarness,
		"claude.ai Claude Docs":      SourceMCP,
	} {
		if got := serverSource(server); got != want {
			t.Errorf("serverSource(%q) = %q, want %q", server, got, want)
		}
	}
	for kind, want := range map[string]string{
		"skill_listing":  SourceSkills,
		"invoked_skills": SourceHarness,
		"memory_file":    SourceMemory,
	} {
		if got := injectedSource(kind); got != want {
			t.Errorf("injectedSource(%q) = %q, want %q", kind, got, want)
		}
	}
}
