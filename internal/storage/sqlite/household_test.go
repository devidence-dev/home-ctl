package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/config"
	"github.com/px1/home-ctl/internal/storage/sqlite"
)

func TestHouseholdStore_SyncMembers_RemovalKeepsHistory_AndUsernameChange(t *testing.T) {
	db := newTestDB(t)
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}
	ctx := context.Background()
	store := sqlite.NewHouseholdStore(db)

	initialConfig := []config.UserConfig{
		{ID: 1001, Name: "Alice", Username: "alice", Locale: "en"},
		{ID: 1002, Name: "Bob", Username: "bob", Locale: "es"},
	}

	if err := store.SyncMembers(ctx, initialConfig, "en"); err != nil {
		t.Fatalf("initial SyncMembers failed: %v", err)
	}

	m1, err := store.GetMember(ctx, 1001)
	if err != nil {
		t.Fatalf("failed to get Alice: %v", err)
	}
	if !m1.Active || m1.Username != "alice" {
		t.Fatalf("unexpected Alice state: %+v", m1)
	}

	// Username change for Alice: "alice" -> "alice_new"
	// Bob removed from config (only Alice in config)
	updatedConfig := []config.UserConfig{
		{ID: 1001, Name: "Alice", Username: "alice_new", Locale: "en"},
	}
	if err := store.SyncMembers(ctx, updatedConfig, "en"); err != nil {
		t.Fatalf("updated SyncMembers failed: %v", err)
	}

	// Verify Alice updated username
	m1Updated, err := store.GetMember(ctx, 1001)
	if err != nil {
		t.Fatalf("failed to get updated Alice: %v", err)
	}
	if m1Updated.Username != "alice_new" {
		t.Fatalf("expected username alice_new, got %s", m1Updated.Username)
	}
	if !m1Updated.Active {
		t.Fatalf("expected Alice to remain active")
	}

	// Verify Bob is deactivated, NOT deleted
	bob, err := store.GetMember(ctx, 1002)
	if err != nil {
		t.Fatalf("failed to get Bob: %v", err)
	}
	if bob.Active {
		t.Fatalf("expected Bob to be deactivated (active=false)")
	}

	// Active members list should only contain Alice
	activeList, err := store.ListMembers(ctx, true)
	if err != nil {
		t.Fatalf("failed to list active members: %v", err)
	}
	if len(activeList) != 1 || activeList[0].ID != 1001 {
		t.Fatalf("expected only Alice in active list, got %+v", activeList)
	}
}

func TestHouseholdStore_OnboardingAndStatus(t *testing.T) {
	db := newTestDB(t)
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}
	ctx := context.Background()
	store := sqlite.NewHouseholdStore(db)

	cfg := []config.UserConfig{
		{ID: 1001, Name: "Alice"},
		{ID: 1002, Name: "Bob"},
		{ID: 1003, Name: "Charlie"},
	}
	if err := store.SyncMembers(ctx, cfg, "en"); err != nil {
		t.Fatalf("SyncMembers failed: %v", err)
	}

	now := time.Now().UTC()
	// Alice and Bob onboard (/start)
	if err := store.RecordOnboarding(ctx, 1001, now); err != nil {
		t.Fatalf("Alice onboarding failed: %v", err)
	}
	if err := store.RecordOnboarding(ctx, 1002, now); err != nil {
		t.Fatalf("Bob onboarding failed: %v", err)
	}

	// Charlie is not connected
	charlieState, err := store.GetOnboardingState(ctx, 1003)
	if err != nil {
		t.Fatalf("failed to get Charlie onboarding: %v", err)
	}
	if charlieState.Connected {
		t.Fatalf("expected Charlie to be not connected")
	}

	// Check status list
	statusList, err := store.GetStatusList(ctx)
	if err != nil {
		t.Fatalf("GetStatusList failed: %v", err)
	}
	if len(statusList) != 3 {
		t.Fatalf("expected 3 members in status list, got %d", len(statusList))
	}

	var aliceConn, bobConn, charlieConn bool
	for _, s := range statusList {
		switch s.Member.ID {
		case 1001:
			aliceConn = s.Connected
		case 1002:
			bobConn = s.Connected
		case 1003:
			charlieConn = s.Connected
		}
	}
	if !aliceConn || !bobConn || charlieConn {
		t.Fatalf("expected Alice=true, Bob=true, Charlie=false; got Alice=%v, Bob=%v, Charlie=%v", aliceConn, bobConn, charlieConn)
	}

	// Bob disconnects (e.g. blocked bot)
	if err := store.RecordDisconnect(ctx, 1002); err != nil {
		t.Fatalf("RecordDisconnect failed: %v", err)
	}
	bobState, err := store.GetOnboardingState(ctx, 1002)
	if err != nil {
		t.Fatalf("failed to get Bob state: %v", err)
	}
	if bobState.Connected {
		t.Fatalf("expected Bob to be disconnected")
	}
}
