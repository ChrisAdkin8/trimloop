package report

import (
	"regexp"
	"strings"

	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// uuidLike matches a server name that is only an identifier.
var uuidLike = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// appTools names the Claude desktop app's own tool servers.
var appTools = map[string]string{
	"ccd_turn":      "task list and wrap-up",
	"ccd_directory": "folder access",
	"ccd_session":   "session management",
	"ccd_view":      "panes",
}

// serverName is an MCP server's name as people would recognise it.
func serverName(n string) string { return serverLabel(n, false) }

// serverLabel names a server; with hideIDs (--no-titles) a connector known
// only by an identifier gets no part of it.
func serverLabel(n string, hideIDs bool) string {
	switch {
	case uuidLike.MatchString(n) && hideIDs:
		return "unnamed connector"
	case uuidLike.MatchString(n):
		return "unnamed connector (" + n[:8] + "…)"
	case strings.HasPrefix(n, "ccd_"):
		if what, ok := appTools[n]; ok {
			return "Claude app: " + what
		}
		return "Claude app (" + n + ")"
	case strings.HasPrefix(n, "plugin:"):
		parts := strings.Split(n, ":")
		return parts[len(parts)-1] + " (from plugin " + parts[1] + ")"
	}
	return n
}

// agentName is a sub-agent type as shown in reports.
func agentName(t string) string {
	if t == "unknown" || t == "" {
		return "type not recorded"
	}
	return t
}

// modelName is a model's name as people write it.
func modelName(m string) string { return price.DisplayName(m) }

// injectedNames describes the kinds of text Claude Code adds.
var injectedNames = map[string]string{
	"skill_listing":           "List of available skills",
	"invoked_skills":          "Skills that were used",
	"mcp_instructions_delta":  "MCP server instructions",
	"total_tokens_reminder":   "Token-count reminders",
	"queued_command":          "Background-task notices and messages from other agents",
	"deferred_tools_delta":    "Notes on tools loaded when needed",
	"agent_listing_delta":     "List of available sub-agents",
	"environment":             "Environment details (folder, date, system)",
	"edited_text_file":        "Notices of files changed outside Claude",
	"hook_additional_context": "Text added by hooks",
	"file":                    "Files you attached",
	"sandbox_instructions":    "Sandbox rules",
	"session_context":         "Session details",
	"date":                    "Today's date",
	"model":                   "Model details",
	"budget_usd":              "Budget reminders",
}

// contextName describes a source of standing or injected context.
func contextName(name string) string {
	if d, ok := injectedNames[name]; ok {
		return d
	}
	if rest, ok := strings.CutPrefix(name, "mcp__"); ok {
		return "MCP server " + serverName(rest) + ": tool definitions"
	}
	return strings.ReplaceAll(name, "_", " ")
}
