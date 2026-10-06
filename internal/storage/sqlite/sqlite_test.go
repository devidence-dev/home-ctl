package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/px1/home-ctl/internal/storage/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tempFile := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(tempFile)
	if err != nil {
		t.Fatalf("failed to open sqlite database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestMigrations_RunTwiceAndRejectModified(t *testing.T) {
	db := newTestDB(t)

	// First run
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("first RunMigrations failed: %v", err)
	}

	// Second run (idempotent, no changes)
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("second RunMigrations failed: %v", err)
	}

	// Reject modified migration
	modifiedFS := fstest.MapFS{
		"0001_initial.sql": &fstest.MapFile{
			Data:    []byte("-- Modified migration content"),
			Mode:    0600,
			ModTime: time.Now(),
		},
	}
	err := sqlite.MigrateFromFS(db, modifiedFS)
	if err == nil {
		t.Fatalf("expected error when running modified migration, got nil")
	}
	if !strings.Contains(err.Error(), "modified after being applied") {
		t.Fatalf("expected 'modified after being applied' error, got %v", err)
	}
}

func TestMigrations_Constraints(t *testing.T) {
	db := newTestDB(t)
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// 1. Insert valid user
	_, err := db.Exec(`INSERT INTO users (id, name, username, locale, active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		1001, "Alice", "alice", "en", 1, now, now)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}

	// Test user active constraint (must be 0 or 1)
	_, err = db.Exec(`INSERT INTO users (id, name, username, locale, active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		1002, "Bob", "bob", "en", 5, now, now)
	if err == nil {
		t.Fatalf("expected CHECK constraint violation for invalid active value, got nil")
	}

	// 2. Shopping items: case-insensitive uniqueness
	_, err = db.Exec(`INSERT INTO shopping_items (name, emoji, created_by, created_at) VALUES (?, ?, ?, ?)`,
		"Eggs", "🥚", 1001, now)
	if err != nil {
		t.Fatalf("failed to insert shopping item: %v", err)
	}
	_, err = db.Exec(`INSERT INTO shopping_items (name, emoji, created_by, created_at) VALUES (?, ?, ?, ?)`,
		"eggs", "🥚", 1001, now)
	if err == nil {
		t.Fatalf("expected unique constraint violation for duplicate item name case-insensitively, got nil")
	}

	// 3. Shopping entries: invalid status
	_, err = db.Exec(`INSERT INTO shopping_entries (item_id, status, added_by, added_at) VALUES (?, ?, ?, ?)`,
		1, "invalid_status", 1001, now)
	if err == nil {
		t.Fatalf("expected CHECK constraint violation for invalid shopping status, got nil")
	}

	// Duplicate pending entry for same item rejected
	_, err = db.Exec(`INSERT INTO shopping_entries (item_id, status, added_by, added_at) VALUES (?, ?, ?, ?)`,
		1, "pending", 1001, now)
	if err != nil {
		t.Fatalf("failed to insert pending shopping entry: %v", err)
	}
	_, err = db.Exec(`INSERT INTO shopping_entries (item_id, status, added_by, added_at) VALUES (?, ?, ?, ?)`,
		1, "pending", 1001, now)
	if err == nil {
		t.Fatalf("expected unique index violation for duplicate pending shopping entry, got nil")
	}

	// 4. Chore definitions: blank name rejected
	_, err = db.Exec(`INSERT INTO chore_definitions (name, emoji, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"   ", "🧹", 1001, now, now)
	if err == nil {
		t.Fatalf("expected CHECK constraint failure for blank chore name, got nil")
	}

	// Insert valid chore definition
	res, err := db.Exec(`INSERT INTO chore_definitions (name, emoji, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"Sweep Floor", "🧹", 1001, now, now)
	if err != nil {
		t.Fatalf("failed to insert chore definition: %v", err)
	}
	choreDefID, _ := res.LastInsertId()

	// 5. Chore occurrences: invalid status
	_, err = db.Exec(`INSERT INTO chore_occurrences (definition_id, assignee_id, scheduled_at, due_at, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		choreDefID, 1001, now, now, "in_progress", now, now)
	if err == nil {
		t.Fatalf("expected CHECK constraint failure for invalid chore status, got nil")
	}

	// Chore occurrences: duplicate (definition_id, scheduled_at)
	_, err = db.Exec(`INSERT INTO chore_occurrences (definition_id, assignee_id, scheduled_at, due_at, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		choreDefID, 1001, now, now, "open", now, now)
	if err != nil {
		t.Fatalf("failed to insert chore occurrence: %v", err)
	}
	_, err = db.Exec(`INSERT INTO chore_occurrences (definition_id, assignee_id, scheduled_at, due_at, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		choreDefID, 1001, now, now, "open", now, now)
	if err == nil {
		t.Fatalf("expected duplicate (definition_id, scheduled_at) failure, got nil")
	}

	// 6. Foreign key enforcement: chore occurrence with invalid definition_id
	_, err = db.Exec(`INSERT INTO chore_occurrences (definition_id, assignee_id, scheduled_at, due_at, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		99999, 1001, "2026-10-10T10:00:00Z", "2026-10-10T10:00:00Z", "open", now, now)
	if err == nil {
		t.Fatalf("expected foreign key violation for nonexistent definition_id, got nil")
	}

	// 7. Processed updates: duplicate update_id
	_, err = db.Exec(`INSERT INTO processed_updates (update_id, processed_at) VALUES (?, ?)`, 500, now)
	if err != nil {
		t.Fatalf("failed to insert processed update: %v", err)
	}
	_, err = db.Exec(`INSERT INTO processed_updates (update_id, processed_at) VALUES (?, ?)`, 500, now)
	if err == nil {
		t.Fatalf("expected duplicate update_id failure, got nil")
	}

	// 8. Reminder definitions & occurrences constraints
	_, err = db.Exec(`INSERT INTO reminder_definitions (title, emoji, target_user_id, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"  ", "⏰", 1001, 1001, now, now)
	if err == nil {
		t.Fatalf("expected blank reminder title failure, got nil")
	}

	resRem, err := db.Exec(`INSERT INTO reminder_definitions (title, emoji, target_user_id, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"Drink Water", "💧", 1001, 1001, now, now)
	if err != nil {
		t.Fatalf("failed to insert reminder definition: %v", err)
	}
	remDefID, _ := resRem.LastInsertId()

	_, err = db.Exec(`INSERT INTO reminder_occurrences (definition_id, target_user_id, scheduled_at, due_at, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		remDefID, 1001, now, now, "open", now, now)
	if err != nil {
		t.Fatalf("failed to insert reminder occurrence: %v", err)
	}
	_, err = db.Exec(`INSERT INTO reminder_occurrences (definition_id, target_user_id, scheduled_at, due_at, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		remDefID, 1001, now, now, "open", now, now)
	if err == nil {
		t.Fatalf("expected duplicate (definition_id, scheduled_at) failure on reminder occurrence, got nil")
	}

	// 9. Postponements constraint
	_, err = db.Exec(`INSERT INTO postponements (entity_type, occurrence_id, user_id, old_due_at, new_due_at, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"invalid_type", 1, 1001, now, now, now)
	if err == nil {
		t.Fatalf("expected invalid entity_type failure on postponements, got nil")
	}

	// 10. Notification attempts outcome constraint
	_, err = db.Exec(`INSERT INTO notification_attempts (entity_type, occurrence_id, target_user_id, due_at, attempt_time, outcome, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"chore", 1, 1001, now, now, "invalid_outcome", now)
	if err == nil {
		t.Fatalf("expected invalid outcome failure on notification_attempts, got nil")
	}

	// 11. Flow state unique (user_id, chat_id, thread_id)
	_, err = db.Exec(`INSERT INTO flow_state (user_id, chat_id, thread_id, step, payload, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		1001, -100, 42, "step1", "{}", now, now, now)
	if err != nil {
		t.Fatalf("failed to insert flow_state: %v", err)
	}
	_, err = db.Exec(`INSERT INTO flow_state (user_id, chat_id, thread_id, step, payload, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		1001, -100, 42, "step2", "{}", now, now, now)
	if err == nil {
		t.Fatalf("expected duplicate flow_state key failure, got nil")
	}
}

func TestMigrations_ErrorCases(t *testing.T) {
	db := newTestDB(t)

	// Invalid filename format
	badNameFS := fstest.MapFS{
		"invalid_name.sql": &fstest.MapFile{
			Data: []byte("SELECT 1;"),
		},
	}
	if err := sqlite.MigrateFromFS(db, badNameFS); err == nil {
		t.Fatalf("expected error for invalid migration filename, got nil")
	}

	// Bad SQL syntax
	badSQLFS := fstest.MapFS{
		"0001_bad.sql": &fstest.MapFile{
			Data: []byte("THIS IS NOT SQL;"),
		},
	}
	if err := sqlite.MigrateFromFS(db, badSQLFS); err == nil {
		t.Fatalf("expected error for bad SQL syntax, got nil")
	}
}

func TestOpen_Memory(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping in-memory database: %v", err)
	}
}

func TestOpen_InvalidPath(t *testing.T) {
	_, err := sqlite.Open("/dev/null/impossible/dir/test.db")
	if err == nil {
		t.Fatalf("expected error when creating db in invalid path, got nil")
	}
}
