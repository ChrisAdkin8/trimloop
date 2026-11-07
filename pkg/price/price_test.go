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
