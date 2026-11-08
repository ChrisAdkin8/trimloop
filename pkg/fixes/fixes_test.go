package fixes

import (
	"strings"
	"testing"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// TestLadders checks the ADR 0030 rules: one primary fix and at most two
// alternatives, all of different kinds, each with text.
func TestLadders(t *testing.T) {
	kinds := []event.NoiseKind{event.NoiseFileView, event.NoiseSearch, event.NoiseBuildTest,
		event.NoiseData, event.NoiseWeb, event.NoiseMCP, event.NoiseSubAgent, event.NoiseOther}
	for _, k := range kinds {
		l, ok := toolOutputFixes[k]
		if !ok {
			t.Errorf("no fixes for noise kind %q", k)
			continue
		}
		if l.title == "" || len(l.fixes) == 0 || len(l.fixes) > 3 {
			t.Errorf("%q: got title %q and %d fixes, want a title and 1 to 3", k, l.title, len(l.fixes))
		}
		seen := map[Kind]bool{}
		for _, f := range l.fixes {
			if seen[f.Kind] {
				t.Errorf("%q: two fixes of kind %s", k, f.Kind)
			}
			seen[f.Kind] = true
			if f.What == "" || f.How == "" {
				t.Errorf("%q: a %s fix has no text", k, f.Kind)
			}
			if f.Kind == Trim {
				t.Errorf("%q: trim fixes are L3 and not offered before M4", k)
			}
		}
	}
}

func TestForToolOutput(t *testing.T) {
	f := detectors.ToolOutput{ByNoise: []detectors.Group{
		{Noise: event.NoiseFileView, Saving: 40, Examples: []string{"sed -n"}},
		{Noise: event.NoiseData, Saving: 5},
		{Noise: "unknown", Saving: 4},           // no fixes for it: skipped
		{Noise: event.NoiseSearch, Saving: 0.5}, // under the minimum
		{Noise: event.NoiseWeb, Saving: 3},
	}}
	recs := ForToolOutput(f, 2, 1)
	if len(recs) != 2 {
		t.Fatalf("got %d recommendations, want 2", len(recs))
	}
	r := recs[0]
	if r.Pattern != "P04" || r.Noise != event.NoiseFileView || r.Saving != 40 || r.Examples[0] != "sed -n" {
		t.Errorf("first: got %+v", r)
	}
	if r.Primary.Kind != Advise || len(r.Alternatives) != 2 || !strings.Contains(r.Primary.How, "CLAUDE.md") {
		t.Errorf("first fix: got %+v", r)
	}
	if recs[1].Noise != event.NoiseData {
		t.Errorf("second: got %q, want data", recs[1].Noise)
	}
	if got := ForToolOutput(f, 10, 1); len(got) != 3 {
		t.Errorf("with room for 10: got %d, want 3 (file view, data, web)", len(got))
	}
}

func TestForSubAgents(t *testing.T) {
	f := detectors.SubAgents{Saving: 50, ByType: []detectors.AgentGroup{
		{Type: "fork", Saving: 0},
		{Type: "general-purpose", Saving: 50, Alternative: "claude-sonnet-5-5"},
	}}
	r, ok := ForSubAgents(f, 1)
	if !ok || r.Pattern != "P13" || r.Saving != 50 || len(r.Examples) != 1 || r.Examples[0] != "general-purpose" {
		t.Fatalf("got %+v ok=%v", r, ok)
	}
	if !strings.Contains(r.Primary.What, "Sonnet 5.5") || r.Check == "" || r.Expected() != 50 {
		t.Errorf("fix: got %+v", r)
	}
	if _, ok := ForSubAgents(detectors.SubAgents{Saving: 0.5}, 1); ok {
		t.Error("a saving under the minimum should give no recommendation")
	}
}

func TestForCompaction(t *testing.T) {
	r, ok := ForCompaction(detectors.Growth{UncompactedCost: 10}, 200_000, 7, 3, 30_000, 1)
	if !ok || r.Saving != 7 || r.Likely != 3 || r.Expected() != 7 || r.Overlap == "" || !strings.Contains(r.Primary.What, "200,000") || !strings.Contains(r.Primary.How, "/compact") {
		t.Errorf("got %+v ok=%v", r, ok)
	}
	if _, ok := ForCompaction(detectors.Growth{UncompactedCost: 0.5}, 200_000, 7, 3, 30_000, 1); ok {
		t.Error("a cost under the minimum should give no recommendation")
	}
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567"} {
		if got := thousandsInt(n); got != want {
			t.Errorf("thousandsInt(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestForPauses(t *testing.T) {
	r, ok := ForPauses(detectors.LongPause{Pauses: 3, Saving: 20}, 15, 30_000, 1)
	if !ok || r.Pattern != "P09" || r.Likely != 15 || !strings.Contains(r.LikelyNote, "30,000") || !strings.Contains(r.Finding, "3 times") {
		t.Errorf("got %+v ok=%v", r, ok)
	}
	if _, ok := ForPauses(detectors.LongPause{Saving: 0.5}, 0, 0, 1); ok {
		t.Error("a saving under the minimum should give no recommendation")
	}
	if dollars(1234.5) != "$1,235" {
		t.Errorf("dollars: got %q", dollars(1234.5))
	}
}

func TestToolOutputFinding(t *testing.T) {
	f := detectors.ToolOutput{Target: 2000, ByNoise: []detectors.Group{{Noise: event.NoiseFileView, Saving: 40, Carried: 100}}}
	r := ForToolOutput(f, 1, 1)[0]
	if r.Title != "Read only the lines you need" || !strings.Contains(r.Finding, "$100") || !strings.Contains(r.Finding, "2,000") {
		t.Errorf("got %+v", r)
	}
}

func TestForStandingContext(t *testing.T) {
	f := detectors.StandingContext{Parts: []detectors.ContextPart{
		{Name: "skill_listing", Source: detectors.SourceSkills, Cost: 30},
		{Name: "mcp__github", Source: detectors.SourceMCP, Cost: 8},
		{Name: "mcp_instructions_delta", Source: detectors.SourceMCP, Cost: 4},
		{Name: "System prompt", Source: detectors.SourceHarness, Cost: 50},
		{Name: "Project CLAUDE.md", Source: detectors.SourceMemory, Cost: 0.5},
	}}
	recs := ForStandingContext(f, 1)
	if len(recs) != 2 {
		t.Fatalf("got %d recommendations, want 2 (none for the harness's own, memory under the minimum)", len(recs))
	}
	if recs[0].Pattern != "P07" || recs[0].Saving != 30 || !strings.Contains(recs[0].Finding, "$30") {
		t.Errorf("first: got %+v", recs[0])
	}
	if recs[1].Saving != 12 || recs[1].Examples[0] != "github" {
		t.Errorf("MCP: got %+v", recs[1])
	}
}
