// Package chores implements chore definition cataloging, scheduling, assignee-only
// completions, postponements, and user-scoped list retrieval.
package chores

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/recurrence"
)

var (
	// ErrBlankName is returned when creating a chore definition with an empty name.
	ErrBlankName = errors.New("chore name cannot be blank")
	// ErrPastDueTime is returned when scheduling a chore in the past.
	ErrPastDueTime = errors.New("due time cannot be in the past")
	// ErrNotAssignee is returned when an actor other than the assignee attempts completion or postponement.
	ErrNotAssignee = errors.New("only the assignee can complete or postpone this chore")
	// ErrOccurrenceNotFound is returned when an occurrence does not exist.
	ErrOccurrenceNotFound = errors.New("chore occurrence not found")
	// ErrOccurrenceResolved is returned when an action needs an open occurrence that is already completed.
	ErrOccurrenceResolved = errors.New("chore occurrence is already resolved")
	// ErrDefinitionNotFound is returned when a definition does not exist.
	ErrDefinitionNotFound = errors.New("chore definition not found")
	// ErrNotMember is returned when an actor or assignee is not an active household member.
	ErrNotMember = errors.New("user is not an active household member")
)

// Definition represents a reusable chore template.
type Definition struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Emoji              string     `json:"emoji"`
	Description        string     `json:"description"`
	RecurrenceInterval *int       `json:"recurrence_interval,omitempty"`
	RecurrenceUnit     *string    `json:"recurrence_unit,omitempty"`
	RecurrenceAnchor   *string    `json:"recurrence_anchor,omitempty"`
	SeriesOrigin       *time.Time `json:"series_origin,omitempty"`
	CreatedBy          int64      `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// Occurrence represents a specific scheduled chore instance.
type Occurrence struct {
	ID           int64       `json:"id"`
	DefinitionID int64       `json:"definition_id"`
	Definition   *Definition `json:"definition,omitempty"`
	AssigneeID   int64       `json:"assignee_id"`
	ScheduledAt  time.Time   `json:"scheduled_at"`
	DueAt        time.Time   `json:"due_at"`
	Status       string      `json:"status"` // "open" or "completed"
	CompletedBy  *int64      `json:"completed_by,omitempty"`
	CompletedAt  *time.Time  `json:"completed_at,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// ListResult holds user-specific open occurrences and truncation metrics.
type ListResult struct {
	Occurrences    []Occurrence `json:"occurrences"`
	TotalOpen      int          `json:"total_open"`
	RemainingCount int          `json:"remaining_count"`
}

// MemberChecker verifies member authorization.
type MemberChecker interface {
	Authorize(ctx context.Context, userID int64) (*household.Member, error)
}

// Clock provides time and location info.
type Clock interface {
	Now() time.Time
	Location() *time.Location
}

// Store defines persistence operations for chores.
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
}

// Service manages chore workflows.
type Service struct {
	store   Store
	members MemberChecker
	clock   Clock
}

// NewService constructs a chore Service.
func NewService(store Store, members MemberChecker, clock Clock) *Service {
	return &Service{
		store:   store,
		members: members,
		clock:   clock,
	}
}

// CreateDefinition registers a new chore definition.
func (s *Service) CreateDefinition(ctx context.Context, actorID int64, name, emoji, desc string, rule *recurrence.Rule) (*Definition, error) {
	if _, err := s.members.Authorize(ctx, actorID); err != nil {
		return nil, ErrNotMember
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, ErrBlankName
	}
	if emoji == "" {
		emoji = "🧹"
	}

	now := s.clock.Now().UTC()
	def := Definition{
		Name:        trimmed,
		Emoji:       emoji,
		Description: strings.TrimSpace(desc),
		CreatedBy:   actorID,
		CreatedAt:   now,
		UpdatedAt:   now,
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

// ListDefinitions returns all chore definitions.
func (s *Service) ListDefinitions(ctx context.Context) ([]Definition, error) {
	return s.store.ListDefinitions(ctx)
}

// ScheduleOccurrence schedules an occurrence for an active assignee.
func (s *Service) ScheduleOccurrence(ctx context.Context, actorID, defID, assigneeID int64, dueAt time.Time) (*Occurrence, error) {
	if _, err := s.members.Authorize(ctx, actorID); err != nil {
		return nil, ErrNotMember
	}
	if _, err := s.members.Authorize(ctx, assigneeID); err != nil {
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

	// First occurrence: scheduled_at == due_at
	nowUTC := now.UTC()
	occ := Occurrence{
		DefinitionID: def.ID,
		AssigneeID:   assigneeID,
		ScheduledAt:  dueAt.UTC(),
		DueAt:        dueAt.UTC(),
		Status:       "open",
		CreatedAt:    nowUTC,
		UpdatedAt:    nowUTC,
	}

	return s.store.CreateOccurrence(ctx, occ)
}

// Complete completes an occurrence. Only the assignee may complete it.
func (s *Service) Complete(ctx context.Context, actorID, occID int64, completedAt time.Time) (*Occurrence, *Occurrence, error) {
	if _, err := s.members.Authorize(ctx, actorID); err != nil {
		return nil, nil, ErrNotMember
	}

	occ, err := s.store.GetOccurrence(ctx, occID)
	if err != nil {
		return nil, nil, ErrOccurrenceNotFound
	}

	if occ.AssigneeID != actorID {
		return nil, nil, ErrNotAssignee
	}

	// Idempotent: if already completed, return existing
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

// Postpone postpones an occurrence. Only the assignee may postpone it.
func (s *Service) Postpone(ctx context.Context, actorID, occID int64, newDueAt time.Time) (*Occurrence, error) {
	if _, err := s.members.Authorize(ctx, actorID); err != nil {
		return nil, ErrNotMember
	}

	occ, err := s.store.GetOccurrence(ctx, occID)
	if err != nil {
		return nil, ErrOccurrenceNotFound
	}

	// Authorize before validating the requested time so non-owners never learn about time errors.
	if occ.AssigneeID != actorID {
		return nil, ErrNotAssignee
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

// ListMyOpen returns the requesting member's open chores capped at 20 items.
func (s *Service) ListMyOpen(ctx context.Context, memberID int64) (ListResult, error) {
	if _, err := s.members.Authorize(ctx, memberID); err != nil {
		return ListResult{}, ErrNotMember
	}
	return s.store.ListMyOpenOccurrences(ctx, memberID, 20)
}

// GetOccurrence retrieves an occurrence by ID.
func (s *Service) GetOccurrence(ctx context.Context, occID int64) (*Occurrence, error) {
	return s.store.GetOccurrence(ctx, occID)
}

// ListUpcoming returns open chores occurrences due in [from, to), ordered by due_at.
// It only reads; it never schedules, moves or resolves anything.
func (s *Service) ListUpcoming(ctx context.Context, from, to time.Time) ([]Occurrence, error) {
	return s.store.ListUpcomingOccurrences(ctx, from.UTC(), to.UTC())
}
