// Package fixes turns findings into recommendations, using the fix ladder of
// ADR 0030: each recommendation has one primary fix and up to two
// alternatives of different kinds, so the user sees a choice of strength.
//
// The fixes here are rendered for Claude Code. Setting names were checked
// against Claude Code's settings documentation on 2026-11-07; other harnesses
// will get their own tables (ADR 0030 §5).
package fixes

import (
	"fmt"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// Kind is how a fix works, from most to least lasting (ADR 0030 §1).
type Kind string

// The kinds of fix, in ladder order.
const (
	Source    Kind = "source"    // the tool or command produces less
	Structure Kind = "structure" // the work happens where its noise is not carried
	Enforce   Kind = "enforce"   // the harness stops or rewrites the noisy action
	Cap       Kind = "cap"       // a hard limit in the harness's settings
	Advise    Kind = "advise"    // instructions the agent may follow
	Trim      Kind = "trim"      // output cut in the request path (L3, M4)
)

// Fix is one way to act on a finding.
type Fix struct {
	Kind Kind   `json:"kind"`
	What string `json:"what"` // the fix, in a sentence
	How  string `json:"how"`  // how to apply it in Claude Code
}

// Recommendation is the advice for one finding.
type Recommendation struct {
	Pattern    string          `json:"pattern"` // e.g. "P04"
	Title      string          `json:"title"`   // the action, e.g. "Read only the lines you need"
	Finding    string          `json:"finding"` // what was found, in a sentence
	Noise      event.NoiseKind `json:"noise"`
	Saving     float64         `json:"saving"`                // the finding's estimated saving, an upper bound
	Likely     float64         `json:"likely"`                // a likelier saving for this fix; 0 if not estimated
	LikelyNote string          `json:"likely_note,omitempty"` // how Likely was estimated
	// LikelyLow and LikelyHigh bound Likely when it rests on a measured
	// quantity that varies (the size after a compaction); 0 if not given.
	LikelyLow  float64 `json:"likely_low,omitempty"`
	LikelyHigh float64 `json:"likely_high,omitempty"`
	// WeeklyLimit is the saving per week as a percentage of a subscriber's
	// weekly usage limit, where that was measured (ADR 0048); 0 otherwise.
	WeeklyLimit  float64  `json:"weekly_limit_share,omitempty"`
	Examples     []string `json:"examples"` // the shapes or tools that cost most
	Primary      Fix      `json:"primary"`
	Alternatives []Fix    `json:"alternatives"`

	// Check says how to tell whether the fix worked or did harm, where that
	// needs more than another run (for example, a fix untested for quality).
	Check string `json:"check"`

	// Overlap says which other findings share the saving, if any.
	Overlap string `json:"overlap,omitempty"`
}

// Expected is the saving to rank by. Only some recommendations have a
// likely estimate, so ranking uses the upper bound, the one figure every
// recommendation has; mixing the two would compare unlike things.
func (r Recommendation) Expected() float64 { return r.Saving }

// ForToolOutput recommends fixes for the kinds of noise in f with the
// largest savings, at most n, skipping any with a saving under minSaving.
//
// Kinds are ranked by saving. Within a kind, fixes follow the order in the
// table below: ADR 0030 ranks by saving × confidence ÷ effort, and until
// applied fixes are measured, that order is a judgement of effort and
// confidence, with ties going to the more lasting kind.
func ForToolOutput(f detectors.ToolOutput, n int, minSaving float64) []Recommendation {
	var out []Recommendation
	for _, g := range f.ByNoise {
		if len(out) == n {
			break
		}
		if g.Saving < minSaving {
			continue
		}
		ladder, ok := toolOutputFixes[g.Noise]
		if !ok {
			continue
		}
		out = append(out, Recommendation{
			Pattern: "P04",
			Title:   ladder.title,
			Finding: fmt.Sprintf("%s stayed in the session and was sent again on every later turn: "+
				"keeping it there cost %s. Results over %s tokens account for the saving.",
				ladder.what, dollars(g.Carried), thousandsInt(f.Target)),
			Noise:        g.Noise,
			Saving:       g.Saving,
			Examples:     g.Examples,
			Primary:      ladder.fixes[0],
			Alternatives: ladder.fixes[1:],
		})
	}
	return out
}

type ladder struct {
	title string // the action
	what  string // the output it is about, as a sentence subject
	fixes []Fix  // primary first, then at most two alternatives of other kinds
}

// toolOutputFixes are P04's fixes by kind of noise (docs/patterns.md, P04).
// Every fix that removes output keeps failures visible (ADR 0030 §3): tail
// keeps the end of a run, where errors and summaries are, and Claude Code's
// shell output limit cuts the middle, not the end.
var toolOutputFixes = map[event.NoiseKind]ladder{
	event.NoiseFileView: {"Read only the lines you need", "Output from viewing files", []Fix{
		{Advise,
			"Read only the lines needed, and don't re-read a file that hasn't changed.",
			`Add to CLAUDE.md: "View files with the Read tool, using offset and limit for long files; ` +
				`don't use cat, sed -n, head or tail to view files. Don't re-read a file you have already read ` +
				`unless it has changed."`},
		{Enforce,
			"Stop whole-file views through the shell.",
			"Add a PreToolUse hook on Bash that blocks cat, sed -n, head and tail on files, with the reason " +
				`"use Read with offset and limit".`},
		{Structure,
			"Survey many files in a sub-agent, which returns only a summary.",
			`Add to CLAUDE.md: "To explore code across many files, use a sub-agent and bring back only file:line references and conclusions."`},
	}},
	event.NoiseSearch: {"Search more narrowly", "Output from searches and file listings", []Fix{
		{Advise,
			"Search narrowly: a path, a result limit, file names before matches.",
			`Add to CLAUDE.md: "Search narrowly: give Grep a path and a head_limit, or use grep -l or grep -m N; ` +
				`don't list whole trees (ls -R, find without -maxdepth)."`},
		{Structure,
			"Run wide searches in a sub-agent.",
			`Add to CLAUDE.md: "For searches across the whole repo, use the Explore sub-agent and bring back only the matches that matter."`},
		{Enforce,
			"Warn on recursive searches with no limit.",
			"Add a PreToolUse hook on Bash that warns when grep -r or ls -R runs without -l, -m or a pipe to head."},
	}},
	event.NoiseBuildTest: {"Make builds and tests quieter", "Output from builds, tests and installs", []Fix{
		{Source,
			"Make builds and tests print less: no verbose flags, a summary and the failures only.",
			"Add a script (for example scripts/test-quiet.sh) that runs the tests and prints the failing tests " +
				"and the summary, and name it in CLAUDE.md as the way to run tests."},
		{Advise,
			"Run builds and tests quietly, keeping the end of the output.",
			`Add to CLAUDE.md: "Run builds and tests without -v; use quiet flags (-q, --silent), and pipe long ` +
				`output through tail -n 80 so the summary and any failures stay visible."`},
		{Cap,
			"Limit how much shell output reaches the model.",
			`Set BASH_MAX_OUTPUT_LENGTH (characters) in the "env" section of .claude/settings.json. ` +
				"Claude Code cuts the middle of longer output, so the end, with errors and summaries, stays."},
	}},
	event.NoiseData: {"Filter data before printing it", "Data printed by scripts, APIs and diffs", []Fix{
		{Advise,
			"Filter data before printing it.",
			`Add to CLAUDE.md: "Don't print whole data structures, API responses or diffs. Select fields ` +
				`(jq, gh --jq), limit rows, print counts or summaries; use git diff --stat before diffing files."`},
		{Source,
			"Ask the source for less.",
			"Use the tools' own filters: gh api --jq, kubectl -o name or -o jsonpath, SQL with LIMIT and only the needed columns."},
		{Structure,
			"Explore data in a sub-agent, which reports only the answer.",
			`Add to CLAUDE.md: "When exploring data or logs, use a sub-agent and bring back only the findings."`},
	}},
	event.NoiseWeb: {"Do web research in a sub-agent", "Web search results and fetched pages", []Fix{
		{Structure,
			"Do web research in a sub-agent; only its summary returns to the main session.",
			`Add to CLAUDE.md: "Do web research in a sub-agent and return a short summary with links."`},
		{Advise,
			"Fetch pages with a specific question.",
			`Add to CLAUDE.md: "When using WebFetch, ask a specific question about the page rather than for its whole content."`},
	}},
	event.NoiseMCP: {"Trim MCP tool results", "Output from MCP tools", []Fix{
		{Source,
			"Make MCP servers return less, and remove ones the project doesn't use.",
			"Remove unused servers with claude mcp remove <name>; for the rest, use their options for summaries, " +
				"fewer fields or paged results."},
		{Cap,
			"Limit the size of MCP results.",
			`Set MAX_MCP_OUTPUT_TOKENS in the "env" section of .claude/settings.json (default 25,000).`},
		{Structure,
			"Call noisy MCP tools from a sub-agent.",
			`Add to CLAUDE.md: "Use a sub-agent for MCP tools that return large results, and bring back only what's needed."`},
	}},
	event.NoiseSubAgent: {"Ask sub-agents for short reports", "Sub-agent reports", []Fix{
		{Advise,
			"Ask sub-agents for short reports.",
			`Add to CLAUDE.md: "When starting a sub-agent, ask it to report in under 300 words, with file:line ` +
				`references instead of quoted code."`},
	}},
	event.NoiseOther: {"Keep long command output short", "Other command output", []Fix{
		{Advise,
			"Keep long command output short.",
			`Add to CLAUDE.md: "Pipe long command output through tail -n 80 or a filter; don't print what you won't use."`},
		{Cap,
			"Limit how much shell output reaches the model.",
			`Set BASH_MAX_OUTPUT_LENGTH (characters) in the "env" section of .claude/settings.json. ` +
				"Claude Code cuts the middle of longer output, so the end, with errors and summaries, stays."},
	}},
}
