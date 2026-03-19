package codexacpbridge

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpServerIdentity struct {
	name    string
	version string
}

func parseMCPServerIdentity(result *mcp.InitializeResult) mcpServerIdentity {
	if result == nil || result.ServerInfo == nil {
		return mcpServerIdentity{}
	}
	return mcpServerIdentity{
		name:    strings.TrimSpace(result.ServerInfo.Name),
		version: strings.TrimSpace(result.ServerInfo.Version),
	}
}

func resolveAgentIdentity(requestedName string, identity mcpServerIdentity) (name string, version string) {
	name = strings.TrimSpace(requestedName)
	if name == "" {
		name = strings.TrimSpace(identity.name)
	}
	if name == "" {
		name = DefaultAgentName
	}

	version = strings.TrimSpace(identity.version)
	if version == "" {
		version = DefaultAgentVersion
	}
	return name, version
}
