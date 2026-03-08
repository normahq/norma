# Codex ACP Proxy

This command runs `codex mcp-server` and exposes it as an ACP agent over stdio.

Command:

```bash
norma proxy codex-acp
```

## Why this exists

- Norma ACP runners need an ACP endpoint.
- Codex CLI exposes MCP (`codex mcp-server`), not ACP directly.
- `norma proxy codex-acp` bridges MCP tools (`codex`, `codex-reply`) into ACP session calls.

## Usage

```bash
# Start proxy with defaults
norma proxy codex-acp

# Set ACP agent name seen by ACP clients in initialize.agentInfo.name
norma proxy codex-acp --name team-codex

# Pass extra flags to codex mcp-server (everything after -- is forwarded)
norma proxy codex-acp -- --trace --raw
```

## Flags

- `--name`:
  ACP agent name reported in `initialize.agentInfo.name`.
  Default: `norma-codex-acp-proxy`.
- `--` separator:
  All arguments after `--` are forwarded directly to `codex mcp-server`.

## Behavior

- Starts `codex mcp-server` in the current working directory.
- Verifies required MCP tools are present: `codex` and `codex-reply`.
- Opens ACP agent-side stdio connection for clients.
- For each ACP session:
  - first prompt calls MCP tool `codex` (new thread)
  - next prompts call MCP tool `codex-reply` (same thread)
- Supports ACP cancellation via `session/cancel`.

## Exit behavior

- Returns non-zero if Codex MCP server exits unexpectedly or bridge setup fails.
- Returns zero when ACP client disconnects normally.
