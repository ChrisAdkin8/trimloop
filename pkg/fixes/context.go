package fixes

import (
	"fmt"
	"strings"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
)

// ForStandingContext recommends fixes for standing and injected context
// (P07): one per source a user can change — skills, MCP servers and memory
// files — skipping any under minSaving. The harness's own prompt, tools and
// notes have no setting to change, so they get none. The saving is the
// source's whole cost: an upper bound, since some of it is needed.
func ForStandingContext(f detectors.StandingContext, minSaving float64) []Recommendation {
	cost := map[string]float64{}
	names := map[string][]string{}
	for _, p := range f.Parts {
		cost[p.Source] += p.Cost
		if len(names[p.Source]) < 3 && p.Cost > 0 {
			names[p.Source] = append(names[p.Source], p.Name)
		}
	}
	var out []Recommendation
	for _, src := range []string{detectors.SourceSkills, detectors.SourceMCP, detectors.SourceMemory} {
		if cost[src] < minSaving {
			continue
		}
		r := contextFixes[src]
		r.Pattern = "P07"
		r.Saving = cost[src]
		r.Examples = names[src]
		r.Finding = fmt.Sprintf(r.Finding, dollars(cost[src]))
		if src == detectors.SourceMCP {
			r.Examples = nil
			for _, n := range names[src] {
				r.Examples = append(r.Examples, strings.TrimPrefix(n, "mcp__"))
			}
		}
		out = append(out, r)
	}
	sortRecs(out)
	return out
}

func sortRecs(rs []Recommendation) {
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && rs[j].Expected() > rs[j-1].Expected(); j-- {
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
}

// contextFixes are P07's fixes by source; Finding takes the cost.
var contextFixes = map[string]Recommendation{
	detectors.SourceSkills: {
		Title: "Turn off skills and plugins you don't use",
		Finding: "Claude Code lists every available skill near the start of each session, and the list stays " +
			"in the conversation: %s. Each plugin you install adds its skills to the list.",
		Primary: Fix{Source,
			"Disable plugins and remove skills you don't use.",
			"Run /plugin to disable plugins you don't use, and delete unused skills from ~/.claude/skills " +
				"and the project's .claude/skills."},
	},
	detectors.SourceMCP: {
		Title: "Remove MCP servers you don't use",
		Finding: "Every connected MCP server sends its tool definitions with each request and adds its " +
			"instructions to the conversation: %s.",
		Primary: Fix{Source,
			"Remove MCP servers this project doesn't use.",
			"claude mcp remove <name>, or turn the server off for this project in /mcp."},
		Alternatives: []Fix{
			{Cap, "Limit what each MCP result can add.",
				`Set MAX_MCP_OUTPUT_TOKENS in the "env" section of .claude/settings.json.`},
		},
	},
	detectors.SourceMemory: {
		Title:   "Trim memory files",
		Finding: "Memory files such as CLAUDE.md are added to every session and stay in the conversation: %s.",
		Primary: Fix{Source,
			"Keep memory files short; move instructions needed only sometimes into a skill.",
			"Trim CLAUDE.md to what every session needs. Put task-specific instructions in a skill " +
				"(.claude/skills/<name>/SKILL.md), which loads only when it is used."},
	},
}
