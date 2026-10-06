// Package sqlite manages SQLite database connections, configuration pragmas,
// and schema migrations for the home-ctl application.
package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver
)

// Open initializes and configures a SQLite database connection with WAL mode,
// busy timeouts, and foreign key enforcement.
func Open(dbPath string) (*sql.DB, error) {
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0750); err != nil {
			return nil, fmt.Errorf("failed to create database directory %s: %w", dir, err)
		}
	}

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database %s: %w", dbPath, err)
	}

	// SQLite operates most reliably with a single writer connection in Go
	db.SetMaxOpenConns(1)

	// Explicitly verify connection and foreign key pragma
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys;").Scan(&fk); err != nil || fk != 1 {
		// Enforce manually if not set via DSN
		if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
		}
	}

	return db, nil
}
