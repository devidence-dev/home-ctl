// Package shopping provides item catalog management, shared shopping lists,
// and idempotent purchase workflows for household members.
package shopping

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/px1/home-ctl/internal/household"
)

var (
	// ErrItemAlreadyExists is returned when attempting to create a catalog item with an existing name.
	ErrItemAlreadyExists = errors.New("shopping item already exists")
	// ErrEntryAlreadyPending is returned when an item is already on the pending list.
	ErrEntryAlreadyPending = errors.New("shopping item is already on the pending list")
	// ErrEntryNotFound is returned when a requested shopping entry does not exist.
	ErrEntryNotFound = errors.New("shopping entry not found")
	// ErrAlreadyPurchased is returned when an entry has already been marked as purchased.
	ErrAlreadyPurchased = errors.New("shopping entry has already been purchased")
	// ErrNotMember is returned when a non-member attempts a shopping operation.
	ErrNotMember = errors.New("user is not an active household member")
	// ErrBlankName is returned when a catalog item has an empty name.
	ErrBlankName = errors.New("shopping item name cannot be blank")
)

// Item represents a reusable catalog item.
type Item struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Emoji     string    `json:"emoji"`
	CreatedBy int64     `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// Entry represents an instance of an item on the shopping list.
type Entry struct {
	ID          int64      `json:"id"`
	ItemID      int64      `json:"item_id"`
	Item        Item       `json:"item"`
	Status      string     `json:"status"` // "pending" or "purchased"
	AddedBy     int64      `json:"added_by"`
	AddedAt     time.Time  `json:"added_at"`
	PurchasedBy *int64     `json:"purchased_by,omitempty"`
	PurchasedAt *time.Time `json:"purchased_at,omitempty"`
}

// MemberChecker verifies household membership.
type MemberChecker interface {
	Authorize(ctx context.Context, userID int64) (*household.Member, error)
}

// Store defines persistence methods for shopping catalog and list entries.
type Store interface {
	CreateItem(ctx context.Context, item Item) (*Item, error)
	GetItem(ctx context.Context, id int64) (*Item, error)
	GetItemByName(ctx context.Context, name string) (*Item, error)
	ListItems(ctx context.Context) ([]Item, error)
	CreateEntry(ctx context.Context, entry Entry) (*Entry, error)
	GetEntry(ctx context.Context, id int64) (*Entry, error)
	ListPendingEntries(ctx context.Context) ([]Entry, error)
	PurchaseEntry(ctx context.Context, entryID, purchaserID int64, purchasedAt time.Time) (*Entry, error)
}

// Service provides shopping business logic and authorization.
type Service struct {
	store   Store
	members MemberChecker
}

// NewService constructs a shopping Service.
func NewService(store Store, members MemberChecker) *Service {
	return &Service{
		store:   store,
		members: members,
	}
}

// CreateItem validates and registers a new catalog item.
func (s *Service) CreateItem(ctx context.Context, actorID int64, name, emoji string) (*Item, error) {
	if _, err := s.members.Authorize(ctx, actorID); err != nil {
		return nil, ErrNotMember
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, ErrBlankName
	}
	if emoji == "" {
		emoji = "🛒"
	}

	existing, err := s.store.GetItemByName(ctx, trimmed)
	if err == nil && existing != nil {
		return nil, ErrItemAlreadyExists
	}

	item := Item{
		Name:      trimmed,
		Emoji:     emoji,
		CreatedBy: actorID,
		CreatedAt: time.Now().UTC(),
	}
	return s.store.CreateItem(ctx, item)
}

// ListItems returns all catalog items.
func (s *Service) ListItems(ctx context.Context) ([]Item, error) {
	return s.store.ListItems(ctx)
}

// AddToList adds one or more catalog items to the pending list.
func (s *Service) AddToList(ctx context.Context, actorID int64, itemIDs []int64) ([]Entry, error) {
	if _, err := s.members.Authorize(ctx, actorID); err != nil {
		return nil, ErrNotMember
	}

	now := time.Now().UTC()
	var created []Entry

	for _, itemID := range itemIDs {
		item, err := s.store.GetItem(ctx, itemID)
		if err != nil {
			return nil, err
		}

		entry := Entry{
			ItemID:  itemID,
			Item:    *item,
			Status:  "pending",
			AddedBy: actorID,
			AddedAt: now,
		}
		newEntry, err := s.store.CreateEntry(ctx, entry)
		if err != nil {
			if errors.Is(err, ErrEntryAlreadyPending) {
				return nil, ErrEntryAlreadyPending
			}
			return nil, err
		}
		created = append(created, *newEntry)
	}

	return created, nil
}

// ListPending returns all currently needed shopping list entries.
func (s *Service) ListPending(ctx context.Context) ([]Entry, error) {
	return s.store.ListPendingEntries(ctx)
}

// Purchase marks a pending entry as purchased by an authorized household member.
func (s *Service) Purchase(ctx context.Context, actorID, entryID int64, purchasedAt time.Time) (*Entry, error) {
	if _, err := s.members.Authorize(ctx, actorID); err != nil {
		return nil, ErrNotMember
	}

	return s.store.PurchaseEntry(ctx, entryID, actorID, purchasedAt)
}
