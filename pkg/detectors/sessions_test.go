package detectors

import (
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// opusTurn is a priced Opus 5.5 turn reading context tokens from the cache.
func opusTurn(context int) event.Turn {
	u := event.Usage{CacheRead: context, CacheWrite1h: 1000, Output: 1000}
	c, _ := price.Cost("claude-opus-5-5", u, price.Options{})
	return event.Turn{Model: "claude-opus-5-5", Usage: u, Cost: c, Priced: true}
}

func TestFindSubAgents(t *testing.T) {
	sessions := []event.Session{
		{ID: "m", Kind: "main", Turns: []event.Turn{opusTurn(10_000)}},
		{ID: "m/a", Kind: "subagent", ParentID: "m", AgentType: "general-purpose",
			Turns: []event.Turn{opusTurn(100_000), opusTurn(300_000)}},
		{ID: "m/f", Kind: "subagent", ParentID: "m", AgentType: "fork", Turns: []event.Turn{opusTurn(10_000)}},
		{ID: "m/u", Kind: "subagent", ParentID: "m", Turns: []event.Turn{{Model: "unpriced"}}},
	}
	f := FindSubAgents(sessions, 200_000)

	gp := sessions[1].Turns
	wantCost := gp[0].Cost.Total() + gp[1].Cost.Total() + sessions[2].Turns[0].Cost.Total()
	if f.Runs != 3 || f.Turns != 4 || !near(f.Cost, wantCost) {
		t.Errorf("totals: got %d runs, %d turns, $%v; want 3, 4, $%v", f.Runs, f.Turns, f.Cost, wantCost)
	}
	// Sonnet 5.5 costs half of Opus 5.5 in every class, so the saving is
	// half the general-purpose spend; the fork is left out.
	if want := (gp[0].Cost.Total() + gp[1].Cost.Total()) / 2; !near(f.Saving, want) {
		t.Errorf("saving: got %v, want %v", f.Saving, want)
	}
	if f.Large != 1 || !near(f.LargeCost, gp[0].Cost.Total()+gp[1].Cost.Total()) {
		t.Errorf("large runs: got %d costing %v", f.Large, f.LargeCost)
	}
	if f.ByType[0].Type != "general-purpose" || f.ByType[0].Alternative != "claude-sonnet-5-5" {
		t.Errorf("first type: got %+v", f.ByType[0])
	}
	for _, g := range f.ByType {
		if g.Type == "fork" && (g.Alternative != "" || g.Saving != 0) {
			t.Errorf("fork: got %+v, want no suggestion", g)
		}
		if g.Type == "unknown" && g.Runs != 1 {
			t.Errorf("unknown: got %+v", g)
		}
	}
	if f.ReadsPerWrite <= 0 || f.MainReadsPerWrite != 10 || f.WriteShare <= 0 {
		t.Errorf("cache use: got %+v", f)
	}
}

func TestFindGrowth(t *testing.T) {
	turn := func(context int, cost float64, project string) event.Turn {
		return event.Turn{Usage: event.Usage{CacheRead: context}, Cost: event.Cost{CacheRead: cost}, Priced: true, Project: project}
	}
	compacted := turn(30_000, 1, "a")
	compacted.CompactedBefore = true
	compacted.Compaction = event.Compaction{Before: 600_000, After: 30_000, Manual: true, Duration: 90 * time.Second}

	sessions := []event.Session{
		{ID: "big", Kind: "main", Turns: []event.Turn{turn(49_999, 1, "a"), turn(600_000, 5, "b"), turn(700_000, 6, "b")}},
		{ID: "big/x", Kind: "subagent", ParentID: "big", Turns: []event.Turn{turn(50_000, 2, "b")}},
		{ID: "ok", Kind: "main", Turns: []event.Turn{turn(600_000, 3, "a"), compacted}},
		{ID: "small", Kind: "main", Turns: []event.Turn{turn(10_000, 2, "c")}},
	}
	f := FindGrowth(sessions, 500_000, 2)

	if f.Conversations != 3 || !near(f.Cost, 20) || !near(f.MedianCost, 4) {
		t.Errorf("totals: got %d conversations, $%v, median $%v", f.Conversations, f.Cost, f.MedianCost)
	}
	if len(f.Top) != 2 || f.Top[0].ID != "big" || f.Top[0].Project != "b" || f.Top[0].Turns != 4 || !near(f.Top[0].Cost, 14) {
		t.Errorf("top: got %+v", f.Top)
	}
	if len(f.TopShares) != 1 || f.TopShares[0].N != 1 || !near(f.TopShares[0].Share, 0.7) {
		t.Errorf("shares: got %+v (only N <= conversations)", f.TopShares)
	}
	wantBands := []int{3, 1, 0, 3} // <50k, 50k-200k, 200k-500k, over 500k
	for i, b := range f.Bands {
		if b.Turns != wantBands[i] {
			t.Errorf("band %d: got %d turns, want %d", i, b.Turns, wantBands[i])
		}
	}
	if !near(f.Bands[3].MeanCost(), 14.0/3) {
		t.Errorf("over-500k mean: got %v", f.Bands[3].MeanCost())
	}
	if f.Compactions != 1 || f.Manual != 1 || f.MedianBefore != 600_000 || f.MedianAfter != 30_000 || f.Longest != 90*time.Second {
		t.Errorf("compactions: got %+v", f)
	}
	if f.Uncompacted != 1 || !near(f.UncompactedCost, 12) {
		t.Errorf("uncompacted: got %d costing %v, want the main session 'big' at $12", f.Uncompacted, f.UncompactedCost)
	}
}

func TestFindAttribution(t *testing.T) {
	turn := func(cost float64, project string, a event.Attribution) event.Turn {
		return event.Turn{Cost: event.Cost{Output: cost}, Priced: true, Project: project, Attribution: a}
	}
	sessions := []event.Session{
		{ID: "m1", Kind: "main", Turns: []event.Turn{
			turn(5, "web", event.Attribution{Skill: "review", Plugin: "tools"}),
			turn(1, "", event.Attribution{MCPServer: "github"}),
		}},
		{ID: "m1/a", Kind: "subagent", ParentID: "m1", Turns: []event.Turn{turn(3, "web", event.Attribution{Agent: "general-purpose"})}},
		{ID: "m2", Kind: "main", Turns: []event.Turn{turn(2, "api", event.Attribution{Skill: "review"})}},
	}
	f := FindAttribution(sessions, 10)
	if !near(f.Cost, 11) {
		t.Errorf("cost: got %v", f.Cost)
	}
	web := f.ByProject[0]
	if web.Name != "web" || !near(web.Cost, 8) || web.Turns != 2 || web.Conversations != 1 || !near(web.SubAgentCost, 3) {
		t.Errorf("web: got %+v", web)
	}
	if len(f.ByProject) != 3 || f.ByProject[2].Name != "(unknown)" {
		t.Errorf("projects: got %+v", f.ByProject)
	}
	if len(f.BySkill) != 1 || f.BySkill[0].Conversations != 2 || !near(f.BySkill[0].Cost, 7) {
		t.Errorf("skills: got %+v", f.BySkill)
	}
	if len(f.ByPlugin) != 1 || len(f.ByMCPServer) != 1 || f.ByMCPServer[0].Name != "github" {
		t.Errorf("plugins %+v, MCP %+v", f.ByPlugin, f.ByMCPServer)
	}
	if got := FindAttribution(sessions, 1); len(got.ByProject) != 1 {
		t.Errorf("topN: got %d projects, want 1", len(got.ByProject))
	}
}
