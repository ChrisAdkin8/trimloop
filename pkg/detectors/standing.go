package detectors

import (
	"sort"
	"strings"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// StandingContext is P07, bloated standing and injected context: what every
// request carries before any work is done (the system prompt and tool
// definitions), and the text the harness adds to the conversation (memory
// files, skill listings, MCP instructions, reminders), priced for as long as
// it stays in the conversation.
type StandingContext struct {
	Turns   int `json:"turns"`
	Covered int `json:"covered"` // turns with a recorded system prompt and tool list

	SystemPrompt    float64 `json:"system_prompt"`    // cost of sending the system prompt on every turn
	ToolDefinitions float64 `json:"tool_definitions"` // cost of sending the tool definitions
	Injected        float64 `json:"injected"`         // carried cost of injected text

	// Parts lists every source, largest cost first: tool definitions by
	// MCP server (or built-in), injected text by kind, memory files by name.
	Parts []ContextPart `json:"parts"`
}

// Total is the cost of everything standing or injected.
func (s StandingContext) Total() float64 { return s.SystemPrompt + s.ToolDefinitions + s.Injected }

// ContextPart is one source of standing or injected context.
type ContextPart struct {
	Name   string  `json:"name"`   // e.g. "skill_listing", "mcp__github", "Project CLAUDE.md"
	Source string  `json:"source"` // see the Source constants
	Cost   float64 `json:"cost"`
	Tokens int     `json:"tokens"` // its average size each time it was sent or added
	Times  int     `json:"times"`  // times it was added (injected) or turns it was sent (standing)

	sum int // tokens over all Times, for the average
}

// Where standing and injected context comes from; the fix depends on it.
const (
	SourceSkills  = "skills"      // the skill listing: turn off unused skills and plugins
	SourceMCP     = "mcp"         // an MCP server's tools or instructions: remove unused servers
	SourceMemory  = "memory_file" // a memory file such as CLAUDE.md: trim it
	SourceHarness = "harness"     // the harness's own prompt, tools and notes: no setting to change
)

// injectedSource sorts an injected kind by what can be done about it.
func injectedSource(kind string) string {
	switch kind {
	case "skill_listing":
		return SourceSkills
	// "invoked_skills" is a skill that was used, not the listing: turning
	// off unused skills does not remove it, so it falls through.
	case "mcp_instructions_delta", "mcp_instructions":
		return SourceMCP
	case "memory_file", "instructions":
		return SourceMemory
	}
	return SourceHarness
}

// FindStandingContext prices the system prompt and tool definitions each
// turn sent, and the injected text each turn carried.
func FindStandingContext(sessions []event.Session) StandingContext {
	var f StandingContext
	parts := map[string]*ContextPart{}
	part := func(name, source string) *ContextPart {
		k := source + "\x00" + name
		p, ok := parts[k]
		if !ok {
			p = &ContextPart{Name: name, Source: source}
			parts[k] = p
		}
		return p
	}

	for _, s := range sessions {
		var inContext []*injectedCost
		for i, t := range s.Turns {
			f.Turns++
			if t.CompactedBefore {
				inContext = nil
			}
			history, prefix, arrival := historyRate(t), prefixRate(t), arrivalRate(t)

			// The fixed start: re-sent on every turn, from the cache after the first.
			if st := t.Standing; st.Bytes() > 0 {
				f.Covered++
				rate := prefix
				if i == 0 || !sameStanding(s.Turns[i-1].Standing, st) {
					rate = arrival // new, or changed: written again
				}
				sys := float64(tokens(st.SystemBytes)) * rate
				f.SystemPrompt += sys
				p := part("System prompt", SourceHarness)
				p.Cost += sys
				p.sum += tokens(st.SystemBytes)
				p.Times++
				byGroup := map[string]int{}
				for _, d := range st.Tools {
					byGroup[toolDefGroup(d.Name)] += d.Bytes
				}
				groups := make([]string, 0, len(byGroup))
				for g := range byGroup {
					groups = append(groups, g)
				}
				sort.Strings(groups) // a fixed order, so the sums are the same every run
				for _, g := range groups {
					b := byGroup[g]
					c := float64(tokens(b)) * rate
					f.ToolDefinitions += c
					src := mcpSource(g)
					p := part(g, src)
					p.Cost += c
					p.sum += tokens(b)
					p.Times++
				}
			}

			// Injected text: carried like tool output.
			for _, ic := range inContext {
				c := float64(ic.piece.Tokens()) * history
				ic.part.Cost += c
				f.Injected += c
			}
			for _, in := range t.Injected {
				name, src := in.Name, injectedSource(in.Kind)
				switch {
				case name == "":
					name = in.Kind
				case src == SourceMCP:
					name, src = name+" (instructions)", serverSource(name)
				}
				p := part(name, src)
				p.sum += in.Tokens()
				p.Times++
				c := float64(in.Tokens()) * arrival
				p.Cost += c
				f.Injected += c
				inContext = append(inContext, &injectedCost{piece: in, part: p})
			}
		}
	}

	for _, p := range parts {
		if p.Times > 0 {
			p.Tokens = p.sum / p.Times
		}
		f.Parts = append(f.Parts, *p)
	}
	sortBy(f.Parts, func(p ContextPart) (float64, string) { return p.Cost, p.Name })
	return f
}

type injectedCost struct {
	piece event.Injected
	part  *ContextPart
}

// toolDefGroup groups tool definitions: MCP tools by server, the rest as the
// harness's built-in tools.
// appServers are MCP servers the Claude desktop app adds itself, besides
// those named "ccd_" or "Claude_": `claude mcp remove` cannot remove them.
var appServers = map[string]bool{
	"visualize": true, "terminal": true, "claude-artifacts": true, "claude-in-chrome": true,
	"mcp-registry": true, "scheduled-tasks": true,
}

// mcpSource sorts a tool-definition group ("mcp__<server>") by what can be
// done about it; other groups are the harness's own tools.
func mcpSource(group string) string {
	server, ok := strings.CutPrefix(group, "mcp__")
	if !ok {
		return SourceHarness
	}
	return serverSource(server)
}

// serverSource sorts an MCP server by who can change it. Servers a plugin
// brings ("plugin_..." in tool names, "plugin:..." in instructions) are
// turned off with the plugin; servers the desktop app adds have no setting
// this report can point to; the rest the user added, or connected in
// claude.ai.
func serverSource(server string) string {
	switch {
	case strings.HasPrefix(server, "plugin_"), strings.HasPrefix(server, "plugin:"):
		return SourceSkills
	case strings.HasPrefix(server, "ccd_"), strings.HasPrefix(server, "Claude_"), appServers[server]:
		return SourceHarness
	}
	return SourceMCP
}

func toolDefGroup(name string) string {
	if strings.HasPrefix(name, "mcp__") {
		return toolGroup(name)
	}
	return "Built-in tool definitions"
}

func tokens(bytes int) int { return (bytes + event.BytesPerToken - 1) / event.BytesPerToken }

// sameStanding reports whether two records of the system prompt and tool
// list have the same content, as far as sizes and tool names show. The
// harness records it again at times without changing it; only a change is
// written to the cache again.
func sameStanding(a, b *event.Standing) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil || a.SystemBytes != b.SystemBytes || len(a.Tools) != len(b.Tools) {
		return false
	}
	for i := range a.Tools {
		if a.Tools[i] != b.Tools[i] {
			return false
		}
	}
	return true
}

// UnusedAppServers returns the tool-definition parts of MCP servers the
// desktop app adds that were sent but never called in these sessions, most
// costly first: P06 in reduced fidelity (ADRs 0050, 0052). They cannot be
// removed like other servers, so P07's "remove" card leaves them out.
func UnusedAppServers(f StandingContext, sessions []event.Session) []ContextPart {
	called := map[string]bool{}
	for _, s := range sessions {
		for _, t := range s.Turns {
			for _, c := range t.ToolCalls {
				called[toolGroup(c.Tool)] = true
			}
		}
	}
	var out []ContextPart
	for _, p := range f.Parts {
		server, ok := strings.CutPrefix(p.Name, "mcp__")
		if !ok || called[p.Name] || p.Cost <= 0 || !appServer(server) {
			continue
		}
		out = append(out, p)
	}
	sortBy(out, func(p ContextPart) (float64, string) { return p.Cost, p.Name })
	return out
}

// appServer reports whether the desktop app adds an MCP server itself.
func appServer(server string) bool {
	return strings.HasPrefix(server, "ccd_") || strings.HasPrefix(server, "Claude_") || appServers[server]
}
