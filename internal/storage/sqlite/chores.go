package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/px1/home-ctl/internal/chores"
)

// ChoresStore manages SQLite persistence for chore definitions, occurrences,
// and postponements.
type ChoresStore struct {
	db *sql.DB
}

// NewChoresStore creates a new ChoresStore.
func NewChoresStore(db *sql.DB) *ChoresStore {
	return &ChoresStore{db: db}
}

// CreateDefinition inserts a new chore definition.
func (s *ChoresStore) CreateDefinition(ctx context.Context, def chores.Definition) (*chores.Definition, error) {
	nowStr := def.CreatedAt.UTC().Format(time.RFC3339)
	var originStr *string
	if def.SeriesOrigin != nil {
		str := def.SeriesOrigin.UTC().Format(time.RFC3339)
		originStr = &str
	}

	query := `
INSERT INTO chore_definitions (
	name, emoji, description, recurrence_interval, recurrence_unit,
	recurrence_anchor, series_origin, created_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	res, err := s.db.ExecContext(ctx, query,
		def.Name, def.Emoji, def.Description, def.RecurrenceInterval,
		def.RecurrenceUnit, def.RecurrenceAnchor, originStr,
		def.CreatedBy, nowStr, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert chore definition: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	def.ID = id
	return &def, nil
}

// GetDefinition retrieves a chore definition by ID.
func (s *ChoresStore) GetDefinition(ctx context.Context, id int64) (*chores.Definition, error) {
	query := `
SELECT id, name, emoji, COALESCE(description, ''), recurrence_interval,
       recurrence_unit, recurrence_anchor, series_origin, created_by,
       created_at, updated_at
FROM chore_definitions WHERE id = ?`

	row := s.db.QueryRowContext(ctx, query, id)
	return scanChoreDefinition(row)
}

// ListDefinitions returns all chore definitions sorted by name.
func (s *ChoresStore) ListDefinitions(ctx context.Context) ([]chores.Definition, error) {
	query := `
SELECT id, name, emoji, COALESCE(description, ''), recurrence_interval,
       recurrence_unit, recurrence_anchor, series_origin, created_by,
       created_at, updated_at
FROM chore_definitions ORDER BY name ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list chore definitions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var defs []chores.Definition
	for rows.Next() {
		def, err := scanChoreDefinition(rows)
		if err != nil {
			return nil, err
		}
		defs = append(defs, *def)
	}
	return defs, rows.Err()
}

