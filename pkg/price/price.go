// Package price holds the price catalogue and turns token usage into cost.
//
// The catalogue is a Go table for now. docs/data-model.md specifies a
// versioned YAML file, overridable for negotiated rates; this table is the
// data that file will hold.
package price

import (
	"maps"
	"regexp"
	"strings"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// Version identifies the catalogue. Every cost figure should record it.
// Source: https://platform.claude.com/docs/en/about-claude/pricing, list
// prices read on 2026-11-07 and checked again on 2026-11-08, when the
// still-served 4.6-and-later models were added, and on 2026-11-09, when
// Opus 4.5 and Sonnet 4.5 were added. Cache reads on Opus 5.5 and
// Sonnet 5.5 are 0.05x the input price and on Fable 5.1 0.025x (the page's
// footnotes), not the usual 0.1x: Claude Code's own cost records price
// Sonnet 5.5 reads at 0.1x, which shows up as reconciliation gap.
const Version = "anthropic-list-2026-11-09"

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
	// Before 4.6: no US-only surcharge and no fast mode.
	"claude-opus-4-5":           {Rates: Rates{5, 6.25, 10, 0.50, 25}},
	"claude-sonnet-4-5":         {Rates: Rates{3, 3.75, 6, 0.30, 15}},
	"claude-haiku-4-5-20251001": {Rates: Rates{1, 1.25, 2, 0.10, 5}},
	"claude-haiku-4-5":          {Rates: Rates{1, 1.25, 2, 0.10, 5}},
}

// Catalogue returns a copy of the catalogue, by model id, for checks that
// compare it with another copy of the price list (the end-to-end test in
// cmd/trimloop compares it with scripts/recount.py's). Changing the copy
// changes nothing here.
func Catalogue() map[string]Model {
	return maps.Clone(catalogue)
}

// Key returns the catalogue key a model id resolves to, and false if the
// catalogue holds no such model. An id the catalogue does not hold as written
// is tried again without what cloud providers add around the model name:
// "us.anthropic.claude-opus-5-5" and "anthropic.claude-opus-4-5-20251101-v1:0"
// (Amazon Bedrock), "claude-opus-4-5@20251101" (Google Vertex AI), and a
// date such as "claude-opus-4-5-20251101". A dated id the catalogue holds
// (claude-haiku-4-5-20251001) is found before its date is removed. Nothing
// else is guessed: an unknown model stays unpriced.
func Key(model string) (string, bool) {
	if _, ok := catalogue[model]; ok {
		return model, true
	}
	id := model
	for _, p := range []string{"us.", "eu.", "apac.", "global."} {
		if rest, ok := strings.CutPrefix(id, p); ok {
			id = rest
			break
		}
	}
	id = strings.TrimPrefix(id, "anthropic.")
	id = bedrockVersion.ReplaceAllString(id, "")
	if _, ok := catalogue[id]; ok {
		return id, true
	}
	id = dateSuffix.ReplaceAllString(id, "")
	if _, ok := catalogue[id]; ok {
		return id, true
	}
	return "", false
}

// key is Key, falling back to the id as written when the catalogue does not
// hold the model.
func key(model string) string {
	if k, ok := Key(model); ok {
		return k
	}
	return model
}

// lookup finds a model's prices through Key.
func lookup(model string) (Model, bool) {
	k, ok := Key(model)
	if !ok {
		return Model{}, false
	}
	return catalogue[k], true
}

var (
	bedrockVersion = regexp.MustCompile(`-v[0-9]+:[0-9]+$`) // "-v1:0"
	dateSuffix     = regexp.MustCompile(`[-@][0-9]{8}$`)    // "-20251101" or Vertex's "@20251101"
)

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
	m, found := lookup(model)
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
// is none. A provider or dated id is read as the model it names (Key).
func Cheaper(model string) (string, bool) {
	m, ok := cheaper[key(model)]
	return m, ok
}

// successor lists, for older models, the newer model in the same line that
// costs no more per token and counts tokens the same way (same tokenizer), so
// the same work costs no more on it.
var successor = map[string]string{
	"claude-opus-5":   "claude-opus-5-5",
	"claude-opus-4-8": "claude-opus-5-5",
	"claude-opus-4-7": "claude-opus-5-5",
	"claude-fable-5":  "claude-fable-5-1",
	"claude-sonnet-5": "claude-sonnet-5-5",
}

// Successor returns the newer model to suggest in place of model, and false
// if there is none. A provider or dated id is read as the model it names
// (Key).
func Successor(model string) (string, bool) {
	m, ok := successor[key(model)]
	return m, ok
}

// lowerTier lists, for main sessions, the model one tier down to suggest for
// routine work (ADR 0042). A trade-off: untested for quality.
var lowerTier = map[string]string{
	"claude-fable-5-1": "claude-opus-5-5",
}

// LowerTier returns the model one tier down from model for main sessions, and
// false if there is none. A provider or dated id is read as the model it
// names (Key).
func LowerTier(model string) (string, bool) {
	m, ok := lowerTier[key(model)]
	return m, ok
}

// DisplayName is a model's name as people write it: "claude-opus-5-5"
// becomes "Opus 5.5", "claude-haiku-4-5-20251001" becomes "Haiku 4.5". A
// provider or dated id the catalogue holds is named as the model it resolves
// to (Key): "us.anthropic.claude-opus-5-5-v1:0" is "Opus 5.5" too. Names it
// does not recognise are returned unchanged.
func DisplayName(model string) string {
	rest, ok := strings.CutPrefix(key(model), "claude-")
	if !ok {
		return model
	}
	parts := strings.Split(rest, "-")
	if len(parts) < 2 || parts[0] == "" { // "claude--5": no family to name
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
