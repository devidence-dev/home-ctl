// Package reminders implements reminder definitions, occurrences, target-only
// completions/postponements, and repeat notification scheduling logic.
package reminders

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/recurrence"
)

var (
	// ErrBlankTitle is returned when creating a reminder with an empty title.
	ErrBlankTitle = errors.New("reminder title cannot be blank")
	// ErrPastDueTime is returned when scheduling a reminder in the past.
	ErrPastDueTime = errors.New("due time cannot be in the past")
	// ErrNotTarget is returned when an actor other than the target member attempts resolution.
	ErrNotTarget = errors.New("only the target member can complete or postpone this reminder")
	// ErrOccurrenceNotFound is returned when an occurrence does not exist.
	ErrOccurrenceNotFound = errors.New("reminder occurrence not found")
	// ErrOccurrenceResolved is returned when an action needs an open occurrence that is already completed.
	ErrOccurrenceResolved = errors.New("reminder occurrence is already resolved")
	// ErrDefinitionNotFound is returned when a definition does not exist.
	ErrDefinitionNotFound = errors.New("reminder definition not found")
	// ErrNotMember is returned when an actor or target is not an active household member.
	ErrNotMember = errors.New("user is not an active household member")
)

// Definition represents a reminder template.
type Definition struct {
	ID                 int64      `json:"id"`
	Title              string     `json:"title"`
	Emoji              string     `json:"emoji"`
	Description        string     `json:"description"`
	TargetUserID       int64      `json:"target_user_id"`
	RecurrenceInterval *int       `json:"recurrence_interval,omitempty"`
	RecurrenceUnit     *string    `json:"recurrence_unit,omitempty"`
	RecurrenceAnchor   *string    `json:"recurrence_anchor,omitempty"`
	SeriesOrigin       *time.Time `json:"series_origin,omitempty"`
	CreatedBy          int64      `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// Occurrence represents a scheduled reminder instance.
type Occurrence struct {
	ID           int64       `json:"id"`
	DefinitionID int64       `json:"definition_id"`
	Definition   *Definition `json:"definition,omitempty"`
	TargetUserID int64       `json:"target_user_id"`
	ScheduledAt  time.Time   `json:"scheduled_at"`
	DueAt        time.Time   `json:"due_at"`
	Status       string      `json:"status"` // "open" or "completed"
	CompletedBy  *int64      `json:"completed_by,omitempty"`
	CompletedAt  *time.Time  `json:"completed_at,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// ListResult holds user-specific open occurrences and metrics.
type ListResult struct {
	Occurrences    []Occurrence `json:"occurrences"`
	TotalOpen      int          `json:"total_open"`
	RemainingCount int          `json:"remaining_count"`
}

// MemberChecker verifies member authorization.
type MemberChecker interface {
	Authorize(ctx context.Context, userID int64) (*household.Member, error)
}

// Clock provides current time and timezone info.
type Clock interface {
	Now() time.Time
	Location() *time.Location
}

// Store defines persistence operations for reminders.
type Store interface {
	CreateDefinition(ctx context.Context, def Definition) (*Definition, error)
	GetDefinition(ctx context.Context, id int64) (*Definition, error)
	ListDefinitions(ctx context.Context) ([]Definition, error)
	CreateOccurrence(ctx context.Context, occ Occurrence) (*Occurrence, error)
	GetOccurrence(ctx context.Context, id int64) (*Occurrence, error)
	CompleteOccurrence(ctx context.Context, occID, actorID int64, completedAt time.Time, nextSlot *time.Time) (*Occurrence, *Occurrence, error)
	PostponeOccurrence(ctx context.Context, occID, actorID int64, newDueAt, now time.Time) (*Occurrence, error)
	ListMyOpenOccurrences(ctx context.Context, memberID int64, limit int) (ListResult, error)
	ListDueOccurrences(ctx context.Context, dueBefore time.Time) ([]Occurrence, error)
	ListUpcomingOccurrences(ctx context.Context, from, to time.Time) ([]Occurrence, error)
	GetLastNotificationTime(ctx context.Context, occID int64) (*time.Time, error)
}

// Service manages reminder operations.
type Service struct {
	store   Store
	members MemberChecker
	clock   Clock
}

// NewService constructs a reminder Service.
func NewService(store Store, members MemberChecker, clock Clock) *Service {
	return &Service{
		store:   store,
		members: members,
		clock:   clock,
	}
}

// CreateDefinition registers a reminder definition.
func (s *Service) CreateDefinition(ctx context.Context, actorID, targetUserID int64, title, emoji, desc string, rule *recurrence.Rule) (*Definition, error) {
	if _, err := s.members.Authorize(ctx, actorID); err != nil {
		return nil, ErrNotMember
	}
	if _, err := s.members.Authorize(ctx, targetUserID); err != nil {
		return nil, ErrNotMember
	}

	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return nil, ErrBlankTitle
	}
	if emoji == "" {
		emoji = "⏰"
	}

	now := s.clock.Now().UTC()
	def := Definition{
		Title:        trimmed,
		Emoji:        emoji,
		Description:  strings.TrimSpace(desc),
		TargetUserID: targetUserID,
		CreatedBy:    actorID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if rule != nil {
		if err := recurrence.Validate(*rule); err != nil {
			return nil, err
		}
		interval := rule.Interval
		unit := string(rule.Unit)
		anchor := string(rule.Anchor)
		def.RecurrenceInterval = &interval
		def.RecurrenceUnit = &unit
		def.RecurrenceAnchor = &anchor
	}

	return s.store.CreateDefinition(ctx, def)
}

// ListDefinitions returns all reminder definitions.
func (s *Service) ListDefinitions(ctx context.Context) ([]Definition, error) {
	return s.store.ListDefinitions(ctx)
}

// ScheduleOccurrence schedules a reminder occurrence for a target member.
func (s *Service) ScheduleOccurrence(ctx context.Context, actorID, defID int64, dueAt time.Time) (*Occurrence, error) {
	if _, err := s.members.Authorize(ctx, actorID); err != nil {
		return nil, ErrNotMember
	}

	now := s.clock.Now()
	if dueAt.Before(now) {
		return nil, ErrPastDueTime
	}

	def, err := s.store.GetDefinition(ctx, defID)
	if err != nil {
		return nil, ErrDefinitionNotFound
	}

	nowUTC := now.UTC()
	occ := Occurrence{
		DefinitionID: def.ID,
		TargetUserID: def.TargetUserID,
		ScheduledAt:  dueAt.UTC(),
		DueAt:        dueAt.UTC(),
		Status:       "open",
		CreatedAt:    nowUTC,
		UpdatedAt:    nowUTC,
	}

	return s.store.CreateOccurrence(ctx, occ)
}

// Complete completes a reminder. Only the target member can complete it.
func (s *Service) Complete(ctx context.Context, actorID, occID int64, completedAt time.Time) (*Occurrence, *Occurrence, error) {
	if _, err := s.members.Authorize(ctx, actorID); err != nil {
		return nil, nil, ErrNotMember
	}

	occ, err := s.store.GetOccurrence(ctx, occID)
	if err != nil {
		return nil, nil, ErrOccurrenceNotFound
	}

	if occ.TargetUserID != actorID {
		return nil, nil, ErrNotTarget
	}

	if occ.Status == "completed" {
		return occ, nil, nil
	}

	def, err := s.store.GetDefinition(ctx, occ.DefinitionID)
	if err != nil {
		return nil, nil, ErrDefinitionNotFound
	}

	var nextSlot *time.Time
	if def.RecurrenceInterval != nil && def.RecurrenceUnit != nil && def.RecurrenceAnchor != nil {
		rule := recurrence.Rule{
			Interval: *def.RecurrenceInterval,
			Unit:     recurrence.Unit(*def.RecurrenceUnit),
			Anchor:   recurrence.Anchor(*def.RecurrenceAnchor),
		}
		origin := occ.ScheduledAt
		if def.SeriesOrigin != nil {
			origin = *def.SeriesOrigin
		}
		next, err := recurrence.NextSlot(rule, origin, occ.ScheduledAt, completedAt, s.clock.Location())
		if err != nil {
			return nil, nil, err
		}
		nextSlot = &next
	}

	return s.store.CompleteOccurrence(ctx, occID, actorID, completedAt, nextSlot)
}

// Postpone postpones a reminder. Only the target member can postpone it.
func (s *Service) Postpone(ctx context.Context, actorID, occID int64, newDueAt time.Time) (*Occurrence, error) {
	if _, err := s.members.Authorize(ctx, actorID); err != nil {
		return nil, ErrNotMember
	}

	occ, err := s.store.GetOccurrence(ctx, occID)
	if err != nil {
		return nil, ErrOccurrenceNotFound
	}

	// Authorize before validating the requested time so non-owners never learn about time errors.
	if occ.TargetUserID != actorID {
		return nil, ErrNotTarget
	}

	// Only open occurrences can move; the store repeats this check atomically.
	if occ.Status != "open" {
		return nil, ErrOccurrenceResolved
	}

	now := s.clock.Now()
	if newDueAt.Before(now) {
		return nil, ErrPastDueTime
	}

	return s.store.PostponeOccurrence(ctx, occID, actorID, newDueAt.UTC(), now.UTC())
}

// ListMyOpen lists reminders targeting the requesting member.
func (s *Service) ListMyOpen(ctx context.Context, memberID int64) (ListResult, error) {
	if _, err := s.members.Authorize(ctx, memberID); err != nil {
		return ListResult{}, ErrNotMember
	}
	return s.store.ListMyOpenOccurrences(ctx, memberID, 20)
}

// GetOccurrence retrieves a reminder occurrence by ID.
func (s *Service) GetOccurrence(ctx context.Context, occID int64) (*Occurrence, error) {
	return s.store.GetOccurrence(ctx, occID)
}

// IsNotificationDue determines whether an unresolved reminder should be notified right now
// according to the configured repeat interval.
func (s *Service) IsNotificationDue(ctx context.Context, occ *Occurrence, interval time.Duration, now time.Time) (bool, error) {
	if occ.Status != "open" {
		return false, nil
	}
	if now.Before(occ.DueAt) {
		return false, nil
	}

	lastNotified, err := s.store.GetLastNotificationTime(ctx, occ.ID)
	if err != nil {
		return false, err
	}
	if lastNotified == nil {
		// Not notified yet for current due_at
		return true, nil
	}

	// Repeat if current time has reached lastNotified + interval
	return !now.Before(lastNotified.Add(interval)), nil
}

// ListUpcoming returns open reminders occurrences due in [from, to), ordered by due_at.
// It only reads; it never schedules, moves or resolves anything.
func (s *Service) ListUpcoming(ctx context.Context, from, to time.Time) ([]Occurrence, error) {
	return s.store.ListUpcomingOccurrences(ctx, from.UTC(), to.UTC())
}
