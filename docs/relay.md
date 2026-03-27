# Norma Relay (V1)

`relay serve` is a standalone Telegram relay server that binds Telegram chats/topics to ADK agents created by Norma's agent factory.

## Summary

- Runtime stack: `tgbotkit/runtime` + Google ADK runners.
- Main agent: relay app key `relay.orchestrator_agent` (profile overrides via `profiles.<profile>.relay.orchestrator_agent`).
- Subagents: one session per Telegram topic (`message_thread_id`) with dedicated git worktree.
- Output streaming: dual `sendMessageDraft` channels.
  - Response channel: MarkdownV2 escaped text.
  - Thoughts/events channel: plain text.
- Auth model: one-time owner authorization with startup-generated token.

## Startup Order (Required)

Relay startup order is strict:

1. Load Norma + relay config.
2. Start internal MCP lifecycle manager.
3. Start relay orchestrator agent via `agentfactory.Factory`.
4. Start Telegram runtime receiver.

Internal MCP v1 scope is config + lifecycle plumbing; server implementations can be added incrementally.

## Configuration

Relay config is merged from:

1. Embedded defaults (`cmd/relay/relay.yaml`)
2. Runtime config in `.norma/relay.yaml` or `.norma/config.yaml` (`relay.*`)
3. Profile app overrides in the same file (`profiles.<name>.relay.*`)
4. Environment variables (`RELAY_*`) via Viper env mapping

### Telegram settings

- `relay.telegram.token`: bot token (required)
- `relay.telegram.webhook.enabled`: enable local HTTP webhook endpoint (`true` => webhook mode, `false` => polling mode; default: `false`)
- `relay.telegram.webhook.url`: outgoing Telegram webhook URL (required when `relay.telegram.webhook.enabled=true`)
- `relay.telegram.webhook.secret_token`: optional webhook secret token
- `relay.telegram.webhook.listen_addr`: local webhook listen address (default: `0.0.0.0:8080`)
- `relay.telegram.webhook.path`: local webhook path (default: `/telegram/webhook`)

### Relay settings

- `relay.working_dir`: optional relay working directory (defaults to process CWD)
- `relay.state_dir`: relay state directory for persistent relay SQLite state (`relay.db`).
  - Stores owner/app KV, `norma.state` MCP KV, session metadata, and Telegram polling offset.
  - Relative paths are resolved from `relay.working_dir`.
- `relay.auth.owner_token`: generated at runtime per server start
- `relay.mcp.address`: optional relay MCP HTTP endpoint
- `relay.workspace.mode`: `on|off|auto` (default `auto`)
  - `on`: always use Git worktrees per session; startup fails if `working_dir` is not a Git repository
  - `off`: run agents directly in relay `working_dir` (no `norma.workspace` MCP)
  - `auto`: enable worktrees only when `working_dir` is a Git repo, otherwise fallback to `off`
- `relay.internal_mcp.servers`: internal MCP server IDs to start with lifecycle
- Relay is Beads-independent by default and does not auto-start bundled `norma.tasks` MCP.

## Session Model

Session key:

- Root relay session: `(chat_id, topic_id=0)`
- Topic subagent session: `(chat_id, topic_id)`

Session runtimes are still in-memory, but metadata is persisted in `relay.db`.
Relay lazy-restores a topic session on first message after restart when metadata exists.

## Message Flow

1. User sends Telegram message.
2. Relay resolves session by `(chat_id, topic_id)`.
3. If topic session is missing in memory, relay attempts lazy restore from persisted metadata.
4. Relay calls ADK runner for that session.
5. Relay streams partial updates to Telegram using Bot API `sendMessageDraft`.

## Telegram Client Draft API

Partial model-response updates MUST be sent through the Telegram client method for Bot API `sendMessageDraft` (not `sendMessage`).

### Request fields

- `chat_id` (required)
- `draft_id` (required, non-zero)
- `text` (required)
- `message_thread_id` (required for topic replies)
- `parse_mode` (optional; use `MarkdownV2` for assistant response text)

### Two draft streams per turn

- Response draft stream:
  - Contains assistant response text.
  - Uses MarkdownV2-escaped text.
- Events draft stream:
  - Contains thoughts and tool-event updates.
  - Uses plain text (no `parse_mode`).

### Draft lifecycle contract

1. At turn start, allocate two separate non-zero `draft_id` values:
   - one for response
   - one for events
2. During the same turn, send each partial update to its stream using the same stream `draft_id`.
3. Keep re-sending with the same `draft_id` as new partial text arrives (Telegram animates draft updates with the same ID).
4. When the model turn completes, stop sending updates for both stream draft IDs.
5. The last update sent for each stream is the final visible draft state for that turn.

### Error handling

- For response stream with `MarkdownV2`:
  - If request fails, retry once without `parse_mode`.
- Treat API bad requests or missing success body as failed draft updates and report/log them.

## Subagent Spawn

Two v1 spawn paths are supported:

1. Manual: `/new <agent_name>`
2. Agent/tool path: relay MCP `norma.relay.start_agent`

Both paths create:

- A new Telegram forum topic
- A topic-bound ADK session
- A dedicated Git worktree when `relay.workspace.mode` resolves to enabled

## Relay MCP API (V1)

- `norma.relay.start_agent`
  - input: `chat_id`, `agent_name`
  - output: `session_id`, `topic_id`, `chat_id`, `agent_name`
- `norma.relay.stop_agent`
  - input: `session_id`
- `norma.relay.list_agents`
- `norma.relay.get_agent`
  - input: `session_id`

## Acceptance/Verification Scenarios

1. Startup order enforces internal MCP -> relay agent -> bot runtime.
2. Polling mode starts by default when `relay.telegram.webhook.enabled=false`.
3. Webhook mode (`relay.telegram.webhook.enabled=true`) fails fast without `relay.telegram.webhook.url`.
4. `/start <token>` registers owner once; non-owner traffic is rejected.
5. `/new <agent>` creates topic + relay session and persists session metadata.
6. Relay MCP `start_agent` creates topic + session and returns IDs.
7. Restart clears in-memory sessions but topic sessions are lazy-restored from persisted metadata.
8. Polling mode resumes from persisted Telegram offset in relay state DB.
9. Partial updates are sent with Telegram Bot API `sendMessageDraft` (not `sendMessage`).
10. Per turn, two non-zero draft streams are used and kept separate (response vs events).
11. Reusing the same `draft_id` within each stream updates the same animated draft message.
12. Response stream uses MarkdownV2 (with fallback retry without `parse_mode`), while events stream stays plain text.
