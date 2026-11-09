package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source/claudecode"
	"github.com/ChrisAdkin8/trimloop/internal/report"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// goldenDir holds synthetic Claude Code logs (scripts/gen-golden.py) and the
// figures an independent implementation, scripts/recount.py, works out from
// them (expected.json). See testdata/golden/README.md.
var goldenDir = filepath.Join("..", "..", "testdata", "golden")

// centTolerance is how far a dollar figure may differ from the recount's:
// half a cent. Counts must match exactly.
const centTolerance = 0.005

// tally is a count of turns and their cost.
type tally struct {
	Turns int     `json:"turns"`
	Cost  float64 `json:"cost"`
}

// golden is expected.json: the recount's figures.
type golden struct {
	TZ          string             `json:"tz"`
	Prices      string             `json:"prices"`
	ReadRaw     map[string]int     `json:"read"`
	Tokens      map[string]int     `json:"tokens"`
	Cost        map[string]float64 `json:"cost"`
	Total       float64            `json:"total"`
	ByModel     map[string]tally   `json:"by_model"`
	MainByModel map[string]tally   `json:"main_by_model"`
	SubAgents   struct {
		Runs    int              `json:"runs"`
		Turns   int              `json:"turns"`
		Cost    float64          `json:"cost"`
		ByModel map[string]tally `json:"by_model"`
	} `json:"sub_agents"`
	Sessions map[string]tally `json:"sessions"`
	Weeks    map[string]struct {
		Spend       float64 `json:"spend"`
		Turns       int     `json:"turns"`
		Compactions int     `json:"compactions"`
	} `json:"weeks"`
	Period struct {
		Days    int     `json:"days"`
		PerWeek float64 `json:"per_week"`
	} `json:"period"`
	Compactions struct {
		Total  int `json:"total"`
		Manual int `json:"manual"`
	} `json:"compactions"`
	P19 struct {
		Turns    int     `json:"turns"`
		Sessions int     `json:"sessions"`
		Cost     float64 `json:"cost"`
		Saving   float64 `json:"saving"`
	} `json:"p19"`
	Reconciliation struct {
		Sessions       int     `json:"sessions"`
		Harness        float64 `json:"harness"`
		Ours           float64 `json:"ours"`
		Gap            float64 `json:"gap"`
		Behind         int     `json:"behind"`
		BehindCost     float64 `json:"behind_cost"`
		Unrecorded     int     `json:"unrecorded"`
		UnrecordedCost float64 `json:"unrecorded_cost"`
		Unpriced       int     `json:"unpriced"`
	} `json:"reconciliation"`
	Projects map[string]struct {
		Cost          float64 `json:"cost"`
		Turns         int     `json:"turns"`
		Conversations int     `json:"conversations"`
		SubAgentCost  float64 `json:"sub_agent_cost"`
	} `json:"projects"`
}

// goldenSessions reads the golden logs as analyse does: it finds and reads
// them, then builds sessions with the same buildSessions, without the
// history file or anything from the home folder.
func goldenSessions(t *testing.T) ([]event.Session, report.Read) {
	t.Helper()
	claude := claudecode.Reader{ConfigDir: goldenDir}
	logs, err := claude.Discover()
	if err != nil {
		t.Fatal(err)
	}
	read := report.Read{Logs: len(logs)}
	data := make([]logData, 0, len(logs))
	for _, l := range logs {
		d := readLog(claude, l)
		if d.err != nil {
			t.Fatalf("%s: %v", l.Path, d.err)
		}
		read.BadLines += d.bad
		data = append(data, d)
	}
	return buildSessions(data), read
}

// figures compares trimloop's figures with the recount's and names each one
// that differs.
type figures struct {
	t *testing.T
}

func (f figures) count(name string, got, want int) {
	f.t.Helper()
	if got != want {
		f.t.Errorf("%s: trimloop %d, recount %d", name, got, want)
	}
}

func (f figures) dollars(name string, got, want float64) {
	f.t.Helper()
	if math.Abs(got-want) > centTolerance {
		f.t.Errorf("%s: trimloop $%.4f, recount $%.4f (differs by $%.4f)", name, got, want, got-want)
	}
}

// keys checks that both sides list the same names under a figure.
func (f figures) keys(name string, got, want []string) {
	f.t.Helper()
	sort.Strings(got)
	sort.Strings(want)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		f.t.Errorf("%s: trimloop lists %q, recount %q", name, got, want)
	}
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// modelKey groups a model id as the catalogue resolves it; an unknown id
// stays as written.
func modelKey(model string) string {
	if k, ok := price.Key(model); ok {
		return k
	}
	return model
}

