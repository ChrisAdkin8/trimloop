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
	if !strings.Contains(r.Primary.What, "claude-sonnet-5-5") || r.Check == "" {
		t.Errorf("fix: got %+v", r)
	}
	if _, ok := ForSubAgents(detectors.SubAgents{Saving: 0.5}, 1); ok {
		t.Error("a saving under the minimum should give no recommendation")
	}
}

func TestForCompaction(t *testing.T) {
	r, ok := ForCompaction(detectors.Growth{UncompactedCost: 10}, 200_000, 1)
	if !ok || r.Saving != 0 || !strings.Contains(r.Primary.What, "200,000") || !strings.Contains(r.Primary.How, "/compact") {
		t.Errorf("got %+v ok=%v", r, ok)
	}
	if _, ok := ForCompaction(detectors.Growth{UncompactedCost: 0.5}, 200_000, 1); ok {
		t.Error("a cost under the minimum should give no recommendation")
	}
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567"} {
		if got := thousandsInt(n); got != want {
			t.Errorf("thousandsInt(%d) = %q, want %q", n, got, want)
		}
	}
}
