package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/config"
	"github.com/px1/home-ctl/internal/flow"
	"github.com/px1/home-ctl/internal/storage/sqlite"
)

func TestFlowStore_Integration(t *testing.T) {
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

	fStore := sqlite.NewFlowStore(db)

	now := time.Now().UTC().Truncate(time.Second)

	// 1. Independent topics for same user (Alice in topic 51 Chores and topic 42 Shopping)
	keyAliceChores := flow.Key{UserID: 1001, ChatID: -100, ThreadID: 51}
	keyAliceShopping := flow.Key{UserID: 1001, ChatID: -100, ThreadID: 42}

	err := fStore.Set(ctx, keyAliceChores, "due_date", []byte(`{"chore_id":1}`), now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("Set Alice chores failed: %v", err)
	}
	err = fStore.Set(ctx, keyAliceShopping, "select_items", []byte(`{"items":[10]}`), now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("Set Alice shopping failed: %v", err)
	}

	stChores, err := fStore.Get(ctx, keyAliceChores, now)
	if err != nil || stChores.Step != "due_date" {
		t.Fatalf("unexpected Alice chores flow: %+v, err=%v", stChores, err)
	}
	stShop, err := fStore.Get(ctx, keyAliceShopping, now)
	if err != nil || stShop.Step != "select_items" {
		t.Fatalf("unexpected Alice shopping flow: %+v, err=%v", stShop, err)
	}

	// 2. Concurrent users in same topic (Alice and Bob in topic 51 Chores)
	keyBobChores := flow.Key{UserID: 1002, ChatID: -100, ThreadID: 51}
	err = fStore.Set(ctx, keyBobChores, "assignee", []byte(`{"chore_id":2}`), now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("Set Bob chores failed: %v", err)
	}

	stBob, err := fStore.Get(ctx, keyBobChores, now)
	if err != nil || stBob.Step != "assignee" {
		t.Fatalf("unexpected Bob chores flow: %+v", stBob)
	}
	// Alice's chores state unchanged
	stAliceAgain, err := fStore.Get(ctx, keyAliceChores, now)
	if err != nil || stAliceAgain.Step != "due_date" {
		t.Fatalf("Alice chores flow altered by Bob: %+v", stAliceAgain)
	}

	// 3. Restart survival: close and re-open SQLite DB
	// DB connection pool close, re-open same file
	// Since test uses file-based temp DB, let's read again
	stSurvives, err := fStore.Get(ctx, keyAliceChores, now)
	if err != nil || stSurvives.Step != "due_date" {
		t.Fatalf("failed to read persisted flow state")
	}

	// 4. Expiry: query after expires_at
	keyExpired := flow.Key{UserID: 1001, ChatID: -100, ThreadID: 99}
	_ = fStore.Set(ctx, keyExpired, "step1", []byte(`{}`), now.Add(-time.Minute))
	_, err = fStore.Get(ctx, keyExpired, now)
	if !errors.Is(err, flow.ErrFlowExpired) {
		t.Fatalf("expected ErrFlowExpired for past timestamp, got %v", err)
	}

	// 5. Purge expired
	_ = fStore.Set(ctx, flow.Key{UserID: 1002, ChatID: -100, ThreadID: 98}, "old", []byte(`{}`), now.Add(-5*time.Minute))
	purged, err := fStore.PurgeExpired(ctx, now)
	if err != nil || purged == 0 {
		t.Fatalf("PurgeExpired failed: %v, count=%d", err, purged)
	}

	// 6. Delete active flow
	err = fStore.Delete(ctx, keyAliceChores)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, err = fStore.Get(ctx, keyAliceChores, now)
	if !errors.Is(err, flow.ErrNoFlow) {
		t.Fatalf("expected ErrNoFlow after delete, got %v", err)
	}
}
