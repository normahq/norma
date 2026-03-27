package state

import (
	"context"

	"github.com/tgbotkit/runtime/updatepoller"
)

const (
	// NamespaceApp stores relay app internal state (for example owner auth).
	NamespaceApp = "relay.app"
	// NamespaceSessionMCP stores norma.state MCP key-value data.
	NamespaceSessionMCP = "relay.session_mcp"

	// SessionStatusActive marks a session that can be lazily restored.
	SessionStatusActive = "active"
)

// Provider exposes relay state capabilities behind a backend-agnostic interface.
// This allows swapping SQLite with another provider later.
type Provider interface {
	AppKV() KVStore
	SessionMCPKV() KVStore
	Sessions() SessionStore
	PollingOffsetStore() updatepoller.OffsetStore
	Close() error
}

// KVStore stores string and JSON key/value records.
type KVStore interface {
	Get(ctx context.Context, key string) (value string, ok bool, err error)
	Set(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
	Clear(ctx context.Context) error
	GetJSON(ctx context.Context, key string) (value any, ok bool, err error)
	SetJSON(ctx context.Context, key string, value any) error
	MergeJSON(ctx context.Context, key string, fields map[string]any) (merged map[string]any, err error)
}

// SessionRecord persists relay session metadata for lazy restore.
type SessionRecord struct {
	SessionID    string
	ChatID       int64
	TopicID      int
	AgentName    string
	WorkspaceDir string
	BranchName   string
	Status       string
}

// SessionStore persists relay session metadata.
type SessionStore interface {
	Upsert(ctx context.Context, record SessionRecord) error
	GetByChatTopic(ctx context.Context, chatID int64, topicID int) (SessionRecord, bool, error)
	DeleteBySessionID(ctx context.Context, sessionID string) error
	List(ctx context.Context) ([]SessionRecord, error)
}
