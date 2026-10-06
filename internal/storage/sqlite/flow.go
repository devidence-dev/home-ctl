package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/px1/home-ctl/internal/flow"
)

// FlowStore provides SQLite persistence for conversation flow state.
type FlowStore struct {
	db *sql.DB
}

// NewFlowStore creates a new FlowStore.
func NewFlowStore(db *sql.DB) *FlowStore {
	return &FlowStore{db: db}
}

// Set saves or updates flow state for a (user, chat, thread) key.
func (s *FlowStore) Set(ctx context.Context, key flow.Key, step string, payload []byte, expiresAt time.Time) error {
	nowStr := time.Now().UTC().Format(time.RFC3339)
	expStr := expiresAt.UTC().Format(time.RFC3339)

	query := `
INSERT INTO flow_state (
	user_id, chat_id, thread_id, step, payload, expires_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id, chat_id, thread_id) DO UPDATE SET
	step = excluded.step,
	payload = excluded.payload,
	expires_at = excluded.expires_at,
	updated_at = excluded.updated_at`

	_, err := s.db.ExecContext(ctx, query,
		key.UserID, key.ChatID, key.ThreadID, step, string(payload), expStr, nowStr, nowStr,
	)
	if err != nil {
		return fmt.Errorf("failed to save flow state: %w", err)
	}
	return nil
}

// Get retrieves active flow state for a key, checking expiration.
func (s *FlowStore) Get(ctx context.Context, key flow.Key, now time.Time) (*flow.State, error) {
	query := `
SELECT step, payload, expires_at
FROM flow_state
WHERE user_id = ? AND chat_id = ? AND thread_id = ?`

	var step, payloadStr, expStr string
	err := s.db.QueryRowContext(ctx, query, key.UserID, key.ChatID, key.ThreadID).Scan(&step, &payloadStr, &expStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, flow.ErrNoFlow
		}
		return nil, fmt.Errorf("failed to query flow state: %w", err)
	}

	expTime, err := time.Parse(time.RFC3339, expStr)
	if err != nil {
		return nil, fmt.Errorf("corrupt expires_at timestamp: %w", err)
	}

	if now.After(expTime) {
		// Expired: delete and return error
		_ = s.Delete(ctx, key)
		return nil, flow.ErrFlowExpired
	}

	return &flow.State{
		Key:       key,
		Step:      step,
		Payload:   []byte(payloadStr),
		ExpiresAt: expTime,
	}, nil
}

// Delete removes flow state for a key.
func (s *FlowStore) Delete(ctx context.Context, key flow.Key) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM flow_state
WHERE user_id = ? AND chat_id = ? AND thread_id = ?`, key.UserID, key.ChatID, key.ThreadID)
	return err
}

// PurgeExpired removes all conversation states where expires_at <= now.
func (s *FlowStore) PurgeExpired(ctx context.Context, now time.Time) (int64, error) {
	nowStr := now.UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `DELETE FROM flow_state WHERE expires_at <= ?`, nowStr)
	if err != nil {
		return 0, fmt.Errorf("failed to purge expired flows: %w", err)
	}
	return res.RowsAffected()
}