// CreateOccurrence inserts a new chore occurrence.
func (s *ChoresStore) CreateOccurrence(ctx context.Context, occ chores.Occurrence) (*chores.Occurrence, error) {
	schedStr := occ.ScheduledAt.UTC().Format(time.RFC3339)
	dueStr := occ.DueAt.UTC().Format(time.RFC3339)
	createdStr := occ.CreatedAt.UTC().Format(time.RFC3339)
	updatedStr := occ.UpdatedAt.UTC().Format(time.RFC3339)

	query := `
INSERT INTO chore_occurrences (
	definition_id, assignee_id, scheduled_at, due_at, status, created_at, updated_at
) VALUES (?, ?, ?, ?, 'open', ?, ?)`

	res, err := s.db.ExecContext(ctx, query,
		occ.DefinitionID, occ.AssigneeID, schedStr, dueStr, createdStr, updatedStr,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert chore occurrence: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetOccurrence(ctx, id)
}

// GetOccurrence retrieves a chore occurrence and its associated definition.
func (s *ChoresStore) GetOccurrence(ctx context.Context, id int64) (*chores.Occurrence, error) {
	query := `
SELECT o.id, o.definition_id, d.name, d.emoji, COALESCE(d.description, ''),
       d.recurrence_interval, d.recurrence_unit, d.recurrence_anchor, d.series_origin,
       d.created_by, d.created_at, d.updated_at,
       o.assignee_id, o.scheduled_at, o.due_at, o.status,
       o.completed_by, o.completed_at, o.created_at, o.updated_at
FROM chore_occurrences o
JOIN chore_definitions d ON o.definition_id = d.id
WHERE o.id = ?`

	row := s.db.QueryRowContext(ctx, query, id)
	return scanChoreOccurrence(row)
}

// CompleteOccurrence completes an occurrence and conditionally creates a successor.
func (s *ChoresStore) CompleteOccurrence(ctx context.Context, occID, actorID int64, completedAt time.Time, nextSlot *time.Time) (*chores.Occurrence, *chores.Occurrence, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	compStr := completedAt.UTC().Format(time.RFC3339)
	nowStr := time.Now().UTC().Format(time.RFC3339)

	updateQuery := `
UPDATE chore_occurrences
SET status = 'completed', completed_by = ?, completed_at = ?, updated_at = ?
WHERE id = ? AND status = 'open'`

	res, err := tx.ExecContext(ctx, updateQuery, actorID, compStr, nowStr, occID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to complete occurrence: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return nil, nil, err
	}

	if rows == 0 {
		_ = tx.Rollback()
		// Already completed: return current occurrence idempotently
		curr, err := s.GetOccurrence(ctx, occID)
		if err != nil {
			return nil, nil, chores.ErrOccurrenceNotFound
		}
		return curr, nil, nil
	}

	completedOcc, err := s.getOccurrenceTx(ctx, tx, occID)
	if err != nil {
		return nil, nil, err
	}

	var successor *chores.Occurrence
	if nextSlot != nil {
		nextSlotStr := nextSlot.UTC().Format(time.RFC3339)
		insertSuccessor := `
INSERT OR IGNORE INTO chore_occurrences (
	definition_id, assignee_id, scheduled_at, due_at, status, created_at, updated_at
) VALUES (?, ?, ?, ?, 'open', ?, ?)`

		if _, err := tx.ExecContext(ctx, insertSuccessor,
			completedOcc.DefinitionID, completedOcc.AssigneeID, nextSlotStr, nextSlotStr, nowStr, nowStr,
		); err != nil {
			return nil, nil, fmt.Errorf("failed to insert successor occurrence: %w", err)
		}

		// Query the successor occurrence
		querySucc := `
SELECT o.id, o.definition_id, d.name, d.emoji, COALESCE(d.description, ''),
       d.recurrence_interval, d.recurrence_unit, d.recurrence_anchor, d.series_origin,
       d.created_by, d.created_at, d.updated_at,
       o.assignee_id, o.scheduled_at, o.due_at, o.status,
       o.completed_by, o.completed_at, o.created_at, o.updated_at
FROM chore_occurrences o
JOIN chore_definitions d ON o.definition_id = d.id
WHERE o.definition_id = ? AND o.scheduled_at = ?`

		row := tx.QueryRowContext(ctx, querySucc, completedOcc.DefinitionID, nextSlotStr)
		succ, err := scanChoreOccurrence(row)
		if err == nil {
			successor = succ
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("failed to commit completion transaction: %w", err)
	}

	return completedOcc, successor, nil
}

// PostponeOccurrence updates an occurrence's due_at and writes a history record.
func (s *ChoresStore) PostponeOccurrence(ctx context.Context, occID, actorID int64, newDueAt, now time.Time) (*chores.Occurrence, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var oldDueStr, status string
	err = tx.QueryRowContext(ctx, `SELECT due_at, status FROM chore_occurrences WHERE id = ?`, occID).Scan(&oldDueStr, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, chores.ErrOccurrenceNotFound
		}
		return nil, err
	}
	if status != "open" {
		return nil, chores.ErrOccurrenceResolved
	}

	newDueStr := newDueAt.UTC().Format(time.RFC3339)
	nowStr := now.UTC().Format(time.RFC3339)
	if newDueStr == oldDueStr {
		// Repeating the same postponement is a no-op: no new history row, no notification reset.
		_ = tx.Rollback()
		return s.GetOccurrence(ctx, occID)
	}

	// Conditional update: a concurrent completion that wins the race leaves zero rows to change.
	updateQuery := `UPDATE chore_occurrences SET due_at = ?, updated_at = ? WHERE id = ? AND status = 'open'`
	res, err := tx.ExecContext(ctx, updateQuery, newDueStr, nowStr, occID)
	if err != nil {
		return nil, fmt.Errorf("failed to update occurrence due_at: %w", err)
	}
	if changed, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if changed == 0 {
		return nil, chores.ErrOccurrenceResolved
	}

	insertPostpone := `
INSERT INTO postponements (
	entity_type, occurrence_id, user_id, old_due_at, new_due_at, created_at
) VALUES ('chore', ?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, insertPostpone, occID, actorID, oldDueStr, newDueStr, nowStr); err != nil {
		return nil, fmt.Errorf("failed to record postponement: %w", err)
	}

	// Reset claimed notification attempts for this occurrence
	if _, err := tx.ExecContext(ctx, `DELETE FROM notification_attempts WHERE entity_type = 'chore' AND occurrence_id = ? AND outcome = 'claimed'`, occID); err != nil {
		return nil, fmt.Errorf("failed to reset notification attempts: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit postponement: %w", err)
	}

	return s.GetOccurrence(ctx, occID)
}

// ListMyOpenOccurrences retrieves up to limit open occurrences for a member alongside total count.
func (s *ChoresStore) ListMyOpenOccurrences(ctx context.Context, memberID int64, limit int) (chores.ListResult, error) {
	var total int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM chore_occurrences
WHERE assignee_id = ? AND status = 'open'`, memberID).Scan(&total)
	if err != nil {
		return chores.ListResult{}, fmt.Errorf("failed to count open chores: %w", err)
	}

	query := `
SELECT o.id, o.definition_id, d.name, d.emoji, COALESCE(d.description, ''),
       d.recurrence_interval, d.recurrence_unit, d.recurrence_anchor, d.series_origin,
       d.created_by, d.created_at, d.updated_at,
       o.assignee_id, o.scheduled_at, o.due_at, o.status,
       o.completed_by, o.completed_at, o.created_at, o.updated_at
FROM chore_occurrences o
JOIN chore_definitions d ON o.definition_id = d.id
WHERE o.assignee_id = ? AND o.status = 'open'
ORDER BY o.due_at ASC
LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, memberID, limit)
	if err != nil {
		return chores.ListResult{}, fmt.Errorf("failed to list open chores: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var occs []chores.Occurrence
	for rows.Next() {
		occ, err := scanChoreOccurrence(rows)
		if err != nil {
			return chores.ListResult{}, err
		}
		occs = append(occs, *occ)
	}
	if err := rows.Err(); err != nil {
		return chores.ListResult{}, err
	}

	remaining := 0
	if total > len(occs) {
		remaining = total - len(occs)
	}

	return chores.ListResult{
		Occurrences:    occs,
		TotalOpen:      total,
		RemainingCount: remaining,
	}, nil
}

const choreOpenSelect = `
SELECT o.id, o.definition_id, d.name, d.emoji, COALESCE(d.description, ''),
       d.recurrence_interval, d.recurrence_unit, d.recurrence_anchor, d.series_origin,
       d.created_by, d.created_at, d.updated_at,
       o.assignee_id, o.scheduled_at, o.due_at, o.status,
       o.completed_by, o.completed_at, o.created_at, o.updated_at
FROM chore_occurrences o
JOIN chore_definitions d ON o.definition_id = d.id
WHERE o.status = 'open'`

const (
	choreDueQuery      = choreOpenSelect + ` AND o.due_at <= ? ORDER BY o.due_at ASC`
	choreUpcomingQuery = choreOpenSelect + ` AND o.due_at >= ? AND o.due_at < ? ORDER BY o.due_at ASC`
)

// ListDueOccurrences returns open chore occurrences due at or before the given timestamp.
func (s *ChoresStore) ListDueOccurrences(ctx context.Context, dueBefore time.Time) ([]chores.Occurrence, error) {
	return s.listOpenOccurrences(ctx, choreDueQuery, dueBefore.UTC().Format(time.RFC3339))
}

// ListUpcomingOccurrences returns open chore occurrences with from <= due_at < to, ordered by due_at.
// It is a deterministic read-only query, safe for any future planning consumer.
func (s *ChoresStore) ListUpcomingOccurrences(ctx context.Context, from, to time.Time) ([]chores.Occurrence, error) {
	return s.listOpenOccurrences(ctx, choreUpcomingQuery,
		from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
}

func (s *ChoresStore) listOpenOccurrences(ctx context.Context, query string, args ...any) ([]chores.Occurrence, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list chore occurrences: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var occs []chores.Occurrence
	for rows.Next() {
		occ, err := scanChoreOccurrence(rows)
		if err != nil {
			return nil, err
		}
		occs = append(occs, *occ)
	}
	return occs, rows.Err()
}

func (s *ChoresStore) getOccurrenceTx(ctx context.Context, tx *sql.Tx, id int64) (*chores.Occurrence, error) {
	query := `
SELECT o.id, o.definition_id, d.name, d.emoji, COALESCE(d.description, ''),
       d.recurrence_interval, d.recurrence_unit, d.recurrence_anchor, d.series_origin,
       d.created_by, d.created_at, d.updated_at,
       o.assignee_id, o.scheduled_at, o.due_at, o.status,
       o.completed_by, o.completed_at, o.created_at, o.updated_at
FROM chore_occurrences o
JOIN chore_definitions d ON o.definition_id = d.id
WHERE o.id = ?`

	row := tx.QueryRowContext(ctx, query, id)
	return scanChoreOccurrence(row)
}

func scanChoreDefinition(s rowScanner) (*chores.Definition, error) {
	var d chores.Definition
	var interval sql.NullInt64
	var unit, anchor, originStr sql.NullString
	var createdStr, updatedStr string

	if err := s.Scan(
		&d.ID, &d.Name, &d.Emoji, &d.Description,
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

func scanChoreOccurrence(s rowScanner) (*chores.Occurrence, error) {
	var o chores.Occurrence
	var d chores.Definition
	var interval sql.NullInt64
	var unit, anchor, originStr sql.NullString
	var dCreatedStr, dUpdatedStr string

	var schedStr, dueStr, oCreatedStr, oUpdatedStr string
	var compBy sql.NullInt64
	var compAtStr sql.NullString

	if err := s.Scan(
		&o.ID, &o.DefinitionID, &d.Name, &d.Emoji, &d.Description,
		&interval, &unit, &anchor, &originStr,
		&d.CreatedBy, &dCreatedStr, &dUpdatedStr,
		&o.AssigneeID, &schedStr, &dueStr, &o.Status,
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
