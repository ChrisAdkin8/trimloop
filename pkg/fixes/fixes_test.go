package fixes

import (
	"strings"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
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
	if _, ok := ForCompaction(detectors.Growth{UncompactedCost: 10}, 200_000, 0.5, 0.2, 30_000, 1); ok {
		t.Error("a saving under the minimum should give no recommendation")
	}
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567"} {
		if got := thousandsInt(n); got != want {
			t.Errorf("thousandsInt(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestForPauses(t *testing.T) {
	r, ok := ForPauses(detectors.LongPause{Pauses: 4, Cost: 20, Saving: 15, Unprompted: 1, UnpromptedCost: 5}, 15, 30_000, 0.06, 1)
	if !ok || r.Pattern != "P09" || r.Saving != 15 || r.Likely != 15 || !strings.Contains(r.LikelyNote, "30,000") ||
		!strings.Contains(r.Finding, "3 times") || !strings.Contains(r.Finding, "Another long wait was") {
		t.Errorf("got %+v ok=%v", r, ok)
	}
	if _, ok := ForPauses(detectors.LongPause{Saving: 0.5}, 0, 0, 0, 1); ok {
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

func TestNewFindingCards(t *testing.T) {
	if _, ok := ForCacheLife(detectors.CacheLife{Saving: 0.5}, 1); ok {
		t.Error("P02 under the minimum should give no card")
	}
	if r, ok := ForCacheLife(detectors.CacheLife{Waits: 3, SubAgent: 2, Saving: 5, MainShort: 304, SubShort: 1, MainHour: 117}, 1); !ok || r.Pattern != "P02" ||
		!strings.Contains(r.Finding, "2 in sub-agents") || !strings.Contains(r.Finding, "304 main sessions and 1 sub-agent used the 5-minute cache") {
		t.Errorf("P02: got %+v", r)
	}
	if r, _ := ForCacheLife(detectors.CacheLife{Waits: 3, Saving: 5, MainShort: 4, SubShort: 2}, 1); strings.Contains(r.Primary.How, "main session") || len(r.Alternatives) != 0 {
		t.Errorf("P02 with no session on the hour: got %+v", r.Primary)
	}
	r, _ := ForEffort(detectors.Effort{Saving: 2, Turns: 5, Usual: "max", TopModels: []string{"claude-fable-5-1"}}, 1)
	if !strings.Contains(r.Primary.What, "come back to high") || strings.Contains(r.Finding, "The other") ||
		!strings.Contains(r.Primary.How, "ran on Fable 5.1") || !strings.Contains(r.Primary.How, "modelSettings") {
		t.Errorf("P14 when usually on max: got %+v", r)
	}
	r, _ = ForEffort(detectors.Effort{Saving: 2, Turns: 5, Unmatched: 2, Usual: "medium"}, 1)
	if !strings.Contains(r.Finding, "The other 2 ran on a model") {
		t.Errorf("P14 unmatched: got %q", r.Finding)
	}
	m := detectors.MainModels{Newer: 3, LowerTier: 4, NewerTurns: 2, TierTurns: 1, ByModel: []detectors.ModelSpend{
		{Model: "claude-opus-5", Newer: "claude-opus-5-5", NewerSave: 3},
		{Model: "claude-fable-5-1", Tier: "claude-opus-5-5", TierSave: 4},
	}}
	recs := ForMainModels(m, 1)
	if len(recs) != 2 || !strings.Contains(recs[0].Primary.What, "from Opus 5 to Opus 5.5") || recs[1].Check == "" {
		t.Errorf("P13 main: got %+v", recs)
	}
	if len(ForMainModels(m, 10)) != 0 {
		t.Error("P13 main under the minimum should give no card")
	}
	if r, ok := ForEffort(detectors.Effort{Saving: 2, Turns: 5, HighMean: 1000, Usual: "medium"}, 1); !ok || r.Pattern != "P14" ||
		!strings.Contains(r.Finding, "same model's high-effort turns") || !strings.Contains(r.Primary.How, "/effort medium") {
		t.Errorf("P14: got %+v", r)
	}
}

func TestForCacheLifeWhenOnlySubAgentsKeepTheHour(t *testing.T) {
	f := detectors.CacheLife{Saving: 50, Waits: 10, MainShort: 4, SubHour: 3}
	r, ok := ForCacheLife(f, 1)
	if !ok || !strings.Contains(r.Primary.What, "main session") || len(r.Alternatives) != 0 ||
		strings.Contains(r.Primary.How, "rather than a sub-agent") {
		t.Errorf("only sub-agents on the hour: got %+v", r)
	}
	f.MainHour = 2 // main sessions keep the hour: move waits there
	if r, _ := ForCacheLife(f, 1); !strings.Contains(r.Primary.What, "keeps its cache for an hour") || len(r.Alternatives) != 1 {
		t.Errorf("main sessions on the hour: got %+v", r)
	}
}

func TestForSubAgentsNamesTheModel(t *testing.T) {
	f := detectors.SubAgents{Cost: 100, Saving: 40, ByType: []detectors.AgentGroup{{Type: "Explore", Alternative: "claude-haiku-5-5", Saving: 40}}}
	r, ok := ForSubAgents(f, 1)
	if !ok || !strings.Contains(r.Primary.How, `"model: haiku"`) || !strings.Contains(r.Alternatives[0].How, "model: haiku;") {
		t.Errorf("haiku alternative: got %+v", r)
	}
	f.ByType[0].Alternative = "" // a saving with no named model
	if r, _ := ForSubAgents(f, 1); strings.Contains(r.Finding, "moved to .") || !strings.Contains(r.Finding, "a cheaper model") {
		t.Errorf("no named model: got %q", r.Finding)
	}
}

func TestForPausesNamesTheGap(t *testing.T) {
	for gap, want := range map[time.Duration]string{time.Hour: "more than an hour away", 2 * time.Hour: "more than 2 hours away",
		90 * time.Minute: "more than 90 minutes away", 0: "more than an hour away"} {
		r, ok := ForPauses(detectors.LongPause{MinGap: gap, Pauses: 3, Saving: 50}, 0, 30_000, 0, 1)
		if !ok || !strings.Contains(r.Finding, want) {
			t.Errorf("gap %v: got %q, want %q", gap, r.Finding, want)
		}
	}
}

func TestGapCards(t *testing.T) {
	if r, ok := ForModelSwitches(detectors.ModelSwitches{Switches: 3, Sessions: 2, Saving: 10}, 1); !ok || r.Pattern != "P01" ||
		!strings.Contains(r.Finding, "3 times, in 2 sessions") || !strings.Contains(r.Primary.How, "/compact first") {
		t.Errorf("model switches: got %+v", r)
	}
	if _, ok := ForModelSwitches(detectors.ModelSwitches{Saving: 0.5}, 1); ok {
		t.Error("model switches under the minimum: want no card")
	}
	f := detectors.Failures{Sleep: 2, SleepResent: 5, MedianContext: 259_000}
	if _, ok := ForFailures(f, 1); ok {
		t.Error("two sleeps: want no card")
	}
	f.Sleep = 57
	if r, ok := ForFailures(f, 1); !ok || r.Pattern != "P12" || !strings.Contains(r.Finding, "57 times") ||
		!strings.Contains(r.Finding, "259,000") || !strings.Contains(r.Primary.How, "caffeinate") {
		t.Errorf("sleep: got %+v", r)
	}
	parts := []detectors.ContextPart{{Name: "mcp__claude-artifacts", Cost: 10}, {Name: "mcp__ccd_session", Cost: 2}}
	if r, ok := ForAppServers(parts, 1); !ok || r.Pattern != "P06" || r.Saving != 12 ||
		!strings.Contains(r.Finding, "claude-artifacts, ccd_session") || !strings.Contains(r.Primary.What, "terminal") {
		t.Errorf("app servers: got %+v", r)
	}
	r, _ := ForPauses(detectors.LongPause{Pauses: 3, Saving: 50}, 0, 30_000, 0.06, 1)
	if !strings.Contains(r.Finding, "about $0.06 for its first turn") {
		t.Errorf("start cost: got %q", r.Finding)
	}
}

// One model under plain and decorated ids is named once in the P13 and P14
// cards: not "ran on Opus 5 and Opus 5".
func TestModelCardsNameEachModelOnce(t *testing.T) {
	u := event.Usage{CacheRead: 1_000_000, Output: 10_000}
	turn := func(model, effort string) event.Turn {
		c, _ := price.Cost(model, u, price.Options{})
		return event.Turn{Model: model, Usage: u, Cost: c, Priced: true, Effort: effort}
	}
	cases := []struct {
		name       string
		ids        []string
		card, want string
	}{
		{"opus plain and Vertex", []string{"claude-opus-5", "claude-opus-5@20260101"}, "Move to the newer model", "from Opus 5 to Opus 5.5"},
		{"opus dated and Bedrock", []string{"claude-opus-5-20260101", "us.anthropic.claude-opus-5-v1:0", "claude-opus-5"}, "Move to the newer model", "from Opus 5 to Opus 5.5"},
		{"fable Bedrock and plain", []string{"us.anthropic.claude-fable-5-1-v1:0", "claude-fable-5-1"}, "Use Opus 5.5 for routine main-session work", "ran on Fable 5.1."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var turns, xhigh []event.Turn
			for _, id := range c.ids {
				turns = append(turns, turn(id, ""))
				xhigh = append(xhigh, turn(id, "xhigh"))
			}
			recs := ForMainModels(detectors.FindMainModels([]event.Session{{ID: "m", Kind: event.MainSession, Turns: turns}}), 0.01)
			var found bool
			for _, r := range recs {
				text := r.Finding + " " + r.Primary.What
				if r.Title == c.card {
					found = true
					if strings.Count(text, c.want) != 1 {
						t.Errorf("%s: want %q once, got %q", r.Title, c.want, text)
					}
				}
				for _, name := range []string{"Opus 5", "Fable 5.1"} {
					if strings.Contains(text, name+" and "+name+" ") || strings.Contains(text, name+" and "+name+".") || len(r.Examples) != 1 {
						t.Errorf("%s: a name repeated: %q (examples %v)", r.Title, text, r.Examples)
					}
				}
			}
			if !found {
				t.Errorf("no %q card in %+v", c.card, recs)
			}

			e := detectors.FindEffort([]event.Session{{ID: "e", Kind: event.MainSession, Turns: xhigh}})
			r, _ := ForEffort(detectors.Effort{Saving: 2, Turns: 5, Usual: "medium", TopModels: e.TopModels}, 1)
			if want := "turns ran on " + price.DisplayName(c.ids[0]) + ". "; !strings.Contains(r.Primary.How, want) {
				t.Errorf("P14: want %q, got %q", want, r.Primary.How)
			}
		})
	}
}
