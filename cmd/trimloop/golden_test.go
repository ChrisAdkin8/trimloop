package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"testing"
	"time"
	_ "time/tzdata" // the zones below, wherever the test runs

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source/claudecode"
	"github.com/ChrisAdkin8/trimloop/internal/report"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// goldenDir holds synthetic Claude Code logs (scripts/gen-golden.py) and the
// figures an independent implementation, scripts/recount.py, works out from
// them (expected.json). See testdata/golden/README.md.
var goldenDir = filepath.Join("..", "..", "testdata", "golden")

// tolerance is how far a dollar figure may differ from the recount's, in
// dollars: the two sides add the same products in different orders, so they
// agree to about 1e-9. Counts must match exactly.
const tolerance = 1e-6

// goldenZones are the time zones the test runs the report in. Weeks are
// calendar weeks in the report's zone (the CLI uses local time), so the
// recount works them out in each (expected.json "zones").
var goldenZones = []string{"UTC", "America/Los_Angeles", "Asia/Kolkata", "Pacific/Auckland"}

// tally is a count of turns and their cost.
type tally struct {
	Turns int     `json:"turns"`
	Cost  float64 `json:"cost"`
}

// rates are one tier of a model's prices in the recount's table, in US
// dollars per million tokens.
type rates struct {
	Input        float64 `json:"input"`
	CacheWrite5m float64 `json:"cache_write_5m"`
	CacheWrite1h float64 `json:"cache_write_1h"`
	CacheRead    float64 `json:"cache_read"`
	Output       float64 `json:"output"`
}

// modelRates is one model in the recount's price table.
type modelRates struct {
	Rates     rates   `json:"rates"`
	LongAbove int     `json:"long_above"`
	Long      *rates  `json:"long"`
	Fast      float64 `json:"fast"`
	USOnly    float64 `json:"us_only"`
}

// zoneFigures are the figures that depend on the time zone.
type zoneFigures struct {
	Weeks map[string]struct {
		Spend       float64 `json:"spend"`
		Turns       int     `json:"turns"`
		Compactions int     `json:"compactions"`
	} `json:"weeks"`
	Period struct {
		Days    int     `json:"days"`
		PerWeek float64 `json:"per_week"`
	} `json:"period"`
}

// golden is expected.json: the recount's figures.
type golden struct {
	Rates       map[string]modelRates  `json:"rates"`
	Zones       map[string]zoneFigures `json:"zones"`
	ReadRaw     map[string]int         `json:"read"`
	Tokens      map[string]int         `json:"tokens"`
	Cost        map[string]float64     `json:"cost"`
	Total       float64                `json:"total"`
	MainByModel map[string]tally       `json:"main_by_model"`
	SubAgents   struct {
		Runs   int     `json:"runs"`
		Turns  int     `json:"turns"`
		Cost   float64 `json:"cost"`
		ByType map[string]struct {
			Runs  int     `json:"runs"`
			Turns int     `json:"turns"`
			Cost  float64 `json:"cost"`
			Model string  `json:"model"`
		} `json:"by_type"`
	} `json:"sub_agents"`
	Sessions    map[string]tally `json:"sessions"`
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
	if math.Abs(got-want) > tolerance {
		f.t.Errorf("%s: trimloop $%.9f, recount $%.9f (differs by $%.9f)", name, got, want, got-want)
	}
}

