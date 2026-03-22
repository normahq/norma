package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Owner represents the authenticated admin user.
type Owner struct {
	UserID       int64     `json:"user_id"`
	Username     string    `json:"username,omitempty"`
	FirstName    string    `json:"first_name,omitempty"`
	LastName     string    `json:"last_name,omitempty"`
	RegisteredAt time.Time `json:"registered_at"`
}

// OwnerStore manages owner persistence.
type OwnerStore struct {
	storePath string
	owner     *Owner
}

// NewOwnerStore creates a new owner store.
func NewOwnerStore(normaDir string) (*OwnerStore, error) {
	storePath := filepath.Join(normaDir, "relay_owner.json")

	store := &OwnerStore{
		storePath: storePath,
	}

	// Try to load existing owner
	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load owner: %w", err)
	}

	return store, nil
}

// RegisterOwner registers a new owner if none exists.
// Returns true if registered, false if already exists.
func (s *OwnerStore) RegisterOwner(userID int64, username, firstName, lastName string) (bool, error) {
	if s.owner != nil {
		return false, nil
	}

	s.owner = &Owner{
		UserID:       userID,
		Username:     username,
		FirstName:    firstName,
		LastName:     lastName,
		RegisteredAt: time.Now(),
	}

	if err := s.save(); err != nil {
		return false, fmt.Errorf("save owner: %w", err)
	}

	return true, nil
}

// IsOwner checks if the given user ID is the registered owner.
func (s *OwnerStore) IsOwner(userID int64) bool {
	if s.owner == nil {
		return false
	}
	return s.owner.UserID == userID
}

// GetOwner returns the registered owner, or nil if none exists.
func (s *OwnerStore) GetOwner() *Owner {
	return s.owner
}

// HasOwner returns true if an owner is registered.
func (s *OwnerStore) HasOwner() bool {
	return s.owner != nil
}

func (s *OwnerStore) load() error {
	data, err := os.ReadFile(s.storePath)
	if err != nil {
		return err
	}

	var owner Owner
	if err := json.Unmarshal(data, &owner); err != nil {
		return fmt.Errorf("unmarshal owner: %w", err)
	}

	s.owner = &owner
	return nil
}

func (s *OwnerStore) save() error {
	// Ensure directory exists
	dir := filepath.Dir(s.storePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	data, err := json.MarshalIndent(s.owner, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal owner: %w", err)
	}

	if err := os.WriteFile(s.storePath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
