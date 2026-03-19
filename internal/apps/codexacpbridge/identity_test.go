package codexacpbridge

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestParseMCPServerIdentity(t *testing.T) {
	t.Run("nil initialize result", func(t *testing.T) {
		got := parseMCPServerIdentity(nil)
		if got.name != "" || got.version != "" {
			t.Fatalf("parseMCPServerIdentity(nil) = %+v, want empty identity", got)
		}
	})

	t.Run("server info", func(t *testing.T) {
		got := parseMCPServerIdentity(&mcp.InitializeResult{
			ServerInfo: &mcp.Implementation{
				Name:    " codex-mcp-server ",
				Version: " " + testMCPVersion + " ",
			},
		})
		if got.name != "codex-mcp-server" {
			t.Fatalf("identity.name = %q, want %q", got.name, "codex-mcp-server")
		}
		if got.version != testMCPVersion {
			t.Fatalf("identity.version = %q, want %q", got.version, testMCPVersion)
		}
	})
}

func TestResolveAgentIdentity(t *testing.T) {
	t.Run("requested name overrides mcp name", func(t *testing.T) {
		name, version := resolveAgentIdentity("team-codex", mcpServerIdentity{
			name:    "codex-mcp-server",
			version: testMCPVersion,
		})
		if name != "team-codex" {
			t.Fatalf("name = %q, want %q", name, "team-codex")
		}
		if version != testMCPVersion {
			t.Fatalf("version = %q, want %q", version, testMCPVersion)
		}
	})

	t.Run("uses mcp identity by default", func(t *testing.T) {
		name, version := resolveAgentIdentity("", mcpServerIdentity{
			name:    "codex-mcp-server",
			version: testMCPVersion,
		})
		if name != "codex-mcp-server" {
			t.Fatalf("name = %q, want %q", name, "codex-mcp-server")
		}
		if version != testMCPVersion {
			t.Fatalf("version = %q, want %q", version, testMCPVersion)
		}
	})

	t.Run("falls back to bridge defaults", func(t *testing.T) {
		name, version := resolveAgentIdentity("", mcpServerIdentity{})
		if name != DefaultAgentName {
			t.Fatalf("name = %q, want %q", name, DefaultAgentName)
		}
		if version != DefaultAgentVersion {
			t.Fatalf("version = %q, want %q", version, DefaultAgentVersion)
		}
	})
}
