// Package anthropic normalises Anthropic Messages API records into the
// canonical event model.
package anthropic

import "github.com/ChrisAdkin8/trimloop/pkg/event"

// Usage is the usage block of an Anthropic Messages response, as it appears in
// the API and in Claude Code transcripts. Optional parts are pointers, so a
// missing part can be told apart from a zero value.
type Usage struct {
	InputTokens              int            `json:"input_tokens"`
	CacheReadInputTokens     int            `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int            `json:"cache_creation_input_tokens"`
	CacheCreation            *CacheCreation `json:"cache_creation"`
	OutputTokens             int            `json:"output_tokens"`
	OutputTokensDetails      *OutputDetails `json:"output_tokens_details"`
}

// CacheCreation splits cache writes by cache lifetime.
type CacheCreation struct {
	Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
}

// OutputDetails breaks down output tokens.
type OutputDetails struct {
	ThinkingTokens *int `json:"thinking_tokens"`
}

// NormalizeUsage maps an Anthropic usage block onto the canonical usage
// classes (docs/data-model.md, "Usage normalisation", Anthropic Messages row).
func NormalizeUsage(u Usage) event.Usage {
	out := event.Usage{
		InputUncached: u.InputTokens, // already excludes cached input
		CacheRead:     u.CacheReadInputTokens,
		Output:        u.OutputTokens,
	}

	// Prefer the split by lifetime: some records give the split with a zero
	// total. Writes the total counts beyond the split count as 5m, the
	// cheaper rate, so none is dropped (docs/data-model.md; scripts/recount.py
	// does the same, and the golden logs hold such a line).
	if u.CacheCreation != nil {
		out.CacheWrite5m = u.CacheCreation.Ephemeral5mInputTokens
		out.CacheWrite1h = u.CacheCreation.Ephemeral1hInputTokens
	}
	if rest := u.CacheCreationInputTokens - out.CacheWrite5m - out.CacheWrite1h; rest > 0 {
		out.CacheWrite5m += rest
	}

	if u.OutputTokensDetails != nil && u.OutputTokensDetails.ThinkingTokens != nil {
		n := *u.OutputTokensDetails.ThinkingTokens
		out.Reasoning = &n
	}
	return out
}
