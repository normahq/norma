# Norma Relay (V1)

`norma relay serve` is a standalone Telegram relay server that binds Telegram chats/topics to ADK agents created by Norma's agent factory.

## Summary

- Runtime stack: `tgbotkit/runtime` + Google ADK runners.
- Main agent: profile-scoped relay orchestrator (`profiles.<profile>.relay`).
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

1. Embedded defaults (`cmd/norma/relay/relay.yaml`)
2. Optional `.norma/relay.yaml`
3. Environment variables (`NORMA_*`)

### Telegram settings

- `relay.telegram.token`: bot token (required)
- `relay.telegram.receiver_mode`: `polling|webhook` (default: `polling`)
- `relay.telegram.webhook_url`: required when receiver mode is `webhook`
- `relay.telegram.webhook_token`: optional webhook secret

### Relay settings

- `relay.auth.owner_token`: generated at runtime per server start
- `relay.mcp.address`: optional relay MCP HTTP endpoint
- `relay.internal_mcp.servers`: internal MCP server IDs to start with lifecycle

## Session Model

Session key:

- Root relay session: `(chat_id, topic_id=0)`
- Topic subagent session: `(chat_id, topic_id)`

Topic bindings are durable across restarts:

- Persistent store: `.norma/relay_sessions.json`
- Record shape:
  - `session_id`
  - `chat_id`
  - `topic_id`
  - `agent_name`
  - `workspace_dir`
  - `status` (`active|stopped|error`)
  - `updated_at`

On restart, relay does **not** restore ADK session history.

Restore strategy is **lazy**:

- Relay loads persisted binding metadata at startup.
- When the first message arrives for a persisted topic that has no in-memory runner, relay restores that topic session on demand.
- Relay sends topic notifications:
  - `Restoring agent session...`
  - `Done`

## Message Flow

1. User sends Telegram message.
2. Relay resolves session by `(chat_id, topic_id)`.
3. If topic session is missing in memory but has persisted binding metadata, relay lazily recreates it and notifies the topic.
4. Relay calls ADK runner for that session.
5. Relay streams events:
   - `part.Text` -> response draft (MarkdownV2)
   - `part.Thought` -> plain-text draft
   - `acp_tool_call` / `acp_tool_call_update` -> plain-text draft

Draft IDs are allocated per turn and separated by channel (response vs thought/event).

## Subagent Spawn

Two v1 spawn paths are supported:

1. Manual: `/new <agent_name>`
2. Agent/tool path: relay MCP `norma.relay.start_agent`

Both paths create:

- A new Telegram forum topic
- A topic-bound ADK session
- A dedicated worktree at `.norma/relay-workspaces/topic-<chat>-<topic>`

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
2. Polling mode starts by default without webhook config.
3. Webhook mode fails fast without `webhook_url`.
4. `/start <token>` registers owner once; non-owner traffic is rejected.
5. `/new <agent>` creates topic + durable session record.
6. Relay MCP `start_agent` creates topic + session and returns IDs.
7. Restart keeps persisted topic bindings but resets ADK session history.
8. First message to a persisted topic lazily restores session runtime and sends restore notifications.
9. Stream channel separation is preserved:
   - thoughts/tool events plain text
   - assistant response MarkdownV2
