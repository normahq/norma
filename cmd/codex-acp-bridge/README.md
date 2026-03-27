# codex-acp-bridge

`codex-acp-bridge` runs `codex mcp-server` and exposes it as an ACP agent over stdio.

## Installation

Global install (distributed via npm):

```bash
npm install -g @normahq/codex-acp-bridge@latest
```

One-off run with npx (no global install):

```bash
npx @normahq/codex-acp-bridge@latest
```

## Run

```bash
codex-acp-bridge
```

Examples:

```bash
codex-acp-bridge
codex-acp-bridge --name team-codex
codex-acp-bridge --codex-model gpt-5.4 --codex-sandbox workspace-write
codex-acp-bridge --debug
```

## Flags

- `--name`: ACP agent name override (default: MCP server name from `codex mcp-server` initialize metadata).
- `--codex-model`: model for MCP `codex` tool calls.
- `--codex-sandbox`: sandbox for MCP `codex` tool calls (`read-only|workspace-write|danger-full-access`).
- `--codex-approval-policy`: approval policy for MCP `codex` tool calls (`untrusted|on-failure|on-request|never`).
- `--codex-profile`: profile for MCP `codex` tool calls.
- `--codex-base-instructions`: base instructions for MCP `codex` tool calls.
- `--codex-developer-instructions`: developer instructions for MCP `codex` tool calls.
- `--codex-compact-prompt`: compact prompt for MCP `codex` tool calls.
- `--codex-config`: JSON object for MCP `codex` tool `config` field.
- `--debug`: enable debug logs.

## Behavior

- Validates that Codex MCP tools `codex` and `codex-reply` are available.
- Creates separate backend Codex MCP sessions per ACP session.
- Supports ACP `session/set_model` and propagates the selected model to new Codex tool calls.
- Accepts ACP `session/set_mode` and resets backend session/thread state, but does not propagate mode to Codex MCP tool arguments.
- Supports per-session MCP servers via ACP `session/new` `mcpServers` parameter (stdio and http transports). SSE transport is not supported, and each server entry must declare exactly one transport.
- For ACP `initialize.agentInfo`, forwards MCP `serverInfo.name` and `serverInfo.version` by default; `--name` overrides only the name.

## MCP Servers

The bridge supports passing MCP servers to the Codex tool via the ACP `session/new` request. On the first turn of a session (no thread ID), any MCP servers provided in the `mcpServers` parameter are translated into Codex config values under `config.mcp_servers`.

Example ACP session/new request with MCP servers:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "session/new",
  "params": {
    "cwd": "/workspace",
    "mcpServers": [
      {
        "stdio": {
          "name": "filesystem",
          "command": "npx",
          "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
        }
      },
      {
        "http": {
          "name": "github",
          "url": "https://api.github.com"
        }
      }
    ]
  }
}
```

Supported transports: `stdio`, `http`. The `sse` transport is explicitly rejected.

## Notes

- See also: `docs/codex-acp-bridge.md`.

## Repository

- Norma GitHub: <https://github.com/normahq/norma>

## Contact

- Issues: <https://github.com/normahq/norma/issues>
- Maintainer: [@metalagman](https://github.com/metalagman)

## License

MIT. See the repository [LICENSE](https://github.com/normahq/norma/blob/main/LICENSE).
