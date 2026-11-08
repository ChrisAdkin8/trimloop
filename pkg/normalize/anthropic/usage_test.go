package anthropic

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

func intPtr(n int) *int { return &n }

func TestNormalizeUsage(t *testing.T) {
	tests := []struct {
		name string
		json string // usage block as it appears in a transcript
		want event.Usage
	}{
		{
			name: "split by lifetime",
			json: `{"input_tokens":10,"cache_read_input_tokens":200,
				"cache_creation_input_tokens":50,
				"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30},
				"output_tokens":7}`,
			want: event.Usage{InputUncached: 10, CacheRead: 200, CacheWrite5m: 20, CacheWrite1h: 30, Output: 7},
		},
		{
			name: "no split: all cache writes count as 5m",
			json: `{"input_tokens":10,"cache_read_input_tokens":200,
				"cache_creation_input_tokens":50,"output_tokens":7}`,
			want: event.Usage{InputUncached: 10, CacheRead: 200, CacheWrite5m: 50, Output: 7},
		},
		{
			name: "split with a zero total: the split counts",
			json: `{"cache_creation_input_tokens":0,
				"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":862}}`,
			want: event.Usage{CacheWrite1h: 862},
		},
		{
			name: "split short of the total: the rest counts as 5m",
			json: `{"cache_creation_input_tokens":50,
				"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0}}`,
			want: event.Usage{CacheWrite5m: 50},
		},
		{
			name: "thinking tokens reported",
			json: `{"input_tokens":1,"output_tokens":90,
				"output_tokens_details":{"thinking_tokens":60}}`,
			want: event.Usage{InputUncached: 1, Output: 90, Reasoning: intPtr(60)},
		},
		{
			name: "thinking tokens reported as zero",
			json: `{"input_tokens":1,"output_tokens":5,
				"output_tokens_details":{"thinking_tokens":0}}`,
			want: event.Usage{InputUncached: 1, Output: 5, Reasoning: intPtr(0)},
		},
		{
			name: "details without thinking tokens: reasoning stays nil",
			json: `{"input_tokens":1,"output_tokens":5,"output_tokens_details":{}}`,
			want: event.Usage{InputUncached: 1, Output: 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u Usage
			if err := json.Unmarshal([]byte(tt.json), &u); err != nil {
				t.Fatal(err)
			}
			got := NormalizeUsage(u)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
