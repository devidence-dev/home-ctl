package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/px1/home-ctl/internal/shopping"
)

// ShoppingStore provides SQLite persistence for shopping catalog and entries.
type ShoppingStore struct {
	db *sql.DB
}

// NewShoppingStore creates a new ShoppingStore.
func NewShoppingStore(db *sql.DB) *ShoppingStore {
	return &ShoppingStore{db: db}
}

// CreateItem inserts a new catalog item.
func (s *ShoppingStore) CreateItem(ctx context.Context, item shopping.Item) (*shopping.Item, error) {
	nowStr := item.CreatedAt.UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
INSERT INTO shopping_items (name, emoji, created_by, created_at)
VALUES (?, ?, ?, ?)`, item.Name, item.Emoji, item.CreatedBy, nowStr)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, shopping.ErrItemAlreadyExists
		}
		return nil, fmt.Errorf("failed to insert shopping item: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	item.ID = id
	return &item, nil
}

// GetItem retrieves a catalog item by ID.
func (s *ShoppingStore) GetItem(ctx context.Context, id int64) (*shopping.Item, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, emoji, created_by, created_at
FROM shopping_items WHERE id = ?`, id)
	return scanItem(row)
}

// GetItemByName retrieves a catalog item by case-insensitive name.
func (s *ShoppingStore) GetItemByName(ctx context.Context, name string) (*shopping.Item, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, emoji, created_by, created_at
FROM shopping_items WHERE name = ? COLLATE NOCASE`, name)
	item, err := scanItem(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

// ListItems returns all catalog items sorted alphabetically.
func (s *ShoppingStore) ListItems(ctx context.Context) ([]shopping.Item, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, emoji, created_by, created_at
FROM shopping_items ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query shopping items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []shopping.Item
	for rows.Next() {
		var it shopping.Item
		var createdStr string
		if err := rows.Scan(&it.ID, &it.Name, &it.Emoji, &it.CreatedBy, &createdStr); err != nil {
			return nil, err
		}
		it.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		items = append(items, it)
	}
	return items, rows.Err()
}

// CreateEntry adds a new pending item entry to the list.
func (s *ShoppingStore) CreateEntry(ctx context.Context, entry shopping.Entry) (*shopping.Entry, error) {
	nowStr := entry.AddedAt.UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
INSERT INTO shopping_entries (item_id, status, added_by, added_at)
VALUES (?, 'pending', ?, ?)`, entry.ItemID, entry.AddedBy, nowStr)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, shopping.ErrEntryAlreadyPending
		}
		return nil, fmt.Errorf("failed to insert shopping entry: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetEntry(ctx, id)
}

// GetEntry retrieves a shopping entry and its associated catalog item.
func (s *ShoppingStore) GetEntry(ctx context.Context, id int64) (*shopping.Entry, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT e.id, e.item_id, i.name, i.emoji, i.created_by, i.created_at,
       e.status, e.added_by, e.added_at, e.purchased_by, e.purchased_at
FROM shopping_entries e
JOIN shopping_items i ON e.item_id = i.id
WHERE e.id = ?`, id)
	return scanEntry(row)
}

// ListPendingEntries returns all pending shopping entries.
func (s *ShoppingStore) ListPendingEntries(ctx context.Context) ([]shopping.Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT e.id, e.item_id, i.name, i.emoji, i.created_by, i.created_at,
       e.status, e.added_by, e.added_at, e.purchased_by, e.purchased_at
FROM shopping_entries e
JOIN shopping_items i ON e.item_id = i.id
WHERE e.status = 'pending'
ORDER BY e.added_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []shopping.Entry
	for rows.Next() {
		entry, err := scanEntryRow(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, *entry)
	}
	return entries, rows.Err()
}

// PurchaseEntry conditionally transitions an entry from pending to purchased.
func (s *ShoppingStore) PurchaseEntry(ctx context.Context, entryID, purchaserID int64, purchasedAt time.Time) (*shopping.Entry, error) {
	nowStr := purchasedAt.UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
UPDATE shopping_entries
SET status = 'purchased', purchased_by = ?, purchased_at = ?
WHERE id = ? AND status = 'pending'`, purchaserID, nowStr, entryID)
	if err != nil {
		return nil, fmt.Errorf("failed to update shopping entry: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		// Entry already purchased or nonexistent
		entry, err := s.GetEntry(ctx, entryID)
		if err != nil {
			return nil, shopping.ErrEntryNotFound
		}
		if entry.Status == "purchased" {
			return entry, shopping.ErrAlreadyPurchased
		}
		return nil, shopping.ErrEntryNotFound
	}

	return s.GetEntry(ctx, entryID)
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanItem(s rowScanner) (*shopping.Item, error) {
	var it shopping.Item
	var createdStr string
	if err := s.Scan(&it.ID, &it.Name, &it.Emoji, &it.CreatedBy, &createdStr); err != nil {
		return nil, err
	}
	it.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &it, nil
}

func scanEntry(s rowScanner) (*shopping.Entry, error) {
	return scanEntryRow(s)
}

func scanEntryRow(s rowScanner) (*shopping.Entry, error) {
	var e shopping.Entry
	var itCreatedStr, addedStr string
	var purchBy sql.NullInt64
	var purchAtStr sql.NullString

	if err := s.Scan(
		&e.ID, &e.ItemID, &e.Item.Name, &e.Item.Emoji, &e.Item.CreatedBy, &itCreatedStr,
		&e.Status, &e.AddedBy, &addedStr, &purchBy, &purchAtStr,
	); err != nil {
		return nil, err
	}

	e.Item.ID = e.ItemID
	e.Item.CreatedAt, _ = time.Parse(time.RFC3339, itCreatedStr)
	e.AddedAt, _ = time.Parse(time.RFC3339, addedStr)

	if purchBy.Valid {
		val := purchBy.Int64
		e.PurchasedBy = &val
	}
	if purchAtStr.Valid {
		t, _ := time.Parse(time.RFC3339, purchAtStr.String)
		e.PurchasedAt = &t
	}

	return &e, nil
}
