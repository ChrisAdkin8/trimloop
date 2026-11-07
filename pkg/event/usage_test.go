package event

import (
	"reflect"
	"testing"
)

func intPtr(n int) *int { return &n }

func TestUsageAdd(t *testing.T) {
	a := Usage{InputUncached: 1, CacheRead: 2, CacheWrite5m: 3, CacheWrite1h: 4, Output: 5}
	b := Usage{InputUncached: 10, CacheRead: 20, CacheWrite5m: 30, CacheWrite1h: 40, Output: 50}
	tests := []struct {
		name             string
		reasonA, reasonB *int
		want             *int
	}{
		{"neither reports reasoning", nil, nil, nil},
		{"one side reports it", intPtr(7), nil, intPtr(7)},
		{"other side reports it", nil, intPtr(7), intPtr(7)},
		{"both report it", intPtr(7), intPtr(8), intPtr(15)},
		{"reported as zero stays reported", intPtr(0), nil, intPtr(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y := a, b
			x.Reasoning, y.Reasoning = tt.reasonA, tt.reasonB
			got := x.Add(y)
			want := Usage{InputUncached: 11, CacheRead: 22, CacheWrite5m: 33, CacheWrite1h: 44, Output: 55, Reasoning: tt.want}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestUsageAddDoesNotShareReasoning(t *testing.T) {
	a := Usage{Reasoning: intPtr(1)}
	sum := a.Add(Usage{})
	*sum.Reasoning = 99
	if *a.Reasoning != 1 {
		t.Errorf("changing the sum changed an input: got %d, want 1", *a.Reasoning)
	}
}
