package sqlite

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// RunMigrations executes all embedded database migrations in version order.
func RunMigrations(db *sql.DB) error {
	sub, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("failed to load embedded migrations directory: %w", err)
	}
	return MigrateFromFS(db, sub)
}

// MigrateFromFS executes migrations located in the provided fs.FS.
func MigrateFromFS(db *sql.DB, fsys fs.FS) error {
	if err := ensureMigrationsTable(db); err != nil {
		return err
	}

	files, err := listMigrationFiles(fsys)
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := migrateFile(db, fsys, file); err != nil {
			return err
		}
	}

	return nil
}

// ensureMigrationsTable creates the schema_migrations table if it does not exist.
func ensureMigrationsTable(db *sql.DB) error {
	initSchema := `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    checksum TEXT NOT NULL,
    applied_at TEXT NOT NULL
) STRICT;
`
	if _, err := db.Exec(initSchema); err != nil {
		return fmt.Errorf("failed to initialize schema_migrations: %w", err)
	}
	return nil
}

// listMigrationFiles returns the .sql file names in fsys, sorted.
func listMigrationFiles(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to read migration files: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

// migrateFile applies one migration file unless it is already recorded.
func migrateFile(db *sql.DB, fsys fs.FS, file string) error {
	content, err := fs.ReadFile(fsys, file)
	if err != nil {
		return fmt.Errorf("failed to read migration file %s: %w", file, err)
	}

	ver, err := parseMigrationVersion(file)
	if err != nil {
		return err
	}

	checksum := computeChecksum(content)

	applied, err := checkAppliedChecksum(db, ver, file, checksum)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}

	return applyMigration(db, file, ver, content, checksum)
}

// checkAppliedChecksum reports whether ver is already recorded, verifying its checksum if so.
func checkAppliedChecksum(db *sql.DB, ver int, file, checksum string) (bool, error) {
	var storedChecksum string
	err := db.QueryRow("SELECT checksum FROM schema_migrations WHERE version = ?", ver).Scan(&storedChecksum)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check migration status for %s: %w", file, err)
	}
	if storedChecksum != checksum {
		return false, fmt.Errorf("migration %d (%s) modified after being applied (expected %s, got %s)",
			ver, file, storedChecksum, checksum)
	}
	return true, nil
}

// applyMigration runs one migration and records it in a single transaction.
func applyMigration(db *sql.DB, file string, ver int, content []byte, checksum string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction for %s: %w", file, err)
	}

	if _, err := tx.Exec(string(content)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to apply migration %s: %w", file, err)
	}

	appliedAt := time.Now().UTC().Format(time.RFC3339)
	insertMigration := `INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`
	if _, err := tx.Exec(insertMigration, ver, file, checksum, appliedAt); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to record applied migration %s: %w", file, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration transaction for %s: %w", file, err)
	}
	return nil
}

func parseMigrationVersion(fileName string) (int, error) {
	base := path.Base(fileName)
	parts := strings.SplitN(base, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid migration filename %q, expected XXXX_name.sql", fileName)
	}
	ver, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid migration version prefix in %q: %w", fileName, err)
	}
	return ver, nil
}

func computeChecksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
