package detectors

import (
	"testing"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// call makes a tool call with a result of the given tokens.
func call(tool, shape string, noise event.NoiseKind, tokens int) event.ToolCall {
	return event.ToolCall{ID: tool + shape, Tool: tool, Shape: shape, Noise: noise,
		Result: event.ToolResult{Bytes: tokens * event.BytesPerToken}, HasResult: true}
}

// priced makes a turn whose cache reads cost $1 per million tokens and whose
// cache writes cost $10 per million.
func priced(read, write int, calls ...event.ToolCall) event.Turn {
	return event.Turn{
		Usage:     event.Usage{CacheRead: read, CacheWrite1h: write},
		Cost:      event.Cost{CacheRead: float64(read) / 1e6, CacheWrite1h: float64(write) * 10 / 1e6},
		Priced:    true,
		ToolCalls: calls,
	}
}

func TestCarriedCost(t *testing.T) {
	c := call("Bash", "go test -v", event.NoiseBuildTest, 1_000_000)
	s := event.Session{Turns: []event.Turn{
		priced(10, 10, c), // calls the tool
		priced(10, 10),    // result arrives: written at $10 per million
		priced(10, 10),    // carried: read at $1 per million
		priced(0, 10),     // cache rebuilt (no reads): written again at $10
		priced(10, 0, c),  // read at $1; calls again, but no later turn sends it
	}}
	got := carry(s)
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1 (the last call is never sent)", len(got))
	}
	if want := 10.0 + 1 + 10 + 1; !near(got[0].carried, want) {
		t.Errorf("carried: got %v, want %v", got[0].carried, want)
	}
}

func TestCarriedCostStopsAtCompaction(t *testing.T) {
	a := call("Read", "", event.NoiseFileView, 1_000_000)
	b := call("Grep", "", event.NoiseSearch, 1_000_000)
	b.ID = "b"
	compacted := priced(10, 10)
	compacted.CompactedBefore = true
	s := event.Session{Turns: []event.Turn{
		priced(10, 10, a),
		priced(10, 10, b), // a arrives ($10)
		compacted,         // a dropped; b went into the compaction, never carried
		priced(10, 10),
	}}
	got := carry(s)
	if len(got) != 1 || !near(got[0].carried, 10) {
		t.Errorf("got %+v, want only a, carried $10", got)
	}
}

func TestRereads(t *testing.T) {
	view := func(id, file string) event.ToolCall {
		return event.ToolCall{ID: id, Tool: "Read", Noise: event.NoiseFileView, File: file, FileOp: event.FileView, HasResult: true}
	}
	edit := event.ToolCall{ID: "e", Tool: "Edit", File: "f", FileOp: event.FileEdit, HasResult: true}
	compacted := priced(1, 1, view("v5", "f"))
	compacted.CompactedBefore = true
	s := event.Session{Turns: []event.Turn{
		priced(1, 1, view("v1", "f"), view("v2", "g")),
		priced(1, 1, view("v3", "f")), // re-read of f
		priced(1, 1, edit),
		priced(1, 1, view("v4", "f")), // f changed: not a re-read
		compacted,                     // context dropped
		priced(1, 1),                  // v5 arrives after the compaction: not a re-read
	}}
	var rereads []string
	for _, cc := range carry(s) {
		if cc.reread {
			rereads = append(rereads, cc.call.ID)
		}
	}
	if len(rereads) != 1 || rereads[0] != "v3" {
		t.Errorf("re-reads: got %v, want [v3]", rereads)
	}
}

func TestFindToolOutput(t *testing.T) {
	sed := call("Bash", "sed -n", event.NoiseFileView, 3_000_000)
	sed.Filtered = true
	read := call("Read", "", event.NoiseFileView, 1_000_000)
	mcp := call("mcp__github__get_issue", "", event.NoiseMCP, 1_000_000)
	small := call("Bash", "ls", event.NoiseSearch, 1_000)
	s := event.Session{Turns: []event.Turn{
		priced(10, 10, sed, read, mcp, small),
		priced(10, 10), // every result arrives at $10 per million
	}}
	f := FindToolOutput([]event.Session{s}, 2_000_000, 10)

	if f.Calls != 4 || f.Tokens != 5_001_000 || !near(f.Carried, 50.01) {
		t.Errorf("totals: got %d calls, %d tokens, $%v", f.Calls, f.Tokens, f.Carried)
	}
	// Only sed is over 2M tokens: a third of its $30 is above the target.
	if !near(f.Saving, 10) {
		t.Errorf("saving: got %v, want 10", f.Saving)
	}
	if len(f.ByTool) != 3 || f.ByTool[0].Name != "Bash" || f.ByTool[2].Name != "mcp__github" {
		t.Errorf("by tool: got %+v", f.ByTool)
	}
	if len(f.ByShape) != 2 || f.ByShape[0].Name != "sed -n" || f.ByShape[0].Filtered != 1 {
		t.Errorf("by shape: got %+v", f.ByShape)
	}
	fv := f.ByNoise[0]
	if fv.Noise != event.NoiseFileView || fv.Calls != 2 || len(fv.Examples) != 2 || fv.Examples[0] != "sed -n" || fv.Examples[1] != "Read" {
		t.Errorf("by noise: got %+v", fv)
	}
}

func TestToolGroup(t *testing.T) {
	for in, want := range map[string]string{"mcp__github__get_issue": "mcp__github", "mcp__x": "mcp__x", "Bash": "Bash"} {
		if got := toolGroup(in); got != want {
			t.Errorf("toolGroup(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRatesWithoutCache(t *testing.T) {
	turn := event.Turn{Usage: event.Usage{InputUncached: 100}, Cost: event.Cost{InputUncached: 1}}
	if arrivalRate(turn) != 0.01 || historyRate(turn) != 0.01 {
		t.Errorf("got arrival %v history %v, want 0.01 each", arrivalRate(turn), historyRate(turn))
	}
	if arrivalRate(event.Turn{}) != 0 || historyRate(event.Turn{}) != 0 {
		t.Error("a turn with no input should have zero rates")
	}
}
