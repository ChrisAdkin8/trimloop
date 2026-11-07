package event

// Cost is what one turn (or a sum of turns) cost, in US dollars, split by the
// same token classes as Usage.
type Cost struct {
	InputUncached float64 `json:"input_uncached"`
	CacheRead     float64 `json:"cache_read"`
	CacheWrite5m  float64 `json:"cache_write_5m"`
	CacheWrite1h  float64 `json:"cache_write_1h"`
	Output        float64 `json:"output"`
}

// Input is the cost of all input: uncached, cache reads and cache writes.
func (c Cost) Input() float64 {
	return c.InputUncached + c.CacheRead + c.CacheWrite5m + c.CacheWrite1h
}

// CacheWrites is the cost of writing to the cache, both lifetimes.
func (c Cost) CacheWrites() float64 { return c.CacheWrite5m + c.CacheWrite1h }

// Total is the cost of everything.
func (c Cost) Total() float64 { return c.Input() + c.Output }

// Add returns the sum of c and o.
func (c Cost) Add(o Cost) Cost {
	return Cost{
		InputUncached: c.InputUncached + o.InputUncached,
		CacheRead:     c.CacheRead + o.CacheRead,
		CacheWrite5m:  c.CacheWrite5m + o.CacheWrite5m,
		CacheWrite1h:  c.CacheWrite1h + o.CacheWrite1h,
		Output:        c.Output + o.Output,
	}
}
