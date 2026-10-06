package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// TelegramStore manages processed update IDs in SQLite to guarantee idempotency.
type TelegramStore struct {
	db *sql.DB
}

// NewTelegramStore creates a new TelegramStore.
func NewTelegramStore(db *sql.DB) *TelegramStore {
	return &TelegramStore{db: db}
}

// IsUpdateProcessed checks whether a given Telegram update ID has already been handled.
func (s *TelegramStore) IsUpdateProcessed(ctx context.Context, updateID int64) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processed_updates WHERE update_id = ?`, updateID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check processed update %d: %w", updateID, err)
	}
	return count > 0, nil
}

// MarkUpdateProcessed records an update ID as processed.
func (s *TelegramStore) MarkUpdateProcessed(ctx context.Context, updateID int64) error {
	nowStr := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO processed_updates (update_id, processed_at)
VALUES (?, ?)`, updateID, nowStr)
	if err != nil {
		return fmt.Errorf("failed to record processed update %d: %w", updateID, err)
	}
	return nil
}
