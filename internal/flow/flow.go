// Package flow manages temporary conversation state across multi-step Telegram interactions.
package flow

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	// ErrNoFlow indicates no active conversation state exists for the key.
	ErrNoFlow = errors.New("no active flow found")
	// ErrFlowExpired indicates conversation state has expired beyond its TTL.
	ErrFlowExpired = errors.New("flow has expired")
)

// Key uniquely scopes multi-step interaction state by user, chat, and topic.
type Key struct {
	UserID   int64 `json:"user_id"`
	ChatID   int64 `json:"chat_id"`
	ThreadID int64 `json:"thread_id"`
}

// State represents the serialized progress of an active user flow.
type State struct {
	Key       Key       `json:"key"`
	Step      string    `json:"step"`
	Payload   []byte    `json:"payload"`
	ExpiresAt time.Time `json:"expires_at"`
	MessageID int64     `json:"message_id,omitempty"`
}

// Nower provides current time.
type Nower interface {
	Now() time.Time
}

// Store persists and retrieves flow states.
type Store interface {
	Set(ctx context.Context, key Key, step string, payload []byte, expiresAt time.Time) error
	SetMessageID(ctx context.Context, key Key, messageID int64) error
	Get(ctx context.Context, key Key, now time.Time) (*State, error)
	GetByMessage(ctx context.Context, chatID int64, threadID int64, messageID int64, now time.Time) (*State, error)
	Delete(ctx context.Context, key Key) error
	PurgeExpired(ctx context.Context, now time.Time) (int64, error)
}

// Manager orchestrates flow operations with typed payload serialization.
type Manager struct {
	store Store
	clock Nower
}

// NewManager creates a flow Manager.
func NewManager(store Store, clock Nower) *Manager {
	return &Manager{
		store: store,
		clock: clock,
	}
}

// Set stores a state step and encodes payload as JSON with the given TTL.
func (m *Manager) Set(ctx context.Context, key Key, step string, payload interface{}, ttl time.Duration) error {
	var data []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		data = b
	} else {
		data = []byte("{}")
	}

	expiresAt := m.clock.Now().Add(ttl).UTC()
	return m.store.Set(ctx, key, step, data, expiresAt)
}

// SetMessageID sets the active bot message id for this flow.
func (m *Manager) SetMessageID(ctx context.Context, key Key, messageID int64) error {
	return m.store.SetMessageID(ctx, key, messageID)
}

// Get retrieves active state and unmarshals payload into target.
func (m *Manager) Get(ctx context.Context, key Key, target interface{}) (*State, error) {
	now := m.clock.Now().UTC()
	st, err := m.store.Get(ctx, key, now)
	if err != nil {
		return nil, err
	}

	if target != nil && len(st.Payload) > 0 {
		if err := json.Unmarshal(st.Payload, target); err != nil {
			return nil, err
		}
	}

	return st, nil
}

// GetByMessage retrieves active state by message ID and unmarshals payload into target.
func (m *Manager) GetByMessage(ctx context.Context, chatID int64, threadID int64, messageID int64, target interface{}) (*State, error) {
	now := m.clock.Now().UTC()
	st, err := m.store.GetByMessage(ctx, chatID, threadID, messageID, now)
	if err != nil {
		return nil, err
	}

	if target != nil && len(st.Payload) > 0 {
		if err := json.Unmarshal(st.Payload, target); err != nil {
			return nil, err
		}
	}

	return st, nil
}

// Clear deletes active flow state for a key upon completion or cancellation.
func (m *Manager) Clear(ctx context.Context, key Key) error {
	return m.store.Delete(ctx, key)
}

// Purge removes all expired conversation states.
func (m *Manager) Purge(ctx context.Context) (int64, error) {
	return m.store.PurgeExpired(ctx, m.clock.Now().UTC())
}
