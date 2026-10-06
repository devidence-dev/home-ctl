package household_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/config"
	"github.com/px1/home-ctl/internal/household"
)

type fakeStore struct {
	members    map[int64]*household.Member
	onboarding map[int64]*household.OnboardingState
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		members:    make(map[int64]*household.Member),
		onboarding: make(map[int64]*household.OnboardingState),
	}
}

func (f *fakeStore) SyncMembers(_ context.Context, users []config.UserConfig, defaultLocale string) error {
	newIDs := make(map[int64]bool)
	for _, u := range users {
		newIDs[u.ID] = true
		loc := u.Locale
		if loc == "" {
			loc = defaultLocale
		}
		f.members[u.ID] = &household.Member{
			ID:       u.ID,
			Name:     u.Name,
			Username: u.Username,
			Locale:   loc,
			Active:   true,
		}
		if _, ok := f.onboarding[u.ID]; !ok {
			f.onboarding[u.ID] = &household.OnboardingState{
				UserID:    u.ID,
				Connected: false,
			}
		}
	}
	for id, m := range f.members {
		if !newIDs[id] {
			m.Active = false
		}
	}
	return nil
}

func (f *fakeStore) GetMember(_ context.Context, id int64) (*household.Member, error) {
	m, ok := f.members[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return m, nil
}

func (f *fakeStore) ListMembers(_ context.Context, activeOnly bool) ([]household.Member, error) {
	var list []household.Member
	for _, m := range f.members {
		if !activeOnly || m.Active {
			list = append(list, *m)
		}
	}
	return list, nil
}

func (f *fakeStore) RecordOnboarding(_ context.Context, userID int64, connectedAt time.Time) error {
	f.onboarding[userID] = &household.OnboardingState{
		UserID:      userID,
		Connected:   true,
		ConnectedAt: &connectedAt,
	}
	return nil
}

func (f *fakeStore) RecordDisconnect(_ context.Context, userID int64) error {
	if o, ok := f.onboarding[userID]; ok {
		o.Connected = false
	}
	return nil
}

func (f *fakeStore) GetOnboardingState(_ context.Context, userID int64) (*household.OnboardingState, error) {
	if o, ok := f.onboarding[userID]; ok {
		return o, nil
	}
	return &household.OnboardingState{UserID: userID, Connected: false}, nil
}

func (f *fakeStore) GetStatusList(_ context.Context) ([]household.MemberStatus, error) {
	var list []household.MemberStatus
	for _, m := range f.members {
		if m.Active {
			conn := false
			if o, ok := f.onboarding[m.ID]; ok {
				conn = o.Connected
			}
			list = append(list, household.MemberStatus{
				Member:    *m,
				Connected: conn,
			})
		}
	}
	return list, nil
}

func TestHouseholdService(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc := household.NewService(store)

	// 1. Sync from config
	err := svc.SyncFromConfig(ctx, []config.UserConfig{
		{ID: 1001, Name: "Alice", Locale: "en"},
		{ID: 1002, Name: "Bob", Locale: "es"},
	}, "en")
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// 2. Authorize
	alice, err := svc.Authorize(ctx, 1001)
	if err != nil || alice.Name != "Alice" {
		t.Fatalf("authorize Alice failed: %v", err)
	}

	// Non-member authorization fails with ErrNotMember
	_, err = svc.Authorize(ctx, 9999)
	if !errors.Is(err, household.ErrNotMember) {
		t.Fatalf("expected ErrNotMember for unknown user, got %v", err)
	}

	// 3. HandlePrivateStart
	// Non-member sends /start
	_, err = svc.HandlePrivateStart(ctx, 9999, time.Now())
	if !errors.Is(err, household.ErrNotMember) {
		t.Fatalf("expected ErrNotMember for unknown user /start, got %v", err)
	}

	// Alice sends /start
	m, err := svc.HandlePrivateStart(ctx, 1001, time.Now())
	if err != nil || m.ID != 1001 {
		t.Fatalf("Alice /start failed: %v", err)
	}

	// Alice is now connected
	conn, err := svc.IsConnected(ctx, 1001)
	if err != nil || !conn {
		t.Fatalf("expected Alice connected, got %v, err=%v", conn, err)
	}

	// Bob is not connected
	connBob, err := svc.IsConnected(ctx, 1002)
	if err != nil || connBob {
		t.Fatalf("expected Bob not connected, got %v", connBob)
	}

	// 4. Status list
	statusList, err := svc.GetStatus(ctx)
	if err != nil || len(statusList) != 2 {
		t.Fatalf("expected 2 status entries, got %v", statusList)
	}

	// 5. Disconnect
	if err := svc.HandleDisconnect(ctx, 1001); err != nil {
		t.Fatalf("disconnect failed: %v", err)
	}
	connAfter, err := svc.IsConnected(ctx, 1001)
	if err != nil || connAfter {
		t.Fatalf("expected Alice disconnected after HandleDisconnect")
	}

	// 6. Deactivated user authorization
	err = svc.SyncFromConfig(ctx, []config.UserConfig{
		{ID: 1001, Name: "Alice"},
	}, "en")
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	// Bob is now removed from config
	_, err = svc.Authorize(ctx, 1002)
	if !errors.Is(err, household.ErrMemberInactive) {
		t.Fatalf("expected ErrMemberInactive for Bob, got %v", err)
	}

	// ListActiveMembers
	active, err := svc.ListActiveMembers(ctx)
	if err != nil || len(active) != 1 {
		t.Fatalf("expected 1 active member, got %v", active)
	}
}
