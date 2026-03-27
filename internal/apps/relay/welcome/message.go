package welcome

import (
	"fmt"
	"strings"
)

// BuildAgentWelcomeMessage returns the markdown-formatted session welcome text.
func BuildAgentWelcomeMessage(agentName, sessionID, agentDesc string, mcpServers []string) string {
	var b strings.Builder
	if strings.TrimSpace(sessionID) == "" {
		fmt.Fprintf(&b, "🤖 Starting **%s** agent session.\n\n**Description:** %s", agentName, agentDesc)
	} else {
		fmt.Fprintf(&b, "🤖 Started new **%s** agent session (%s).\n\n**Description:** %s", agentName, sessionID, agentDesc)
	}
	if len(mcpServers) > 0 {
		fmt.Fprintf(&b, "\n\n**MCP Servers:**\n")
		for _, s := range mcpServers {
			fmt.Fprintf(&b, "- %s\n", s)
		}
	}
	return b.String()
}
