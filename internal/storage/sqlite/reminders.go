package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/px1/home-ctl/internal/reminders"
)

// RemindersStore provides SQLite persistence for reminder definitions, occurrences,
// postponements, and notification intervals.
type RemindersStore struct {
	db *sql.DB
}

// NewRemindersStore creates a new RemindersStore.
func NewRemindersStore(db *sql.DB) *RemindersStore {
	return &RemindersStore{db: db}
}

// CreateDefinition inserts a new reminder definition.
func (s *RemindersStore) CreateDefinition(ctx context.Context, def reminders.Definition) (*reminders.Definition, error) {
	nowStr := def.CreatedAt.UTC().Format(time.RFC3339)
	var originStr *string
	if def.SeriesOrigin != nil {
		str := def.SeriesOrigin.UTC().Format(time.RFC3339)
		originStr = &str
	}

	query := `
INSERT INTO reminder_definitions (
	title, emoji, description, target_user_id, recurrence_interval,
	recurrence_unit, recurrence_anchor, series_origin, created_by,
	created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	res, err := s.db.ExecContext(ctx, query,
		def.Title, def.Emoji, def.Description, def.TargetUserID,
		def.RecurrenceInterval, def.RecurrenceUnit, def.RecurrenceAnchor,
		originStr, def.CreatedBy, nowStr, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert reminder definition: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	def.ID = id
	return &def, nil
}

// GetDefinition retrieves a reminder definition by ID.
func (s *RemindersStore) GetDefinition(ctx context.Context, id int64) (*reminders.Definition, error) {
	query := `
SELECT id, title, emoji, COALESCE(description, ''), target_user_id,
       recurrence_interval, recurrence_unit, recurrence_anchor,
       series_origin, created_by, created_at, updated_at
FROM reminder_definitions WHERE id = ?`

	row := s.db.QueryRowContext(ctx, query, id)
	return scanReminderDefinition(row)
}

// ListDefinitions lists all reminder definitions.
func (s *RemindersStore) ListDefinitions(ctx context.Context) ([]reminders.Definition, error) {
	query := `
SELECT id, title, emoji, COALESCE(description, ''), target_user_id,
       recurrence_interval, recurrence_unit, recurrence_anchor,
       series_origin, created_by, created_at, updated_at
FROM reminder_definitions ORDER BY title ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list reminder definitions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var defs []reminders.Definition
	for rows.Next() {
		def, err := scanReminderDefinition(rows)
		if err != nil {
			return nil, err
		}
		defs = append(defs, *def)
	}
	return defs, rows.Err()
}

