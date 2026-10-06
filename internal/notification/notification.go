// Package notification defines notification attempt tracking, status updates,
// and the Notifier interface.
package notification

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrAlreadyClaimed is returned when an attempt for a chore due moment has already been claimed or sent.
	ErrAlreadyClaimed = errors.New("notification attempt already claimed or sent")
)

// Attempt represents a recorded attempt to deliver a private notification.
type Attempt struct {
	ID                int64     `json:"id"`
	EntityType        string    `json:"entity_type"` // "chore" or "reminder"
	OccurrenceID      int64     `json:"occurrence_id"`
	TargetUserID      int64     `json:"target_user_id"`
	DueAt             time.Time `json:"due_at"`
	AttemptTime       time.Time `json:"attempt_time"`
	AttemptSeq        int       `json:"attempt_seq"`
	Outcome           string    `json:"outcome"` // "claimed", "sent", "failed"
	FailureReason     string    `json:"failure_reason,omitempty"`
	TelegramMessageID *int64    `json:"telegram_message_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// Notifier sends private notifications and group topic updates to Telegram.
type Notifier interface {
	SendPrivateNotification(ctx context.Context, targetUserID int64, entityType string, occID int64, title string, dueAt time.Time) (int64, error)
	SendGroupTopicUpdate(ctx context.Context, topicID int64, text string) (int64, error)
}

// Store persists and manages notification attempts in SQLite.
type Store interface {
	ClaimChoreAttempt(ctx context.Context, occID, targetUserID int64, dueAt, attemptTime time.Time) (int64, error)
	ClaimReminderAttempt(ctx context.Context, occID, targetUserID int64, dueAt, attemptTime time.Time) (int64, int, error)
	UpdateAttemptOutcome(ctx context.Context, attemptID int64, outcome, failureReason string, msgID *int64) error
	ListAttempts(ctx context.Context, entityType string, occID int64) ([]Attempt, error)
}
