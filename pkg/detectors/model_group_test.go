package detectors

import (
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

func TestModelGroup(t *testing.T) {
	for in, want := range map[string]string{
		"claude-opus-5":                            "claude-opus-5",
		"claude-opus-5@20260101":                   "claude-opus-5",
		"claude-opus-5-20260101":                   "claude-opus-5",
		"us.anthropic.claude-opus-5-v1:0":          "claude-opus-5",
		"us.anthropic.claude-fable-5-1-v1:0":       "claude-fable-5-1",
		"claude-fable-5-1":                         "claude-fable-5-1",
		"claude-haiku-4-5-20251001":                "claude-haiku-4-5", // a dated key: same price as the dateless one
		"anthropic.claude-haiku-4-5-20251001-v1:0": "claude-haiku-4-5",
		"new-model":                                "new-model", // unknown: as written
		"new-model@20260101":                       "new-model@20260101",
		"":                                         "",
	} {
		if got := modelGroup(in); got != want {
			t.Errorf("modelGroup(%q) = %q, want %q", in, got, want)
		}
	}
}

// Turns on one model logged under plain and decorated ids are one group in
// every by-model finding.
func TestDecoratedModelIDsGroupAsOne(t *testing.T) {
	u := event.Usage{CacheRead: 1_000_000, Output: 10_000}
	at := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	turn := func(model, effort string) event.Turn {
		tr := pricedTurn(model, u, 0)
		tr.Effort, tr.Time = effort, at
		return tr
	}
	cases := []struct {
		name  string
		ids   []string
		group string
	}{
		{"opus plain and Vertex", []string{"claude-opus-5", "claude-opus-5@20260101"}, "claude-opus-5"},
		{"opus plain, dated and Bedrock", []string{"claude-opus-5", "claude-opus-5-20260101", "us.anthropic.claude-opus-5-v1:0"}, "claude-opus-5"},
		{"fable Bedrock and plain", []string{"us.anthropic.claude-fable-5-1-v1:0", "claude-fable-5-1"}, "claude-fable-5-1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var main, sub, xhigh []event.Turn
			for _, id := range c.ids {
				main = append(main, turn(id, ""))
				sub = append(sub, turn(id, ""))
				xhigh = append(xhigh, turn(id, "xhigh"))
			}
			main = append(main, event.Turn{Model: "new-model", Time: at}) // unpriced: left out as before
			sessions := []event.Session{
				{ID: "m", Kind: event.MainSession, Turns: main},
				{ID: "m/a", Kind: event.SubAgentSession, ParentID: "m", AgentType: "Explore", Turns: sub},
			}

			mm := FindMainModels(sessions)
			if len(mm.ByModel) != 1 || mm.ByModel[0].Model != c.group || mm.ByModel[0].Turns != len(c.ids) {
				t.Errorf("main models: got %+v, want one row for %s with %d turns", mm.ByModel, c.group, len(c.ids))
			}

			sa := FindSubAgents(sessions, 200_000)
			if len(sa.ByType) != 1 || sa.ByType[0].Model != c.group {
				t.Errorf("sub-agents: got %+v, want model %s", sa.ByType, c.group)
			}
			if alt, _ := price.Cheaper(c.group); sa.ByType[0].Alternative != alt {
				t.Errorf("sub-agent alternative: got %q, want %q", sa.ByType[0].Alternative, alt)
			}

			ef := FindEffort([]event.Session{{ID: "e", Kind: event.MainSession, Turns: xhigh}})
			if len(ef.TopModels) != 1 || ef.TopModels[0] != c.group {
				t.Errorf("effort top models: got %v, want [%s]", ef.TopModels, c.group)
			}

			// The harness names the model one way, the logs another.
			rc := FindReconciliation([]event.Session{{ID: "r", Kind: event.MainSession, Turns: main[:len(c.ids)],
				Harness: &event.HarnessRecord{Cost: 100, At: at, ByModel: map[string]event.ModelRecord{c.ids[len(c.ids)-1]: {Cost: 100}}}}})
			if len(rc.ByModel) != 1 || rc.ByModel[0].Model != c.group || rc.ByModel[0].Harness != 100 || !near(rc.ByModel[0].Ours, rc.Ours) {
				t.Errorf("reconciliation: got %+v, want one row for %s", rc.ByModel, c.group)
			}
		})
	}
}
