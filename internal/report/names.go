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
func serverName(n string) string {
	switch {
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
