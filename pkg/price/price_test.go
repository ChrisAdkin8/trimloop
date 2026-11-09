package price

import (
	"math"
	"testing"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// million of each token class, so each cost equals the per-million rate.
var million = event.Usage{
	InputUncached: 1e6, CacheRead: 1e6, CacheWrite5m: 1e6, CacheWrite1h: 1e6, Output: 1e6,
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestCost(t *testing.T) {
	tests := []struct {
		name  string
		model string
		usage event.Usage
		opts  Options
		want  event.Cost
	}{
		{
			name: "opus 5.5 list prices", model: "claude-opus-5-5", usage: million,
			want: event.Cost{InputUncached: 4, CacheRead: 0.20, CacheWrite5m: 5, CacheWrite1h: 8, Output: 20},
		},
		{
			name: "fast mode doubles every rate", model: "claude-opus-5-5", usage: million, opts: Options{Fast: true},
			want: event.Cost{InputUncached: 8, CacheRead: 0.40, CacheWrite5m: 10, CacheWrite1h: 16, Output: 40},
		},
		{
			name: "US-only adds 10%", model: "claude-sonnet-5-5", usage: million, opts: Options{USOnly: true},
			want: event.Cost{InputUncached: 2.2, CacheRead: 0.11, CacheWrite5m: 2.75, CacheWrite1h: 4.4, Output: 11},
		},
		{
			name: "US-only has no surcharge before 4.6", model: "claude-haiku-4-5-20251001", usage: million, opts: Options{USOnly: true},
			want: event.Cost{InputUncached: 1, CacheRead: 0.10, CacheWrite5m: 1.25, CacheWrite1h: 2, Output: 5},
		},
		{
			name: "haiku 5.5 short prompt", model: "claude-haiku-5-5",
			usage: event.Usage{InputUncached: 100_000, Output: 1e6},
			want:  event.Cost{InputUncached: 0.01, Output: 0.50},
		},
		{
			name: "haiku 5.5 long prompt: over 100k input, cache reads included", model: "claude-haiku-5-5",
			usage: event.Usage{InputUncached: 1, CacheRead: 100_000, Output: 1e6},
			want:  event.Cost{InputUncached: 0.0000005, CacheRead: 0.005, Output: 2.50},
		},
		{
			name: "no tokens costs nothing, even for an unknown model", model: "<synthetic>",
			want: event.Cost{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Cost(tt.model, tt.usage, tt.opts)
			if !ok {
				t.Fatal("not priced")
			}
			if !near(got.InputUncached, tt.want.InputUncached) || !near(got.CacheRead, tt.want.CacheRead) ||
				!near(got.CacheWrite5m, tt.want.CacheWrite5m) || !near(got.CacheWrite1h, tt.want.CacheWrite1h) ||
				!near(got.Output, tt.want.Output) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCostNotPriced(t *testing.T) {
	if _, ok := Cost("claude-unknown", million, Options{}); ok {
		t.Error("unknown model: got ok")
	}
	if _, ok := Cost("claude-sonnet-5-5", million, Options{Fast: true}); ok {
		t.Error("fast mode with no published price: got ok")
	}
}

// TestCostProviderIDs checks that the ids cloud providers and dated releases
// use are priced as the model they name, and that only those forms are.
func TestCostProviderIDs(t *testing.T) {
	opus55 := event.Cost{InputUncached: 4, CacheRead: 0.20, CacheWrite5m: 5, CacheWrite1h: 8, Output: 20}
	opus45 := event.Cost{InputUncached: 5, CacheRead: 0.50, CacheWrite5m: 6.25, CacheWrite1h: 10, Output: 25}
	sonnet55 := event.Cost{InputUncached: 2, CacheRead: 0.10, CacheWrite5m: 2.50, CacheWrite1h: 4, Output: 10}
	sonnet45 := event.Cost{InputUncached: 3, CacheRead: 0.30, CacheWrite5m: 3.75, CacheWrite1h: 6, Output: 15}
	haiku45 := event.Cost{InputUncached: 1, CacheRead: 0.10, CacheWrite5m: 1.25, CacheWrite1h: 2, Output: 5}
	for _, c := range []struct {
		model string
		want  event.Cost
	}{
		{"claude-opus-4-5", opus45},
		{"claude-sonnet-4-5", sonnet45},
		{"claude-haiku-4-5-20251001", haiku45}, // held as written
		{"us.anthropic.claude-opus-5-5", opus55},
		{"eu.anthropic.claude-opus-5-5", opus55},
		{"apac.anthropic.claude-opus-5-5", opus55},
		{"anthropic.claude-opus-5-5", opus55},
		{"global.anthropic.claude-sonnet-5-5", sonnet55},
		{"anthropic.claude-opus-4-5-20251101-v1:0", opus45},
		{"us.anthropic.claude-sonnet-4-5-20250929-v2:0", sonnet45},
		{"anthropic.claude-haiku-4-5-20251001-v1:0", haiku45},
		{"claude-opus-4-5@20251101", opus45},
		{"claude-opus-4-5-20251101", opus45},
		{"claude-sonnet-4-5-20250929", sonnet45},
	} {
		t.Run(c.model, func(t *testing.T) {
			got, ok := Cost(c.model, million, Options{})
			if !ok {
				t.Fatal("not priced")
			}
			if got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
	// Before 4.6: US-only adds nothing and fast mode has no price.
	if got, ok := Cost("anthropic.claude-opus-4-5-20251101-v1:0", million, Options{USOnly: true}); !ok || got != opus45 {
		t.Errorf("opus 4.5 US-only: got %+v, %v; want %+v", got, ok, opus45)
	}
	if _, ok := Cost("claude-sonnet-4-5", million, Options{Fast: true}); ok {
		t.Error("sonnet 4.5 fast mode: got ok")
	}
	for _, model := range []string{
		"claude-opus",                     // a family, not a model
		"claude-opus-4",                   // not in the catalogue
		"us.anthropic.claude-unknown-1-0", // unknown under a prefix
		"anthropic.claude-opus-5-5-latest",
		"claude-opus-4-5-2025110", // seven digits: not a date
		"xx.anthropic.claude-opus-5-5",
		"us.us.anthropic.claude-opus-5-5",
	} {
		if _, ok := Cost(model, million, Options{}); ok {
			t.Errorf("%s: got ok, want unpriced", model)
		}
	}
}

func TestCheaper(t *testing.T) {
	for from, to := range cheaper {
		if _, ok := catalogue[to]; !ok {
			t.Errorf("%s: suggested model %s has no price", from, to)
		}
		a, _ := Cost(from, event.Usage{InputUncached: 1e6, Output: 1e6}, Options{})
		b, _ := Cost(to, event.Usage{InputUncached: 1e6, Output: 1e6}, Options{})
		if b.Total() >= a.Total() {
			t.Errorf("%s: suggested model %s is not cheaper", from, to)
		}
	}
	if _, ok := Cheaper("claude-sonnet-5-5"); ok {
		t.Error("the cheapest models should have no suggestion")
	}
}

func TestDisplayName(t *testing.T) {
	for in, want := range map[string]string{
		"claude-opus-5-5": "Opus 5.5", "claude-sonnet-5": "Sonnet 5", "claude-fable-5-1": "Fable 5.1",
		"claude-haiku-4-5-20251001": "Haiku 4.5", "<synthetic>": "<synthetic>", "claude-x": "claude-x", "gpt-5": "gpt-5",
		"claude--5": "claude--5", "claude-": "claude-", // no family name: as written, never a crash
	} {
		if got := DisplayName(in); got != want {
			t.Errorf("DisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKey(t *testing.T) {
	for in, want := range map[string]string{
		"claude-opus-5-5":                            "claude-opus-5-5",
		"us.anthropic.claude-opus-5-5-v1:0":          "claude-opus-5-5",
		"claude-opus-5-5@20261001":                   "claude-opus-5-5",
		"claude-opus-5-5-20261001":                   "claude-opus-5-5",
		"us.anthropic.claude-opus-4-5-20251101-v1:0": "claude-opus-4-5",
		"claude-haiku-4-5-20251001":                  "claude-haiku-4-5-20251001", // held as written
		"anthropic.claude-haiku-4-5-20251001-v1:0":   "claude-haiku-4-5-20251001",
	} {
		if got, ok := Key(in); !ok || got != want {
			t.Errorf("Key(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"claude-opus-4", "xx.anthropic.claude-opus-5-5", "<synthetic>"} {
		if got, ok := Key(in); ok {
			t.Errorf("Key(%q) = %q, want not found", in, got)
		}
	}
}

// TestSuggestionsForProviderIDs checks that a provider or dated id gets the
// same suggestions as the model it names.
func TestSuggestionsForProviderIDs(t *testing.T) {
	helpers := map[string]func(string) (string, bool){
		"Cheaper": Cheaper, "Successor": Successor, "LowerTier": LowerTier,
	}
	tests := []struct {
		helper, model, want string
	}{
		{"Cheaper", "us.anthropic.claude-opus-5-5-v1:0", "claude-sonnet-5-5"},
		{"Cheaper", "claude-opus-5-5@20261001", "claude-sonnet-5-5"},
		{"Cheaper", "claude-opus-5-5-20261001", "claude-sonnet-5-5"},
		{"Cheaper", "global.anthropic.claude-fable-5-1", "claude-sonnet-5-5"},
		{"Successor", "us.anthropic.claude-opus-5-v1:0", "claude-opus-5-5"},
		{"Successor", "claude-opus-4-8@20260801", "claude-opus-5-5"},
		{"Successor", "claude-sonnet-5-20260601", "claude-sonnet-5-5"},
		{"LowerTier", "us.anthropic.claude-fable-5-1-v1:0", "claude-opus-5-5"},
		{"LowerTier", "claude-fable-5-1@20261001", "claude-opus-5-5"},
		{"LowerTier", "claude-fable-5-1-20261001", "claude-opus-5-5"},
	}
	for _, tt := range tests {
		if got, ok := helpers[tt.helper](tt.model); !ok || got != tt.want {
			t.Errorf("%s(%q) = %q, %v; want %q", tt.helper, tt.model, got, ok, tt.want)
		}
	}
	for _, tt := range []struct{ helper, model string }{
		{"Cheaper", "us.anthropic.claude-sonnet-5-5-v1:0"},
		{"Successor", "claude-opus-5-5@20261001"},
		{"LowerTier", "claude-opus-5-5-20261001"},
		{"Cheaper", "us.anthropic.claude-unknown-1-0"},
	} {
		if got, ok := helpers[tt.helper](tt.model); ok {
			t.Errorf("%s(%q) = %q, want none", tt.helper, tt.model, got)
		}
	}
}

func TestDisplayNameProviderIDs(t *testing.T) {
	for in, want := range map[string]string{
		"claude-opus-5-5@20261001":                   "Opus 5.5",
		"claude-opus-5-5-20261001":                   "Opus 5.5",
		"us.anthropic.claude-opus-5-5-v1:0":          "Opus 5.5",
		"us.anthropic.claude-opus-4-5-20251101-v1:0": "Opus 4.5",
		"anthropic.claude-haiku-4-5-20251001-v1:0":   "Haiku 4.5",
		"global.anthropic.claude-sonnet-5-5":         "Sonnet 5.5",
		"us.anthropic.claude-unknown-1-0":            "us.anthropic.claude-unknown-1-0", // unknown: unchanged
	} {
		if got := DisplayName(in); got != want {
			t.Errorf("DisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}
