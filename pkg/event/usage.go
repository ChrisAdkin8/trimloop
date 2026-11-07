// Package event holds Trimloop's canonical event model: the records every tap
// and normaliser produce, whatever the harness or provider.
package event

// Usage is one provider response's token usage, sorted into the classes in
// docs/data-model.md ("Usage normalisation"). Every cost, ratio and saving is
// computed from these fields, never from raw provider fields.
type Usage struct {
	InputUncached int // input billed at the normal input price
	CacheRead     int // input served from cache
	CacheWrite5m  int // input written to cache with the short lifetime
	CacheWrite1h  int // input written to cache with the long lifetime
	Output        int // all billed output, including reasoning

	// Reasoning is the reasoning / thinking tokens, a subset of Output.
	// It is nil when the provider does not report it.
	Reasoning *int
}

// InputTotal is all input tokens, cached or not.
func (u Usage) InputTotal() int {
	return u.InputUncached + u.CacheRead + u.CacheWrite5m + u.CacheWrite1h
}

// Add returns the sum of u and o. Reasoning stays nil only if neither side
// reports it; a side that does not report it adds nothing.
func (u Usage) Add(o Usage) Usage {
	sum := Usage{
		InputUncached: u.InputUncached + o.InputUncached,
		CacheRead:     u.CacheRead + o.CacheRead,
		CacheWrite5m:  u.CacheWrite5m + o.CacheWrite5m,
		CacheWrite1h:  u.CacheWrite1h + o.CacheWrite1h,
		Output:        u.Output + o.Output,
	}
	if u.Reasoning != nil || o.Reasoning != nil {
		n := 0
		if u.Reasoning != nil {
			n += *u.Reasoning
		}
		if o.Reasoning != nil {
			n += *o.Reasoning
		}
		sum.Reasoning = &n
	}
	return sum
}
