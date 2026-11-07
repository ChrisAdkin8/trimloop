package detectors

import "github.com/ChrisAdkin8/trimloop/pkg/event"

// Attribution is spend by project, and by the skill, plugin and MCP server
// the harness names on each turn (ADR 0031). Nothing is inferred: turns the
// harness attributes to nothing are left out of the skill, plugin and MCP
// tables.
type Attribution struct {
	Cost float64 // all spend, for shares

	ByProject   []Spend
	BySkill     []Spend
	ByPlugin    []Spend
	ByMCPServer []Spend
}

// Spend is what one project, skill, plugin or MCP server cost.
type Spend struct {
	Name          string
	Cost          float64
	Turns         int
	Conversations int     // main sessions with their sub-agents
	SubAgentCost  float64 // the part spent in sub-agents
}

// FindAttribution adds up spend by project, skill, plugin and MCP server,
// keeping the topN of each, largest first.
func FindAttribution(sessions []event.Session, topN int) Attribution {
	var f Attribution
	groups := map[string]map[string]*Spend{"project": {}, "skill": {}, "plugin": {}, "mcp": {}}
	convs := map[string]map[string]map[string]bool{"project": {}, "skill": {}, "plugin": {}, "mcp": {}}

	add := func(kind, name string, s event.Session, t event.Turn) {
		if name == "" {
			return
		}
		g, ok := groups[kind][name]
		if !ok {
			g = &Spend{Name: name}
			groups[kind][name] = g
			convs[kind][name] = map[string]bool{}
		}
		g.Cost += t.Cost.Total()
		g.Turns++
		if s.Kind == "subagent" {
			g.SubAgentCost += t.Cost.Total()
		}
		convs[kind][name][s.Conversation()] = true
	}

	for _, s := range sessions {
		for _, t := range s.Turns {
			project := t.Project
			if project == "" {
				project = "(unknown)"
			}
			f.Cost += t.Cost.Total()
			add("project", project, s, t)
			add("skill", t.Attribution.Skill, s, t)
			add("plugin", t.Attribution.Plugin, s, t)
			add("mcp", t.Attribution.MCPServer, s, t)
		}
	}

	list := func(kind string) []Spend {
		out := make([]Spend, 0, len(groups[kind]))
		for name, g := range groups[kind] {
			g.Conversations = len(convs[kind][name])
			out = append(out, *g)
		}
		sortBy(out, func(s Spend) (float64, string) { return s.Cost, s.Name })
		return out[:min(topN, len(out))]
	}
	f.ByProject = list("project")
	f.BySkill = list("skill")
	f.ByPlugin = list("plugin")
	f.ByMCPServer = list("mcp")
	return f
}