// TestGolden runs trimloop on the golden logs and compares every figure
// with expected.json, which scripts/recount.py worked out independently. A
// failure names the figure: decide from docs/measurements.md which side is
// right before changing either (testdata/golden/README.md).
func TestGolden(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(goldenDir, "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want golden
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if want.TZ != "UTC" {
		t.Fatalf("expected.json was made with --tz %s; the test runs in UTC", want.TZ)
	}
	if want.Prices != price.Version {
		t.Fatalf("expected.json was priced with %s, the catalogue is now %s: update RATES in scripts/recount.py "+
			"and regenerate expected.json (testdata/golden/README.md)", want.Prices, price.Version)
	}

	sessions, read := goldenSessions(t)
	settings := report.DefaultSettings()
	settings.Location = time.UTC // weeks are calendar weeks in UTC, as in the recount
	settings.TopGroups = 1000    // every project, not only the largest ten
	r := report.Analyse(sessions, read, settings, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC))
	f := figures{t}

	f.count("read.logs", r.Read.Logs, want.ReadRaw["logs"])
	f.count("read.bad_lines", r.Read.BadLines, want.ReadRaw["bad_lines"])
	f.count("read.sessions", r.Read.Sessions, want.ReadRaw["sessions"])
	f.count("read.turns", r.Read.Turns, want.ReadRaw["turns"])
	f.count("read.unpriced", r.Read.Unpriced, want.ReadRaw["unpriced"])

	var tok event.Usage
	byModel, subByModel := map[string]tally{}, map[string]tally{}
	perSession := map[string]tally{}
	for _, s := range sessions {
		for _, tn := range s.Turns {
			tok = tok.Add(tn.Usage)
			add := func(m map[string]tally, k string) {
				x := m[k]
				x.Turns++
				x.Cost += tn.Cost.Total()
				m[k] = x
			}
			add(byModel, modelKey(tn.Model))
			add(perSession, s.ID)
			if s.Kind == event.SubAgentSession {
				add(subByModel, modelKey(tn.Model))
			}
		}
	}
	f.count("tokens.input_uncached", tok.InputUncached, want.Tokens["input_uncached"])
	f.count("tokens.cache_read", tok.CacheRead, want.Tokens["cache_read"])
	f.count("tokens.cache_write_5m", tok.CacheWrite5m, want.Tokens["cache_write_5m"])
	f.count("tokens.cache_write_1h", tok.CacheWrite1h, want.Tokens["cache_write_1h"])
	f.count("tokens.output", tok.Output, want.Tokens["output"])

	f.dollars("cost.input_uncached", r.Cost.InputUncached, want.Cost["input_uncached"])
	f.dollars("cost.cache_read", r.Cost.CacheRead, want.Cost["cache_read"])
	f.dollars("cost.cache_write_5m", r.Cost.CacheWrite5m, want.Cost["cache_write_5m"])
	f.dollars("cost.cache_write_1h", r.Cost.CacheWrite1h, want.Cost["cache_write_1h"])
	f.dollars("cost.output", r.Cost.Output, want.Cost["output"])
	f.dollars("total", r.Total, want.Total)

	compareTallies := func(name string, got, want map[string]tally) {
		t.Helper()
		f.keys(name, mapKeys(got), mapKeys(want))
		for k, w := range want {
			g := got[k]
			f.count(name+"."+k+".turns", g.Turns, w.Turns)
			f.dollars(name+"."+k+".cost", g.Cost, w.Cost)
		}
	}
	compareTallies("by_model", byModel, want.ByModel)
	compareTallies("sessions", perSession, want.Sessions)
	mainByModel := map[string]tally{}
	for _, m := range r.MainModels.ByModel {
		mainByModel[m.Model] = tally{Turns: m.Turns, Cost: m.Cost}
	}
	compareTallies("main_by_model", mainByModel, want.MainByModel)
	f.count("sub_agents.runs", r.SubAgents.Runs, want.SubAgents.Runs)
	f.count("sub_agents.turns", r.SubAgents.Turns, want.SubAgents.Turns)
	f.dollars("sub_agents.cost", r.SubAgents.Cost, want.SubAgents.Cost)
	compareTallies("sub_agents.by_model", subByModel, want.SubAgents.ByModel)

	weeks := map[string]bool{}
	for _, w := range r.Weeks {
		weeks[w.Week] = true
		e := want.Weeks[w.Week]
		f.dollars("weeks."+w.Week+".spend", w.Spend, e.Spend)
		f.count("weeks."+w.Week+".turns", w.Turns, e.Turns)
		f.count("weeks."+w.Week+".compactions", w.Compactions, e.Compactions)
	}
	f.keys("weeks", mapKeys(weeks), mapKeys(want.Weeks))
	f.count("period.days", r.Period.Days, want.Period.Days)
	f.dollars("period.per_week", r.Period.PerWeek, want.Period.PerWeek)

	f.count("compactions.total", r.Growth.Compactions, want.Compactions.Total)
	f.count("compactions.manual", r.Growth.Manual, want.Compactions.Manual)

	f.count("p19.turns", r.Context.Turns, want.P19.Turns)
	f.count("p19.sessions", r.Context.Sessions, want.P19.Sessions)
	f.dollars("p19.cost", r.Context.Cost, want.P19.Cost)
	f.dollars("p19.saving", r.Context.Saving, want.P19.Saving)

	rc, wr := r.Reconciliation, want.Reconciliation
	f.count("reconciliation.sessions", rc.Sessions, wr.Sessions)
	f.dollars("reconciliation.harness", rc.Harness, wr.Harness)
	f.dollars("reconciliation.ours", rc.Ours, wr.Ours)
	f.dollars("reconciliation.gap", rc.Gap, wr.Gap)
	f.count("reconciliation.behind", rc.Behind, wr.Behind)
	f.dollars("reconciliation.behind_cost", rc.BehindCost, wr.BehindCost)
	f.count("reconciliation.unrecorded", rc.Unrecorded, wr.Unrecorded)
	f.dollars("reconciliation.unrecorded_cost", rc.UnrecordedCost, wr.UnrecordedCost)
	f.count("reconciliation.unpriced", rc.Unpriced, wr.Unpriced)

	projects := map[string]bool{}
	for _, p := range r.Attribution.ByProject {
		projects[p.Name] = true
		e := want.Projects[p.Name]
		f.dollars("projects."+p.Name+".cost", p.Cost, e.Cost)
		f.count("projects."+p.Name+".turns", p.Turns, e.Turns)
		f.count("projects."+p.Name+".conversations", p.Conversations, e.Conversations)
		f.dollars("projects."+p.Name+".sub_agent_cost", p.SubAgentCost, e.SubAgentCost)
	}
	f.keys("projects", mapKeys(projects), mapKeys(want.Projects))
}
