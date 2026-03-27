package state

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSQLiteProvider_KVRoundTrip(t *testing.T) {
	provider := newTestProvider(t)
	defer closeProvider(t, provider)

	ctx := context.Background()
	store := provider.SessionMCPKV()

	if err := store.Set(ctx, "alpha", "one"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, ok, err := store.Get(ctx, "alpha")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() found = false, want true")
	}
	if got != "one" {
		t.Fatalf("Get() value = %q, want %q", got, "one")
	}

	if err := store.SetJSON(ctx, "json", map[string]any{"count": 2}); err != nil {
		t.Fatalf("SetJSON() error = %v", err)
	}
	merged, err := store.MergeJSON(ctx, "json", map[string]any{"name": "relay"})
	if err != nil {
		t.Fatalf("MergeJSON() error = %v", err)
	}
	if merged["count"] != float64(2) {
		t.Fatalf("merged[count] = %v, want 2", merged["count"])
	}
	if merged["name"] != "relay" {
		t.Fatalf("merged[name] = %v, want relay", merged["name"])
	}
}

func TestSQLiteProvider_SessionStoreRoundTrip(t *testing.T) {
	provider := newTestProvider(t)
	defer closeProvider(t, provider)

	ctx := context.Background()
	store := provider.Sessions()

	record := SessionRecord{
		SessionID:    "relay-1-2",
		ChatID:       1,
		TopicID:      2,
		AgentName:    "agent",
		WorkspaceDir: "/tmp/ws",
		BranchName:   "norma/relay/relay-1-2",
		Status:       SessionStatusActive,
	}
	if err := store.Upsert(ctx, record); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	got, ok, err := store.GetByChatTopic(ctx, 1, 2)
	if err != nil {
		t.Fatalf("GetByChatTopic() error = %v", err)
	}
	if !ok {
		t.Fatal("GetByChatTopic() found = false, want true")
	}
	if got.SessionID != record.SessionID {
		t.Fatalf("session_id = %q, want %q", got.SessionID, record.SessionID)
	}
	if got.AgentName != record.AgentName {
		t.Fatalf("agent_name = %q, want %q", got.AgentName, record.AgentName)
	}
}

func TestSQLiteProvider_OffsetPersistsAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "relay.db")
	ctx := context.Background()

	providerA, err := NewSQLiteProvider(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteProvider(A) error = %v", err)
	}
	if err := providerA.PollingOffsetStore().Save(ctx, 99); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	closeProvider(t, providerA)

	providerB, err := NewSQLiteProvider(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteProvider(B) error = %v", err)
	}
	defer closeProvider(t, providerB)

	offset, err := providerB.PollingOffsetStore().Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if offset != 99 {
		t.Fatalf("offset = %d, want 99", offset)
	}
}

func newTestProvider(t *testing.T) Provider {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "relay.db")
	provider, err := NewSQLiteProvider(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteProvider() error = %v", err)
	}
	return provider
}

func closeProvider(t *testing.T, provider Provider) {
	t.Helper()
	if err := provider.Close(); err != nil {
		t.Fatalf("provider.Close() error = %v", err)
	}
}