// CreateOccurrence inserts a new reminder occurrence.
func (s *RemindersStore) CreateOccurrence(ctx context.Context, occ reminders.Occurrence) (*reminders.Occurrence, error) {
	schedStr := occ.ScheduledAt.UTC().Format(time.RFC3339)
	dueStr := occ.DueAt.UTC().Format(time.RFC3339)
	createdStr := occ.CreatedAt.UTC().Format(time.RFC3339)
	updatedStr := occ.UpdatedAt.UTC().Format(time.RFC3339)

	query := `
INSERT INTO reminder_occurrences (
	definition_id, target_user_id, scheduled_at, due_at, status, created_at, updated_at
) VALUES (?, ?, ?, ?, 'open', ?, ?)`

	res, err := s.db.ExecContext(ctx, query,
		occ.DefinitionID, occ.TargetUserID, schedStr, dueStr, createdStr, updatedStr,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert reminder occurrence: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetOccurrence(ctx, id)
}

// GetOccurrence retrieves a reminder occurrence and its definition.
func (s *RemindersStore) GetOccurrence(ctx context.Context, id int64) (*reminders.Occurrence, error) {
	query := `
SELECT o.id, o.definition_id, d.title, d.emoji, COALESCE(d.description, ''),
       d.target_user_id, d.recurrence_interval, d.recurrence_unit,
       d.recurrence_anchor, d.series_origin, d.created_by,
       d.created_at, d.updated_at,
       o.target_user_id, o.scheduled_at, o.due_at, o.status,
       o.completed_by, o.completed_at, o.created_at, o.updated_at
FROM reminder_occurrences o
JOIN reminder_definitions d ON o.definition_id = d.id
WHERE o.id = ?`

	row := s.db.QueryRowContext(ctx, query, id)
	return scanReminderOccurrence(row)
}

// CompleteOccurrence completes an occurrence and creates a successor if recurrent.
func (s *RemindersStore) CompleteOccurrence(ctx context.Context, occID, actorID int64, completedAt time.Time, nextSlot *time.Time) (*reminders.Occurrence, *reminders.Occurrence, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	compStr := completedAt.UTC().Format(time.RFC3339)
	nowStr := time.Now().UTC().Format(time.RFC3339)

	updateQuery := `
UPDATE reminder_occurrences
SET status = 'completed', completed_by = ?, completed_at = ?, updated_at = ?
WHERE id = ? AND status = 'open'`

	res, err := tx.ExecContext(ctx, updateQuery, actorID, compStr, nowStr, occID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to complete reminder: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return nil, nil, err
	}

	if rows == 0 {
		_ = tx.Rollback()
		curr, err := s.GetOccurrence(ctx, occID)
		if err != nil {
			return nil, nil, reminders.ErrOccurrenceNotFound
		}
		return curr, nil, nil
	}

	completedOcc, err := s.getOccurrenceTx(ctx, tx, occID)
	if err != nil {
		return nil, nil, err
	}

	var successor *reminders.Occurrence
	if nextSlot != nil {
		nextSlotStr := nextSlot.UTC().Format(time.RFC3339)
		insertSuccessor := `
INSERT OR IGNORE INTO reminder_occurrences (
	definition_id, target_user_id, scheduled_at, due_at, status, created_at, updated_at
) VALUES (?, ?, ?, ?, 'open', ?, ?)`

		if _, err := tx.ExecContext(ctx, insertSuccessor,
			completedOcc.DefinitionID, completedOcc.TargetUserID, nextSlotStr, nextSlotStr, nowStr, nowStr,
		); err != nil {
			return nil, nil, fmt.Errorf("failed to insert successor reminder: %w", err)
		}

		querySucc := `
SELECT o.id, o.definition_id, d.title, d.emoji, COALESCE(d.description, ''),
       d.target_user_id, d.recurrence_interval, d.recurrence_unit,
       d.recurrence_anchor, d.series_origin, d.created_by,
       d.created_at, d.updated_at,
       o.target_user_id, o.scheduled_at, o.due_at, o.status,
       o.completed_by, o.completed_at, o.created_at, o.updated_at
FROM reminder_occurrences o
JOIN reminder_definitions d ON o.definition_id = d.id
WHERE o.definition_id = ? AND o.scheduled_at = ?`

		row := tx.QueryRowContext(ctx, querySucc, completedOcc.DefinitionID, nextSlotStr)
		succ, err := scanReminderOccurrence(row)
		if err == nil {
			successor = succ
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("failed to commit reminder completion: %w", err)
	}

	return completedOcc, successor, nil
}

// PostponeOccurrence updates due_at and writes a postponement record.
func (s *RemindersStore) PostponeOccurrence(ctx context.Context, occID, actorID int64, newDueAt, now time.Time) (*reminders.Occurrence, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var oldDueStr, status string
	err = tx.QueryRowContext(ctx, `SELECT due_at, status FROM reminder_occurrences WHERE id = ?`, occID).Scan(&oldDueStr, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, reminders.ErrOccurrenceNotFound
		}
		return nil, err
	}
	if status != "open" {
		return nil, reminders.ErrOccurrenceResolved
	}

	newDueStr := newDueAt.UTC().Format(time.RFC3339)
	nowStr := now.UTC().Format(time.RFC3339)
	if newDueStr == oldDueStr {
		// Repeating the same postponement is a no-op: no new history row, no notification reset.
		_ = tx.Rollback()
		return s.GetOccurrence(ctx, occID)
	}

	// Conditional update: a concurrent completion that wins the race leaves zero rows to change.
	updateQuery := `UPDATE reminder_occurrences SET due_at = ?, updated_at = ? WHERE id = ? AND status = 'open'`
	res, err := tx.ExecContext(ctx, updateQuery, newDueStr, nowStr, occID)
	if err != nil {
		return nil, fmt.Errorf("failed to update reminder due_at: %w", err)
	}
	if changed, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if changed == 0 {
		return nil, reminders.ErrOccurrenceResolved
	}

	insertPostpone := `
INSERT INTO postponements (
	entity_type, occurrence_id, user_id, old_due_at, new_due_at, created_at
) VALUES ('reminder', ?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, insertPostpone, occID, actorID, oldDueStr, newDueStr, nowStr); err != nil {
		return nil, fmt.Errorf("failed to record reminder postponement: %w", err)
	}

	// Reset claimed notification attempts for this reminder
	if _, err := tx.ExecContext(ctx, `DELETE FROM notification_attempts WHERE entity_type = 'reminder' AND occurrence_id = ? AND outcome = 'claimed'`, occID); err != nil {
		return nil, fmt.Errorf("failed to reset reminder notification attempts: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit postponement: %w", err)
	}

	return s.GetOccurrence(ctx, occID)
}

// ListMyOpenOccurrences lists up to limit open reminders for a target member.
func (s *RemindersStore) ListMyOpenOccurrences(ctx context.Context, memberID int64, limit int) (reminders.ListResult, error) {
	var total int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM reminder_occurrences
WHERE target_user_id = ? AND status = 'open'`, memberID).Scan(&total)
	if err != nil {
		return reminders.ListResult{}, fmt.Errorf("failed to count open reminders: %w", err)
	}

	query := `
SELECT o.id, o.definition_id, d.title, d.emoji, COALESCE(d.description, ''),
       d.target_user_id, d.recurrence_interval, d.recurrence_unit,
       d.recurrence_anchor, d.series_origin, d.created_by,
       d.created_at, d.updated_at,
       o.target_user_id, o.scheduled_at, o.due_at, o.status,
       o.completed_by, o.completed_at, o.created_at, o.updated_at
FROM reminder_occurrences o
JOIN reminder_definitions d ON o.definition_id = d.id
WHERE o.target_user_id = ? AND o.status = 'open'
ORDER BY o.due_at ASC
LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, memberID, limit)
	if err != nil {
		return reminders.ListResult{}, fmt.Errorf("failed to list open reminders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var occs []reminders.Occurrence
	for rows.Next() {
		occ, err := scanReminderOccurrence(rows)
		if err != nil {
			return reminders.ListResult{}, err
		}
		occs = append(occs, *occ)
	}
	if err := rows.Err(); err != nil {
		return reminders.ListResult{}, err
	}

	remaining := 0
	if total > len(occs) {
		remaining = total - len(occs)
	}

	return reminders.ListResult{
		Occurrences:    occs,
		TotalOpen:      total,
		RemainingCount: remaining,
	}, nil
}

const reminderOpenSelect = `
SELECT o.id, o.definition_id, d.title, d.emoji, COALESCE(d.description, ''),
       d.target_user_id, d.recurrence_interval, d.recurrence_unit,
       d.recurrence_anchor, d.series_origin, d.created_by,
       d.created_at, d.updated_at,
       o.target_user_id, o.scheduled_at, o.due_at, o.status,
       o.completed_by, o.completed_at, o.created_at, o.updated_at
FROM reminder_occurrences o
JOIN reminder_definitions d ON o.definition_id = d.id
WHERE o.status = 'open'`

const (
	reminderDueQuery      = reminderOpenSelect + ` AND o.due_at <= ? ORDER BY o.due_at ASC`
	reminderUpcomingQuery = reminderOpenSelect + ` AND o.due_at >= ? AND o.due_at < ? ORDER BY o.due_at ASC`
)

// ListDueOccurrences returns open reminder occurrences due at or before the given timestamp.
func (s *RemindersStore) ListDueOccurrences(ctx context.Context, dueBefore time.Time) ([]reminders.Occurrence, error) {
	return s.listOpenOccurrences(ctx, reminderDueQuery, dueBefore.UTC().Format(time.RFC3339))
}

// ListUpcomingOccurrences returns open reminder occurrences with from <= due_at < to, ordered by due_at.
// It is a deterministic read-only query, safe for any future planning consumer.
func (s *RemindersStore) ListUpcomingOccurrences(ctx context.Context, from, to time.Time) ([]reminders.Occurrence, error) {
	return s.listOpenOccurrences(ctx, reminderUpcomingQuery,
		from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
}

func (s *RemindersStore) listOpenOccurrences(ctx context.Context, query string, args ...any) ([]reminders.Occurrence, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list reminder occurrences: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var occs []reminders.Occurrence
	for rows.Next() {
		occ, err := scanReminderOccurrence(rows)
		if err != nil {
			return nil, err
		}
		occs = append(occs, *occ)
	}
	return occs, rows.Err()
}

// GetLastNotificationTime returns the latest notification attempt timestamp for the occurrence's
// current due_at, so a postponement starts a fresh notification cycle at the new due time.
func (s *RemindersStore) GetLastNotificationTime(ctx context.Context, occID int64) (*time.Time, error) {
	var maxTimeStr sql.NullString
	query := `
SELECT MAX(attempt_time) FROM notification_attempts
WHERE entity_type = 'reminder' AND occurrence_id = ? AND outcome IN ('sent', 'claimed')
  AND due_at = (SELECT due_at FROM reminder_occurrences WHERE id = ?)`

	err := s.db.QueryRowContext(ctx, query, occID, occID).Scan(&maxTimeStr)
	if err != nil || !maxTimeStr.Valid {
		return nil, err
	}

	t, err := time.Parse(time.RFC3339, maxTimeStr.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *RemindersStore) getOccurrenceTx(ctx context.Context, tx *sql.Tx, id int64) (*reminders.Occurrence, error) {
	query := `
SELECT o.id, o.definition_id, d.title, d.emoji, COALESCE(d.description, ''),
       d.target_user_id, d.recurrence_interval, d.recurrence_unit,
       d.recurrence_anchor, d.series_origin, d.created_by,
       d.created_at, d.updated_at,
       o.target_user_id, o.scheduled_at, o.due_at, o.status,
       o.completed_by, o.completed_at, o.created_at, o.updated_at
FROM reminder_occurrences o
JOIN reminder_definitions d ON o.definition_id = d.id
WHERE o.id = ?`

	row := tx.QueryRowContext(ctx, query, id)
	return scanReminderOccurrence(row)
}

func scanReminderDefinition(s rowScanner) (*reminders.Definition, error) {
	var d reminders.Definition
	var interval sql.NullInt64
	var unit, anchor, originStr sql.NullString
	var createdStr, updatedStr string

	if err := s.Scan(
		&d.ID, &d.Title, &d.Emoji, &d.Description, &d.TargetUserID,
		&interval, &unit, &anchor, &originStr,
		&d.CreatedBy, &createdStr, &updatedStr,
	); err != nil {
		return nil, err
	}

	if interval.Valid {
		v := int(interval.Int64)
		d.RecurrenceInterval = &v
	}
	if unit.Valid {
		d.RecurrenceUnit = &unit.String
	}
	if anchor.Valid {
		d.RecurrenceAnchor = &anchor.String
	}
	if originStr.Valid {
		t, _ := time.Parse(time.RFC3339, originStr.String)
		d.SeriesOrigin = &t
	}
	d.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	d.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	return &d, nil
}

func scanReminderOccurrence(s rowScanner) (*reminders.Occurrence, error) {
	var o reminders.Occurrence
	var d reminders.Definition
	var interval sql.NullInt64
	var unit, anchor, originStr sql.NullString
	var dCreatedStr, dUpdatedStr string

	var schedStr, dueStr, oCreatedStr, oUpdatedStr string
	var compBy sql.NullInt64
	var compAtStr sql.NullString

	if err := s.Scan(
		&o.ID, &o.DefinitionID, &d.Title, &d.Emoji, &d.Description,
		&d.TargetUserID, &interval, &unit, &anchor, &originStr,
		&d.CreatedBy, &dCreatedStr, &dUpdatedStr,
		&o.TargetUserID, &schedStr, &dueStr, &o.Status,
		&compBy, &compAtStr, &oCreatedStr, &oUpdatedStr,
	); err != nil {
		return nil, err
	}

	d.ID = o.DefinitionID
	if interval.Valid {
		v := int(interval.Int64)
		d.RecurrenceInterval = &v
	}
	if unit.Valid {
		d.RecurrenceUnit = &unit.String
	}
	if anchor.Valid {
		d.RecurrenceAnchor = &anchor.String
	}
	if originStr.Valid {
		t, _ := time.Parse(time.RFC3339, originStr.String)
		d.SeriesOrigin = &t
	}
	d.CreatedAt, _ = time.Parse(time.RFC3339, dCreatedStr)
	d.UpdatedAt, _ = time.Parse(time.RFC3339, dUpdatedStr)
	o.Definition = &d

	o.ScheduledAt, _ = time.Parse(time.RFC3339, schedStr)
	o.DueAt, _ = time.Parse(time.RFC3339, dueStr)
	o.CreatedAt, _ = time.Parse(time.RFC3339, oCreatedStr)
	o.UpdatedAt, _ = time.Parse(time.RFC3339, oUpdatedStr)

	if compBy.Valid {
		val := compBy.Int64
		o.CompletedBy = &val
	}
	if compAtStr.Valid {
		t, _ := time.Parse(time.RFC3339, compAtStr.String)
		o.CompletedAt = &t
	}

	return &o, nil
}
