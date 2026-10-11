// Package scheduler coordinates periodic due-occurrence scans, transactional attempt claims,
// and notification dispatch.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/px1/home-ctl/internal/chores"
	"github.com/px1/home-ctl/internal/notification"
	"github.com/px1/home-ctl/internal/reminders"
)

// Nower provides current time.
type Nower interface {
	Now() time.Time
}

// OnboardingChecker verifies if a member has a connected private chat.
type OnboardingChecker interface {
	IsConnected(ctx context.Context, userID int64) (bool, error)
	HandleDisconnect(ctx context.Context, userID int64) error
}

// Scheduler runs due scans for chores and reminders.
type Scheduler struct {
	choresStore      chores.Store
	remindersStore   reminders.Store
	remindersService *reminders.Service
	notifStore       notification.Store
	notifier         notification.Notifier
	onboarding       OnboardingChecker
	clock            Nower
	reminderInterval time.Duration
	logger           *slog.Logger
}

// Config holds dependencies for constructing a Scheduler.
type Config struct {
	ChoresStore      chores.Store
	RemindersStore   reminders.Store
	RemindersService *reminders.Service
	NotifStore       notification.Store
	Notifier         notification.Notifier
	Onboarding       OnboardingChecker
	Clock            Nower
	ReminderInterval time.Duration
	Logger           *slog.Logger
}

// New constructs a Scheduler.
func New(cfg Config) *Scheduler {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		choresStore:      cfg.ChoresStore,
		remindersStore:   cfg.RemindersStore,
		remindersService: cfg.RemindersService,
		notifStore:       cfg.NotifStore,
		notifier:         cfg.Notifier,
		onboarding:       cfg.Onboarding,
		clock:            cfg.Clock,
		reminderInterval: cfg.ReminderInterval,
		logger:           logger,
	}
}

// ScanAndNotify checks for all open chores and reminders that are due and dispatches notifications.
func (s *Scheduler) ScanAndNotify(ctx context.Context) error {
	now := s.clock.Now().UTC()

	// 1. Process due chores
	dueChores, err := s.choresStore.ListDueOccurrences(ctx, now)
	if err != nil {
		return fmt.Errorf("failed to list due chores: %w", err)
	}

	for _, chore := range dueChores {
		attemptID, err := s.notifStore.ClaimChoreAttempt(ctx, chore.ID, chore.AssigneeID, chore.DueAt, now)
		if err != nil {
			if errors.Is(err, notification.ErrAlreadyClaimed) {
				continue
			}
			s.logger.Error("failed to claim chore attempt", "chore_id", chore.ID, "err", err)
			continue
		}

		s.dispatchPrivateNotification(ctx, attemptID, chore.AssigneeID, "chore", chore.ID, chore.Definition.Name, chore.DueAt)
	}

	// 2. Process due reminders
	dueReminders, err := s.remindersStore.ListDueOccurrences(ctx, now)
	if err != nil {
		return fmt.Errorf("failed to list due reminders: %w", err)
	}

	for _, rem := range dueReminders {
		isDue, err := s.remindersService.IsNotificationDue(ctx, &rem, s.reminderInterval, now)
		if err != nil {
			s.logger.Error("failed to check reminder notification due", "reminder_id", rem.ID, "err", err)
			continue
		}
		if !isDue {
			continue
		}

		attemptID, _, err := s.notifStore.ClaimReminderAttempt(ctx, rem.ID, rem.TargetUserID, rem.DueAt, now)
		if err != nil {
			s.logger.Error("failed to claim reminder attempt", "reminder_id", rem.ID, "err", err)
			continue
		}

		s.dispatchPrivateNotification(ctx, attemptID, rem.TargetUserID, "reminder", rem.ID, rem.Definition.Title, rem.DueAt)
	}

	return nil
}

func (s *Scheduler) dispatchPrivateNotification(ctx context.Context, attemptID, targetUserID int64, entityType string, occID int64, title string, dueAt time.Time) {
	conn, err := s.onboarding.IsConnected(ctx, targetUserID)
	if err != nil || !conn {
		// Target user not connected: record failure without resolving occurrence
		_ = s.notifStore.UpdateAttemptOutcome(ctx, attemptID, "failed", "not connected", nil)
		s.logger.Info("notification skipped: user not connected", "user_id", targetUserID, "entity", entityType, "occ_id", occID)
		return
	}

	msgID, err := s.notifier.SendPrivateNotification(ctx, targetUserID, entityType, occID, title, dueAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "blocked") {
			_ = s.onboarding.HandleDisconnect(ctx, targetUserID)
		}
		_ = s.notifStore.UpdateAttemptOutcome(ctx, attemptID, "failed", err.Error(), nil)
		s.logger.Error("notification delivery failed", "user_id", targetUserID, "err", err)
		return
	}

	_ = s.notifStore.UpdateAttemptOutcome(ctx, attemptID, "sent", "", &msgID)
}

// PostTopicStatusUpdate safely sends a group topic status update after domain commit,
// recording any failure without rolling back committed state.
func PostTopicStatusUpdate(ctx context.Context, notifier notification.Notifier, topicID int64, text string, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	_, err := notifier.SendGroupTopicUpdate(ctx, topicID, text)
	if err != nil {
		logger.Error("failed to post group topic status update", "topic_id", topicID, "err", err)
	}
}
