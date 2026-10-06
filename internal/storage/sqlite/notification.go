package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/px1/home-ctl/internal/notification"
)

// NotificationStore manages SQLite storage for notification attempts.
type NotificationStore struct {
	db *sql.DB
}

// NewNotificationStore creates a new NotificationStore.
func NewNotificationStore(db *sql.DB) *NotificationStore {
	return &NotificationStore{db: db}
}

// ClaimChoreAttempt transactionally records a claim for a chore notification attempt.
func (s *NotificationStore) ClaimChoreAttempt(ctx context.Context, occID, targetUserID int64, dueAt, attemptTime time.Time) (int64, error) {
	dueStr := dueAt.UTC().Format(time.RFC3339)
	attemptStr := attemptTime.UTC().Format(time.RFC3339)

	// Check if already claimed or sent
	var existingCount int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM notification_attempts
WHERE entity_type = 'chore' AND occurrence_id = ? AND due_at = ? AND outcome IN ('claimed', 'sent')`,
		occID, dueStr).Scan(&existingCount)
	if err != nil {
		return 0, err
	}
	if existingCount > 0 {
		return 0, notification.ErrAlreadyClaimed
	}

	res, err := s.db.ExecContext(ctx, `
INSERT INTO notification_attempts (
	entity_type, occurrence_id, target_user_id, due_at, attempt_time, attempt_seq, outcome, created_at
) VALUES ('chore', ?, ?, ?, ?, 1, 'claimed', ?)`,
		occID, targetUserID, dueStr, attemptStr, attemptStr)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return 0, notification.ErrAlreadyClaimed
		}
		return 0, fmt.Errorf("failed to claim chore attempt: %w", err)
	}

	return res.LastInsertId()
}

// ClaimReminderAttempt transactionally claims a reminder notification attempt with incremented sequence.
func (s *NotificationStore) ClaimReminderAttempt(ctx context.Context, occID, targetUserID int64, dueAt, attemptTime time.Time) (int64, int, error) {
	dueStr := dueAt.UTC().Format(time.RFC3339)
	attemptStr := attemptTime.UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var maxSeq sql.NullInt64
	err = tx.QueryRowContext(ctx, `
SELECT MAX(attempt_seq) FROM notification_attempts
WHERE entity_type = 'reminder' AND occurrence_id = ? AND due_at = ?`,
		occID, dueStr).Scan(&maxSeq)
	if err != nil {
		return 0, 0, err
	}

	seq := 1
	if maxSeq.Valid {
		seq = int(maxSeq.Int64) + 1
	}

	res, err := tx.ExecContext(ctx, `
INSERT INTO notification_attempts (
	entity_type, occurrence_id, target_user_id, due_at, attempt_time, attempt_seq, outcome, created_at
) VALUES ('reminder', ?, ?, ?, ?, ?, 'claimed', ?)`,
		occID, targetUserID, dueStr, attemptStr, seq, attemptStr)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to claim reminder attempt: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}

	return id, seq, nil
}

// UpdateAttemptOutcome updates the final outcome of an attempt.
func (s *NotificationStore) UpdateAttemptOutcome(ctx context.Context, attemptID int64, outcome, failureReason string, msgID *int64) error {
	query := `
UPDATE notification_attempts
SET outcome = ?, failure_reason = ?, telegram_message_id = ?
WHERE id = ?`

	_, err := s.db.ExecContext(ctx, query, outcome, failureReason, msgID, attemptID)
	return err
}

// ListAttempts returns all recorded notification attempts for an entity occurrence.
func (s *NotificationStore) ListAttempts(ctx context.Context, entityType string, occID int64) ([]notification.Attempt, error) {
	query := `
SELECT id, entity_type, occurrence_id, target_user_id, due_at, attempt_time,
       attempt_seq, outcome, COALESCE(failure_reason, ''), telegram_message_id, created_at
FROM notification_attempts
WHERE entity_type = ? AND occurrence_id = ?
ORDER BY attempt_seq ASC`

	rows, err := s.db.QueryContext(ctx, query, entityType, occID)
	if err != nil {
		return nil, fmt.Errorf("failed to list attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var attempts []notification.Attempt
	for rows.Next() {
		var a notification.Attempt
		var dueStr, attemptStr, createdStr string
		var msgID sql.NullInt64

		if err := rows.Scan(
			&a.ID, &a.EntityType, &a.OccurrenceID, &a.TargetUserID,
			&dueStr, &attemptStr, &a.AttemptSeq, &a.Outcome,
			&a.FailureReason, &msgID, &createdStr,
		); err != nil {
			return nil, err
		}

		a.DueAt, _ = time.Parse(time.RFC3339, dueStr)
		a.AttemptTime, _ = time.Parse(time.RFC3339, attemptStr)
		a.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		if msgID.Valid {
			val := msgID.Int64
			a.TelegramMessageID = &val
		}

		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}
