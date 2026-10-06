package shopping_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/shopping"
)

type fakeMemberChecker struct {
	allowed map[int64]bool
}

func (f *fakeMemberChecker) Authorize(_ context.Context, userID int64) (*household.Member, error) {
	if f.allowed[userID] {
		return &household.Member{ID: userID, Active: true}, nil
	}
	return nil, household.ErrNotMember
}

type fakeShoppingStore struct {
	mu      sync.Mutex
	items   map[int64]*shopping.Item
	entries map[int64]*shopping.Entry
	nextID  int64
}

func newFakeShoppingStore() *fakeShoppingStore {
	return &fakeShoppingStore{
		items:   make(map[int64]*shopping.Item),
		entries: make(map[int64]*shopping.Entry),
		nextID:  1,
	}
}

func (f *fakeShoppingStore) CreateItem(_ context.Context, item shopping.Item) (*shopping.Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, it := range f.items {
		if strings.EqualFold(it.Name, item.Name) {
			return nil, shopping.ErrItemAlreadyExists
		}
	}
	item.ID = f.nextID
	f.nextID++
	f.items[item.ID] = &item
	return &item, nil
}

func (f *fakeShoppingStore) GetItem(_ context.Context, id int64) (*shopping.Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	it, ok := f.items[id]
	if !ok {
		return nil, errors.New("item not found")
	}
	return it, nil
}

func (f *fakeShoppingStore) GetItemByName(_ context.Context, name string) (*shopping.Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, it := range f.items {
		if strings.EqualFold(it.Name, name) {
			return it, nil
		}
	}
	return nil, nil
}

func (f *fakeShoppingStore) ListItems(_ context.Context) ([]shopping.Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var list []shopping.Item
	for _, it := range f.items {
		list = append(list, *it)
	}
	return list, nil
}

func (f *fakeShoppingStore) CreateEntry(_ context.Context, entry shopping.Entry) (*shopping.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.entries {
		if e.ItemID == entry.ItemID && e.Status == "pending" {
			return nil, shopping.ErrEntryAlreadyPending
		}
	}
	entry.ID = f.nextID
	f.nextID++
	f.entries[entry.ID] = &entry
	return &entry, nil
}

func (f *fakeShoppingStore) GetEntry(_ context.Context, id int64) (*shopping.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[id]
	if !ok {
		return nil, shopping.ErrEntryNotFound
	}
	return e, nil
}

func (f *fakeShoppingStore) ListPendingEntries(_ context.Context) ([]shopping.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var list []shopping.Entry
	for _, e := range f.entries {
		if e.Status == "pending" {
			list = append(list, *e)
		}
	}
	return list, nil
}

func (f *fakeShoppingStore) PurchaseEntry(_ context.Context, entryID, purchaserID int64, purchasedAt time.Time) (*shopping.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[entryID]
	if !ok {
		return nil, shopping.ErrEntryNotFound
	}
	if e.Status == "purchased" {
		return e, shopping.ErrAlreadyPurchased
	}
	e.Status = "purchased"
	e.PurchasedBy = &purchaserID
	e.PurchasedAt = &purchasedAt
	return e, nil
}

func TestShoppingService_CatalogAndAdd(t *testing.T) {
	ctx := context.Background()
	members := &fakeMemberChecker{allowed: map[int64]bool{1001: true, 1002: true}}
	store := newFakeShoppingStore()
	svc := shopping.NewService(store, members)

	// Non-member rejected
	_, err := svc.CreateItem(ctx, 9999, "Eggs", "🥚")
	if !errors.Is(err, shopping.ErrNotMember) {
		t.Fatalf("expected ErrNotMember, got %v", err)
	}

	// Blank name rejected
	_, err = svc.CreateItem(ctx, 1001, "   ", "🥚")
	if !errors.Is(err, shopping.ErrBlankName) {
		t.Fatalf("expected ErrBlankName, got %v", err)
	}

	// Create valid item
	item1, err := svc.CreateItem(ctx, 1001, "Eggs", "🥚")
	if err != nil || item1.ID == 0 {
		t.Fatalf("failed to create item: %v", err)
	}

	// Duplicate case-insensitive name rejected
	_, err = svc.CreateItem(ctx, 1002, "eggs", "🥚")
	if !errors.Is(err, shopping.ErrItemAlreadyExists) {
		t.Fatalf("expected ErrItemAlreadyExists for 'eggs', got %v", err)
	}

	// Add item to list
	entries, err := svc.AddToList(ctx, 1001, []int64{item1.ID})
	if err != nil || len(entries) != 1 {
		t.Fatalf("AddToList failed: %v", err)
	}

	// Duplicate pending rejected
	_, err = svc.AddToList(ctx, 1002, []int64{item1.ID})
	if !errors.Is(err, shopping.ErrEntryAlreadyPending) {
		t.Fatalf("expected ErrEntryAlreadyPending, got %v", err)
	}

	// Purchase item
	purchased, err := svc.Purchase(ctx, 1002, entries[0].ID, time.Now())
	if err != nil {
		t.Fatalf("purchase failed: %v", err)
	}
	if purchased.Status != "purchased" || *purchased.PurchasedBy != 1002 {
		t.Fatalf("unexpected purchased entry state: %+v", purchased)
	}

	// Double tap (already purchased)
	_, err = svc.Purchase(ctx, 1002, entries[0].ID, time.Now())
	if !errors.Is(err, shopping.ErrAlreadyPurchased) {
		t.Fatalf("expected ErrAlreadyPurchased on second tap, got %v", err)
	}

	// Can re-add purchased item
	readded, err := svc.AddToList(ctx, 1001, []int64{item1.ID})
	if err != nil || len(readded) != 1 {
		t.Fatalf("failed to re-add purchased item: %v", err)
	}
	if readded[0].ID == entries[0].ID {
		t.Fatalf("expected new entry ID for re-added item")
	}

	// Exercise ListItems and ListPending
	all, err := svc.ListItems(ctx)
	if err != nil || len(all) == 0 {
		t.Fatalf("ListItems failed: %v", err)
	}
	pending, err := svc.ListPending(ctx)
	if err != nil || len(pending) == 0 {
		t.Fatalf("ListPending failed: %v", err)
	}
}
