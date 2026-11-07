package detectors

import (
	"sort"
	"strings"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// ToolOutput is P04, oversized and noisy tool output. It is also the
// breakdown of what fills the context behind P19: its cost is part of every
// turn's input, so it is never added to P19's saving (docs/patterns.md, P04).
type ToolOutput struct {
	Target int `json:"target"` // tokens per result above which the excess counts as saving

	Calls     int     `json:"calls"`     // tool results sent to the model
	Tokens    int     `json:"tokens"`    // their estimated tokens, each counted once
	Carried   float64 `json:"carried"`   // carried cost of all of them
	Saving    float64 `json:"saving"`    // carried cost of the tokens above Target, an upper bound
	Persisted int     `json:"persisted"` // results the harness saved to disk, sending a preview

	ByTool  []Group `json:"by_tool"`  // by tool, MCP tools by server; largest carried cost first
	ByShape []Group `json:"by_shape"` // shell commands by shape; largest carried cost first
	ByNoise []Group `json:"by_noise"` // by kind of noise; largest saving first

	FileViews     int     `json:"file_views"`     // results that showed a file
	Rereads       int     `json:"rereads"`        // views of a file already viewed, unchanged, in the session
	RereadCarried float64 `json:"reread_carried"` // carried cost of those re-reads
}

// Group is one tool, command shape or kind of noise.
type Group struct {
	Name     string          `json:"name"`
	Noise    event.NoiseKind `json:"noise"`
	Calls    int             `json:"calls"`
	Tokens   int             `json:"tokens"`
	Carried  float64         `json:"carried"`
	Saving   float64         `json:"saving"`
	Filtered int             `json:"filtered"` // shell calls whose output was already filtered
	Rereads  int             `json:"rereads"`

	// Examples, for a kind of noise: the shapes and tools that cost most in
	// it, largest first, at most three.
	Examples []string `json:"examples"`
}

// callCost is one tool result and what carrying it cost.
type callCost struct {
	call    event.ToolCall
	carried float64
	reread  bool
	at      time.Time // when the result was first sent; zero if not known
}

// FindToolOutput works out the carried cost of every tool result and groups
// it. target is the result size, in tokens, above which the excess counts as
// saving; topN limits ByTool and ByShape.
func FindToolOutput(sessions []event.Session, target, topN int) ToolOutput {
	f := ToolOutput{Target: target}
	byTool := map[string]*Group{}
	byShape := map[string]*Group{}
	byNoise := map[event.NoiseKind]*Group{}
	examples := map[event.NoiseKind]map[string]float64{}

	add := func(groups map[string]*Group, name string, cc callCost, saving float64) {
		g, ok := groups[name]
		if !ok {
			g = &Group{Name: name, Noise: cc.call.Noise}
			groups[name] = g
		}
		addTo(g, cc, saving)
	}

	for _, s := range sessions {
		for _, cc := range carry(s) {
			c := cc.call
			tokens := c.Result.Tokens()
			saving := 0.0
			if tokens > target {
				saving = cc.carried * float64(tokens-target) / float64(tokens)
			}

			f.Calls++
			f.Tokens += tokens
			f.Carried += cc.carried
			f.Saving += saving
			if c.Result.Persisted {
				f.Persisted++
			}
			if c.FileOp == event.FileView {
				f.FileViews++
			}
			if cc.reread {
				f.Rereads++
				f.RereadCarried += cc.carried
			}

			tool := toolGroup(c.Tool)
			add(byTool, tool, cc, saving)
			name := tool
			if c.Shape != "" {
				add(byShape, c.Shape, cc, saving)
				name = c.Shape
			}

			g, ok := byNoise[c.Noise]
			if !ok {
				g = &Group{Name: string(c.Noise), Noise: c.Noise}
				byNoise[c.Noise] = g
				examples[c.Noise] = map[string]float64{}
			}
			addTo(g, cc, saving)
			examples[c.Noise][name] += cc.carried
		}
	}

	f.ByTool = ranked(byTool, topN, func(g Group) float64 { return g.Carried })
	f.ByShape = ranked(byShape, topN, func(g Group) float64 { return g.Carried })
	noise := map[string]*Group{}
	for k, g := range byNoise {
		g.Examples = largest(examples[k], 3)
		noise[string(k)] = g
	}
	f.ByNoise = ranked(noise, len(noise), func(g Group) float64 { return g.Saving })
	return f
}

func addTo(g *Group, cc callCost, saving float64) {
	g.Calls++
	g.Tokens += cc.call.Result.Tokens()
	g.Carried += cc.carried
	g.Saving += saving
	if cc.call.Filtered {
		g.Filtered++
	}
	if cc.reread {
		g.Rereads++
	}
}

// carry prices every tool result in a session for as long as it stays in
// the context. A turn's tool results are first sent on the next turn, which
// pays to add them (usually a cache write); every later turn pays to re-send
// them (usually a cache read), until the session ends or is compacted.
//
// Each turn's prices come from its own cost and usage, so model, long-prompt
// tier and fast-mode prices are already applied. Results of the last turn,
// never sent, cost nothing.
func carry(s event.Session) []callCost {
	var out []callCost
	var inContext []int         // indexes into out
	viewed := map[string]bool{} // files viewed, unchanged, since the last compaction
	for i, t := range s.Turns {
		if t.CompactedBefore {
			inContext = nil
			viewed = map[string]bool{}
		}
		history := historyRate(t)
		for _, k := range inContext {
			out[k].carried += float64(out[k].call.Result.Tokens()) * history
		}
		if i == 0 || t.CompactedBefore {
			continue // the previous turn's results went into the compaction
		}
		arrival := arrivalRate(t)
		for _, c := range s.Turns[i-1].ToolCalls {
			if !c.HasResult {
				continue
			}
			cc := callCost{call: c, carried: float64(c.Result.Tokens()) * arrival, at: t.Time}
			switch {
			case c.File == "":
			case c.FileOp == event.FileView:
				cc.reread = viewed[c.File]
				viewed[c.File] = true
			case c.FileOp == event.FileEdit:
				delete(viewed, c.File)
			}
			out = append(out, cc)
			inContext = append(inContext, len(out)-1)
		}
	}
	return out
}

// arrivalRate is a turn's price per token for input new to it: a cache
// write if the turn wrote to the cache, else uncached input.
func arrivalRate(t event.Turn) float64 {
	u, c := t.Usage, t.Cost
	if w := u.CacheWrite5m + u.CacheWrite1h; w > 0 {
		return c.CacheWrites() / float64(w)
	}
	if u.InputUncached > 0 {
		return c.InputUncached / float64(u.InputUncached)
	}
	return perToken(c.CacheRead, u.CacheRead)
}

// historyRate is a turn's price per token for the history it re-sends: a
// cache read if it read from the cache, else a cache write (the cache was
// rebuilt, for example after a pause), else uncached input.
func historyRate(t event.Turn) float64 {
	u, c := t.Usage, t.Cost
	if u.CacheRead > 0 {
		return c.CacheRead / float64(u.CacheRead)
	}
	if w := u.CacheWrite5m + u.CacheWrite1h; w > 0 {
		return c.CacheWrites() / float64(w)
	}
	return perToken(c.InputUncached, u.InputUncached)
}

func perToken(cost float64, tokens int) float64 {
	if tokens == 0 {
		return 0
	}
	return cost / float64(tokens)
}

// toolGroup groups MCP tools by server: "mcp__github__get_issue" becomes
// "mcp__github".
func toolGroup(tool string) string {
	if rest, ok := strings.CutPrefix(tool, "mcp__"); ok {
		server, _, _ := strings.Cut(rest, "__")
		return "mcp__" + server
	}
	return tool
}

// ranked returns the n groups with the largest key, largest first.
func ranked(groups map[string]*Group, n int, key func(Group) float64) []Group {
	out := make([]Group, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := key(out[i]), key(out[j]); a != b {
			return a > b
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// largest returns the n names with the largest values, largest first.
func largest(m map[string]float64, n int) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		if m[names[i]] != m[names[j]] {
			return m[names[i]] > m[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) > n {
		names = names[:n]
	}
	return names
}
