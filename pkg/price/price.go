// Package price holds the price catalogue and turns token usage into cost.
//
// The catalogue is a Go table for now. docs/data-model.md specifies a
// versioned YAML file, overridable for negotiated rates; this table is the
// data that file will hold.
package price

import (
	"strings"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// Version identifies the catalogue. Every cost figure should record it.
// Source: https://platform.claude.com/docs/en/about-claude/pricing, list
// prices read on 2026-11-07 and checked again on 2026-11-08, when the
// still-served 4.6-and-later models were added. Cache reads on Opus 5.5 and
// Sonnet 5.5 are 0.05x the input price and on Fable 5.1 0.025x (the page's
// footnotes), not the usual 0.1x: Claude Code's own cost records price
// Sonnet 5.5 reads at 0.1x, which shows up as reconciliation gap.
const Version = "anthropic-list-2026-11-08"

// Rates are prices in US dollars per million tokens.
type Rates struct {
	Input        float64 // uncached input
	CacheWrite5m float64
	CacheWrite1h float64
	CacheRead    float64 // cache hits and refreshes
	Output       float64
}

// Model is one model's prices.
type Model struct {
	Rates

	// LongAbove and Long give different prices for prompts with more than
	// LongAbove input tokens (cache reads and writes included). Zero means
	// no long-prompt tier.
	LongAbove int
	Long      Rates

	// Fast multiplies every rate in fast mode; zero means fast mode has no
	// published price for this model.
	Fast float64

	// USOnly multiplies every rate when inference is pinned to the US
	// (inference_geo "us"); zero means no surcharge.
	USOnly float64
}

var catalogue = map[string]Model{
	"claude-opus-5-5":   {Rates: Rates{4, 5, 8, 0.20, 20}, Fast: 2, USOnly: 1.1},
	"claude-opus-5":     {Rates: Rates{5, 6.25, 10, 0.50, 25}, Fast: 2, USOnly: 1.1},
	"claude-fable-5-1":  {Rates: Rates{10, 12.50, 20, 0.25, 50}, USOnly: 1.1},
	"claude-fable-5":    {Rates: Rates{10, 12.50, 20, 1, 50}, USOnly: 1.1},
	"claude-opus-4-8":   {Rates: Rates{5, 6.25, 10, 0.50, 25}, Fast: 2, USOnly: 1.1},
	"claude-opus-4-7":   {Rates: Rates{5, 6.25, 10, 0.50, 25}, USOnly: 1.1},
	"claude-opus-4-6":   {Rates: Rates{5, 6.25, 10, 0.50, 25}, USOnly: 1.1},
	"claude-sonnet-5-5": {Rates: Rates{2, 2.50, 4, 0.10, 10}, USOnly: 1.1},
	"claude-sonnet-5":   {Rates: Rates{2, 2.50, 4, 0.20, 10}, USOnly: 1.1},
	"claude-sonnet-4-6": {Rates: Rates{3, 3.75, 6, 0.30, 15}, USOnly: 1.1},
	"claude-haiku-5-5": {
		Rates:     Rates{0.10, 0.125, 0.20, 0.01, 0.50},
		LongAbove: 100_000,
		Long:      Rates{0.50, 0.625, 1, 0.05, 2.50},
		USOnly:    1.1,
	},
	"claude-haiku-4-5-20251001": {Rates: Rates{1, 1.25, 2, 0.10, 5}}, // before 4.6: no US-only surcharge
	"claude-haiku-4-5":          {Rates: Rates{1, 1.25, 2, 0.10, 5}},
}

// Options are the request settings that change the price.
type Options struct {
	Fast   bool // fast mode (usage "speed": "fast")
	USOnly bool // US-only inference (usage "inference_geo": "us")
}

// Cost prices one turn's usage. ok is false if the catalogue has no price for
// the model, or none for the options asked for. A turn with no tokens costs
// nothing whatever the model (Claude Code logs some "<synthetic>" messages).
func Cost(model string, u event.Usage, o Options) (c event.Cost, ok bool) {
	if u.InputTotal() == 0 && u.Output == 0 {
		return event.Cost{}, true
	}
	m, found := catalogue[model]
	if !found {
		return event.Cost{}, false
	}

	r := m.Rates
	if m.LongAbove > 0 && u.InputTotal() > m.LongAbove {
		r = m.Long
	}
	mult := 1.0
	if o.Fast {
		if m.Fast == 0 {
			return event.Cost{}, false
		}
		mult *= m.Fast
	}
	if o.USOnly && m.USOnly != 0 {
		mult *= m.USOnly
	}

	per := func(tokens int, rate float64) float64 { return float64(tokens) * rate * mult / 1e6 }
	return event.Cost{
		InputUncached: per(u.InputUncached, r.Input),
		CacheRead:     per(u.CacheRead, r.CacheRead),
		CacheWrite5m:  per(u.CacheWrite5m, r.CacheWrite5m),
		CacheWrite1h:  per(u.CacheWrite1h, r.CacheWrite1h),
		Output:        per(u.Output, r.Output),
	}, true
}

// cheaper lists, for each model, the model to suggest for work that does not
// need it — in M0, sub-agents that search, read and summarise (ADR 0031).
var cheaper = map[string]string{
	"claude-opus-5-5":  "claude-sonnet-5-5",
	"claude-opus-5":    "claude-sonnet-5-5",
	"claude-opus-4-8":  "claude-sonnet-5-5",
	"claude-opus-4-7":  "claude-sonnet-5-5",
	"claude-opus-4-6":  "claude-sonnet-5-5",
	"claude-fable-5-1": "claude-sonnet-5-5",
	"claude-fable-5":   "claude-sonnet-5-5",
}

// Cheaper returns the model to suggest in place of model, and false if there
// is none.
func Cheaper(model string) (string, bool) {
	m, ok := cheaper[model]
	return m, ok
}

// DisplayName is a model's name as people write it: "claude-opus-5-5"
// becomes "Opus 5.5", "claude-haiku-4-5-20251001" becomes "Haiku 4.5". Names
// it does not recognise are returned unchanged.
func DisplayName(model string) string {
	rest, ok := strings.CutPrefix(model, "claude-")
	if !ok {
		return model
	}
	parts := strings.Split(rest, "-")
	if len(parts) < 2 {
		return model
	}
	var version []string
	for _, p := range parts[1:] {
		if len(p) > 2 { // a date stamp such as 20251001
			break
		}
		version = append(version, p)
	}
	if len(version) == 0 {
		return model
	}
	family := strings.ToUpper(parts[0][:1]) + parts[0][1:]
	return family + " " + strings.Join(version, ".")
}

// WebSearch is the price of one server-side web search request, in US
// dollars ($10 per 1,000 searches), charged on top of the tokens.
const WebSearch = 0.01
