package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/px1/home-ctl/internal/config"
	"github.com/px1/home-ctl/internal/household"
)

// HouseholdStore provides SQLite persistence for household members and onboarding.
type HouseholdStore struct {
	db *sql.DB
}

// NewHouseholdStore creates a new HouseholdStore.
func NewHouseholdStore(db *sql.DB) *HouseholdStore {
	return &HouseholdStore{db: db}
}

// SyncMembers syncs the config members with SQLite.
func (s *HouseholdStore) SyncMembers(ctx context.Context, users []config.UserConfig, defaultLocale string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	configIDs := make(map[int64]bool)

	for _, u := range users {
		configIDs[u.ID] = true
		locale := u.Locale
		if locale == "" {
			locale = defaultLocale
		}
		if locale == "" {
			locale = "en"
		}

		upsertQuery := `
INSERT INTO users (id, name, username, locale, active, created_at, updated_at)
VALUES (?, ?, ?, ?, 1, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	username = excluded.username,
	locale = excluded.locale,
	active = 1,
	updated_at = excluded.updated_at;
`
		if _, err := tx.ExecContext(ctx, upsertQuery, u.ID, u.Name, u.Username, locale, now, now); err != nil {
			return fmt.Errorf("failed to upsert user %d: %w", u.ID, err)
		}

		onboardingInit := `
INSERT OR IGNORE INTO user_onboarding (user_id, connected, updated_at)
VALUES (?, 0, ?);
`
		if _, err := tx.ExecContext(ctx, onboardingInit, u.ID, now); err != nil {
			return fmt.Errorf("failed to initialize onboarding for user %d: %w", u.ID, err)
		}
	}

	// Deactivate any existing users in DB that are not in config
	rows, err := tx.QueryContext(ctx, "SELECT id FROM users")
	if err != nil {
		return fmt.Errorf("failed to query existing users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var toDeactivate []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("failed to scan user id: %w", err)
		}
		if !configIDs[id] {
			toDeactivate = append(toDeactivate, id)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error reading users rows: %w", err)
	}

	for _, id := range toDeactivate {
		if _, err := tx.ExecContext(ctx, "UPDATE users SET active = 0, updated_at = ? WHERE id = ?", now, id); err != nil {
			return fmt.Errorf("failed to deactivate user %d: %w", id, err)
		}
	}

	return tx.Commit()
}

// GetMember retrieves a single member by user ID.
func (s *HouseholdStore) GetMember(ctx context.Context, id int64) (*household.Member, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, COALESCE(username, ''), locale, active FROM users WHERE id = ?`, id)
	var m household.Member
	var activeInt int
	if err := row.Scan(&m.ID, &m.Name, &m.Username, &m.Locale, &activeInt); err != nil {
		return nil, err
	}
	m.Active = activeInt == 1
	return &m, nil
}

// ListMembers returns all users, optionally filtered by active status.
func (s *HouseholdStore) ListMembers(ctx context.Context, activeOnly bool) ([]household.Member, error) {
	query := `SELECT id, name, COALESCE(username, ''), locale, active FROM users`
	if activeOnly {
		query += ` WHERE active = 1 ORDER BY name ASC`
	} else {
		query += ` ORDER BY name ASC`
	}

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query members: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var members []household.Member
	for rows.Next() {
		var m household.Member
		var activeInt int
		if err := rows.Scan(&m.ID, &m.Name, &m.Username, &m.Locale, &activeInt); err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		m.Active = activeInt == 1
		members = append(members, m)
	}
	return members, rows.Err()
}

// RecordOnboarding marks a user as connected with the given timestamp.
func (s *HouseholdStore) RecordOnboarding(ctx context.Context, userID int64, connectedAt time.Time) error {
	now := connectedAt.UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
UPDATE user_onboarding
SET connected = 1, connected_at = ?, updated_at = ?
WHERE user_id = ?`, now, now, userID)
	if err != nil {
		return fmt.Errorf("failed to update onboarding: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		_, err = s.db.ExecContext(ctx, `
INSERT INTO user_onboarding (user_id, connected, connected_at, updated_at)
VALUES (?, 1, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET connected = 1, connected_at = excluded.connected_at, updated_at = excluded.updated_at`,
			userID, now, now)
		return err
	}
	return nil
}

// RecordDisconnect marks a user as disconnected.
func (s *HouseholdStore) RecordDisconnect(ctx context.Context, userID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
UPDATE user_onboarding
SET connected = 0, updated_at = ?
WHERE user_id = ?`, now, userID)
	return err
}

// GetOnboardingState retrieves the onboarding state for a user.
func (s *HouseholdStore) GetOnboardingState(ctx context.Context, userID int64) (*household.OnboardingState, error) {
	row := s.db.QueryRowContext(ctx, `SELECT connected, connected_at FROM user_onboarding WHERE user_id = ?`, userID)
	var connectedInt int
	var connAtStr sql.NullString
	if err := row.Scan(&connectedInt, &connAtStr); err != nil {
		if err == sql.ErrNoRows {
			return &household.OnboardingState{
				UserID:    userID,
				Connected: false,
			}, nil
		}
		return nil, err
	}

	state := &household.OnboardingState{
		UserID:    userID,
		Connected: connectedInt == 1,
	}
	if connAtStr.Valid {
		if t, err := time.Parse(time.RFC3339, connAtStr.String); err == nil {
			state.ConnectedAt = &t
		}
	}
	return state, nil
}

// GetStatusList returns all active members alongside their connection state.
func (s *HouseholdStore) GetStatusList(ctx context.Context) ([]household.MemberStatus, error) {
	query := `
SELECT u.id, u.name, COALESCE(u.username, ''), u.locale, u.active, COALESCE(o.connected, 0)
FROM users u
LEFT JOIN user_onboarding o ON u.id = o.user_id
WHERE u.active = 1
ORDER BY u.name ASC;
`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query member status list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []household.MemberStatus
	for rows.Next() {
		var m household.Member
		var activeInt, connInt int
		if err := rows.Scan(&m.ID, &m.Name, &m.Username, &m.Locale, &activeInt, &connInt); err != nil {
			return nil, fmt.Errorf("failed to scan member status: %w", err)
		}
		m.Active = activeInt == 1
		results = append(results, household.MemberStatus{
			Member:    m,
			Connected: connInt == 1,
		})
	}
	return results, rows.Err()
}
