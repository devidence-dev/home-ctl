package sqlite_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/config"
	"github.com/px1/home-ctl/internal/shopping"
	"github.com/px1/home-ctl/internal/storage/sqlite"
)

func TestShoppingStore_ConcurrentPurchaseAndConstraints(t *testing.T) {
	db := newTestDB(t)
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}
	ctx := context.Background()

	hStore := sqlite.NewHouseholdStore(db)
	_ = hStore.SyncMembers(ctx, []config.UserConfig{
		{ID: 1001, Name: "Alice"},
		{ID: 1002, Name: "Bob"},
	}, "en")

	shopStore := sqlite.NewShoppingStore(db)

	// Create item
	item, err := shopStore.CreateItem(ctx, shopping.Item{
		Name:      "Eggs",
		Emoji:     "🥚",
		CreatedBy: 1001,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to create item: %v", err)
	}

	// Duplicate case-insensitive
	_, err = shopStore.CreateItem(ctx, shopping.Item{
		Name:      "eggs",
		Emoji:     "🥚",
		CreatedBy: 1002,
		CreatedAt: time.Now().UTC(),
	})
	if !errors.Is(err, shopping.ErrItemAlreadyExists) {
		t.Fatalf("expected ErrItemAlreadyExists, got %v", err)
	}

	// Add entry
	entry, err := shopStore.CreateEntry(ctx, shopping.Entry{
		ItemID:  item.ID,
		AddedBy: 1001,
		AddedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to create entry: %v", err)
	}

	// Duplicate pending rejected
	_, err = shopStore.CreateEntry(ctx, shopping.Entry{
		ItemID:  item.ID,
		AddedBy: 1002,
		AddedAt: time.Now().UTC(),
	})
	if !errors.Is(err, shopping.ErrEntryAlreadyPending) {
		t.Fatalf("expected ErrEntryAlreadyPending, got %v", err)
	}

	// Concurrent purchase by Alice and Bob
	var wg sync.WaitGroup
	results := make([]error, 2)
	purchasers := []int64{1001, 1002}

	for i := 0; i < 2; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			_, err := shopStore.PurchaseEntry(ctx, entry.ID, purchasers[idx], time.Now().UTC())
			results[idx] = err
		}()
	}
	wg.Wait()

	// Exactly one purchase succeeds, the other gets ErrAlreadyPurchased
	successes := 0
	alreadyPurchased := 0
	for _, err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, shopping.ErrAlreadyPurchased) {
			alreadyPurchased++
		} else {
			t.Fatalf("unexpected error during concurrent purchase: %v", err)
		}
	}
	if successes != 1 || alreadyPurchased != 1 {
		t.Fatalf("expected 1 success and 1 ErrAlreadyPurchased, got %d successes and %d alreadyPurchased", successes, alreadyPurchased)
	}

	// Verify final entry state has purchaser and is marked purchased
	finalEntry, err := shopStore.GetEntry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("GetEntry failed: %v", err)
	}
	if finalEntry.Status != "purchased" || finalEntry.PurchasedBy == nil {
		t.Fatalf("final entry not purchased: %+v", finalEntry)
	}

	// Re-add eggs after purchase
	readded, err := shopStore.CreateEntry(ctx, shopping.Entry{
		ItemID:  item.ID,
		AddedBy: 1001,
		AddedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to re-add item after purchase: %v", err)
	}
	if readded.ID == entry.ID {
		t.Fatalf("expected new entry ID for re-added item")
	}

	// List pending has 1 item
	pending, err := shopStore.ListPendingEntries(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatalf("expected 1 pending entry, got %v", pending)
	}

	// Exercise ListItems and GetItem
	items, err := shopStore.ListItems(ctx)
	if err != nil || len(items) == 0 {
		t.Fatalf("ListItems failed: %v", err)
	}
	fetched, err := shopStore.GetItem(ctx, item.ID)
	if err != nil || fetched.Name != "Eggs" {
		t.Fatalf("GetItem failed: %v", err)
	}
}