func (f figures) text(name, got, want string) {
	f.t.Helper()
	if got != want {
		f.t.Errorf("%s: trimloop %q, recount %q", name, got, want)
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

// TestGolden runs trimloop on the golden logs and compares every figure
// with expected.json, which scripts/recount.py worked out independently, in
// each of goldenZones. A failure names the figure: decide from
// docs/measurements.md which side is right before changing either
// (testdata/golden/README.md).
func TestGolden(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(goldenDir, "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want golden
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if !t.Run("prices", func(t *testing.T) { comparePrices(t, want.Rates) }) {
		return // every cost would differ: fix the price table first
	}
	figures{t}.keys("zones", mapKeys(want.Zones), append([]string(nil), goldenZones...))
	for _, zone := range goldenZones {
		t.Run(zone, func(t *testing.T) {
			loc, err := time.LoadLocation(zone)
			if err != nil {
				t.Fatal(err)
			}
			compareReport(t, want, want.Zones[zone], loc)
		})
	}
}

// comparePrices checks the recount's price table against pkg/price's
// catalogue, model by model, so that a price change in either is noticed
// even when the version string is not changed.
func comparePrices(t *testing.T, want map[string]modelRates) {
	const fix = "update RATES, LONG, FAST or NO_US_SURCHARGE in scripts/recount.py to match pkg/price " +
		"and regenerate expected.json (testdata/golden/README.md)"
	tier := func(r price.Rates) rates {
		return rates{r.Input, r.CacheWrite5m, r.CacheWrite1h, r.CacheRead, r.Output}
	}
	cat := price.Catalogue()
	for _, id := range slices.Sorted(maps.Keys(cat)) {
		m := cat[id]
		w, ok := want[id]
		if !ok {
			t.Errorf("%s is in the catalogue but not in the recount's price table: %s", id, fix)
			continue
		}
		var long *rates
		if m.LongAbove > 0 {
			l := tier(m.Long)
			long = &l
		}
		got := modelRates{Rates: tier(m.Rates), LongAbove: m.LongAbove, Long: long, Fast: m.Fast, USOnly: m.USOnly}
		if !reflect.DeepEqual(got, w) {
			t.Errorf("%s: the catalogue has %s, the recount %s: %s", id, describe(got), describe(w), fix)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(want)) {
		if _, ok := cat[id]; !ok {
			t.Errorf("%s is in the recount's price table but not in the catalogue: %s", id, fix)
		}
	}
}

func describe(m modelRates) string {
	s := fmt.Sprintf("%+v, fast x%g, US-only x%g", m.Rates, m.Fast, m.USOnly)
	if m.Long != nil {
		s += fmt.Sprintf(", above %d tokens %+v", m.LongAbove, *m.Long)
	}
	return s
}

// compareReport runs the report in time zone loc, as the CLI does in local
// time, and compares its figures with the recount's.
func compareReport(t *testing.T, want golden, zone zoneFigures, loc *time.Location) {
	sessions, read := goldenSessions(t)
	settings := report.DefaultSettings()
	settings.Location = loc   // weeks are calendar weeks in this zone, as in the recount
	settings.TopGroups = 1000 // every project, not only the largest ten
	r := report.Analyse(sessions, read, settings, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC))
	f := figures{t}

	f.count("read.logs", r.Read.Logs, want.ReadRaw["logs"])
	f.count("read.bad_lines", r.Read.BadLines, want.ReadRaw["bad_lines"])
	f.count("read.sessions", r.Read.Sessions, want.ReadRaw["sessions"])
	f.count("read.turns", r.Read.Turns, want.ReadRaw["turns"])
	f.count("read.unpriced", r.Read.Unpriced, want.ReadRaw["unpriced"])

	// Tokens and spend per session are not in the report: add them up from
	// the sessions the report was given.
	var tok event.Usage
	perSession := map[string]tally{}
	for _, s := range sessions {
		for _, tn := range s.Turns {
			tok = tok.Add(tn.Usage)
			x := perSession[s.ID]
			x.Turns++
			x.Cost += tn.Cost.Total()
			perSession[s.ID] = x
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
	compareTallies("sessions", perSession, want.Sessions)

	// P13's tables, as trimloop reports them: main-session spend by model
	// and sub-agent spend by agent type, with the model that cost each
	// type most. Both group a model's ids under its catalogue name.
	mainByModel := map[string]tally{}
	for _, m := range r.MainModels.ByModel {
		mainByModel[m.Model] = tally{Turns: m.Turns, Cost: m.Cost}
	}
	compareTallies("main_by_model", mainByModel, want.MainByModel)
	f.count("sub_agents.runs", r.SubAgents.Runs, want.SubAgents.Runs)
	f.count("sub_agents.turns", r.SubAgents.Turns, want.SubAgents.Turns)
	f.dollars("sub_agents.cost", r.SubAgents.Cost, want.SubAgents.Cost)
	types := map[string]bool{}
	for _, g := range r.SubAgents.ByType {
		types[g.Type] = true
		e := want.SubAgents.ByType[g.Type]
		name := "sub_agents.by_type." + g.Type
		f.count(name+".runs", g.Runs, e.Runs)
		f.count(name+".turns", g.Turns, e.Turns)
		f.dollars(name+".cost", g.Cost, e.Cost)
		f.text(name+".model", g.Model, e.Model)
	}
	f.keys("sub_agents.by_type", mapKeys(types), mapKeys(want.SubAgents.ByType))

	weeks := map[string]bool{}
	for _, w := range r.Weeks {
		weeks[w.Week] = true
		e := zone.Weeks[w.Week]
		f.dollars("weeks."+w.Week+".spend", w.Spend, e.Spend)
		f.count("weeks."+w.Week+".turns", w.Turns, e.Turns)
		f.count("weeks."+w.Week+".compactions", w.Compactions, e.Compactions)
	}
	f.keys("weeks", mapKeys(weeks), mapKeys(zone.Weeks))
	f.count("period.days", r.Period.Days, zone.Period.Days)
	f.dollars("period.per_week", r.Period.PerWeek, zone.Period.PerWeek)

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
