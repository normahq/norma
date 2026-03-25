package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	statusActive  = "active"
	statusStopped = "stopped"
	statusError   = "error"
)

// Record is a serializable session metadata entry.
type Record struct {
	SessionID    string `json:"session_id"`
	ChatID       int64  `json:"chat_id"`
	TopicID      int    `json:"topic_id"`
	AgentName    string `json:"agent_name"`
	WorkspaceDir string `json:"workspace_dir"`
	Status       string `json:"status"`
	UpdatedAt    string `json:"updated_at"`
}

type sessionSnapshot struct {
	Sessions []Record `json:"sessions"`
}

// Store persists topic session records as JSON.
type Store struct {
	path string
	mu   sync.Mutex
}

func newStore(normaDir string) (*Store, error) {
	if err := os.MkdirAll(normaDir, 0o755); err != nil {
		return nil, fmt.Errorf("create norma dir: %w", err)
	}
	return &Store{
		path: filepath.Join(normaDir, "relay_sessions.json"),
	}, nil
}

func (s *Store) Load() (map[string]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Record{}, nil
		}
		return nil, fmt.Errorf("read session store: %w", err)
	}
	if len(raw) == 0 {
		return map[string]Record{}, nil
	}

	var snap sessionSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, fmt.Errorf("decode session store: %w", err)
	}

	out := make(map[string]Record, len(snap.Sessions))
	for _, rec := range snap.Sessions {
		if rec.SessionID == "" {
			continue
		}
		out[rec.SessionID] = rec
	}
	return out, nil
}

func (s *Store) Save(records map[string]Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys := make([]string, 0, len(records))
	for k := range records {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	snap := sessionSnapshot{
		Sessions: make([]Record, 0, len(keys)),
	}
	for _, k := range keys {
		snap.Sessions = append(snap.Sessions, records[k])
	}

	blob, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session store: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o644); err != nil {
		return fmt.Errorf("write session store temp file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace session store: %w", err)
	}
	return nil
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
