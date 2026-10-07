package flow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/flow"
)

type fakeFlowStore struct {
	states map[flow.Key]*flow.State
}

func newFakeFlowStore() *fakeFlowStore {
	return &fakeFlowStore{states: make(map[flow.Key]*flow.State)}
}

func (f *fakeFlowStore) Set(_ context.Context, key flow.Key, step string, payload []byte, expiresAt time.Time) error {
	f.states[key] = &flow.State{
		Key:       key,
		Step:      step,
		Payload:   payload,
		ExpiresAt: expiresAt,
	}
	return nil
}

func (f *fakeFlowStore) Get(_ context.Context, key flow.Key, now time.Time) (*flow.State, error) {
	st, ok := f.states[key]
	if !ok {
		return nil, flow.ErrNoFlow
	}
	if now.After(st.ExpiresAt) {
		delete(f.states, key)
		return nil, flow.ErrFlowExpired
	}
	return st, nil
}

func (f *fakeFlowStore) SetMessageID(_ context.Context, key flow.Key, messageID int64) error {
	st, ok := f.states[key]
	if !ok {
		return flow.ErrNoFlow
	}
	st.MessageID = messageID
	return nil
}

func (f *fakeFlowStore) GetByMessage(_ context.Context, chatID int64, threadID int64, messageID int64, now time.Time) (*flow.State, error) {
	for k, st := range f.states {
		if k.ChatID == chatID && k.ThreadID == threadID && st.MessageID == messageID && messageID != 0 {
			if now.After(st.ExpiresAt) {
				delete(f.states, k)
				return nil, flow.ErrFlowExpired
			}
			return st, nil
		}
	}
	return nil, flow.ErrNoFlow
}

func (f *fakeFlowStore) Delete(_ context.Context, key flow.Key) error {
	delete(f.states, key)
	return nil
}

func (f *fakeFlowStore) PurgeExpired(_ context.Context, now time.Time) (int64, error) {
	var count int64
	for k, v := range f.states {
		if now.After(v.ExpiresAt) {
			delete(f.states, k)
			count++
		}
	}
	return count, nil
}

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

type samplePayload struct {
	ItemID int64  `json:"item_id"`
	Note   string `json:"note"`
}

func TestFlowManager(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}
	store := newFakeFlowStore()
	mgr := flow.NewManager(store, clk)

	key := flow.Key{UserID: 1001, ChatID: -100, ThreadID: 42}
	payload := samplePayload{ItemID: 42, Note: "eggs"}

	// 1. Set state
	err := mgr.Set(ctx, key, "choose_date", payload, 10*time.Minute)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// 2. Get state and unmarshal
	var retrieved samplePayload
	st, err := mgr.Get(ctx, key, &retrieved)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if st.Step != "choose_date" || retrieved.ItemID != 42 || retrieved.Note != "eggs" {
		t.Fatalf("unexpected retrieved state: %+v, payload=%+v", st, retrieved)
	}

	// 3. Clear state
	if err := mgr.Clear(ctx, key); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
	_, err = mgr.Get(ctx, key, &retrieved)
	if !errors.Is(err, flow.ErrNoFlow) {
		t.Fatalf("expected ErrNoFlow, got %v", err)
	}

	// 4. Expiry
	_ = mgr.Set(ctx, key, "exp", nil, time.Minute)
	clk.now = clk.now.Add(2 * time.Minute)
	_, err = mgr.Get(ctx, key, nil)
	if !errors.Is(err, flow.ErrFlowExpired) {
		t.Fatalf("expected ErrFlowExpired, got %v", err)
	}

	// 5. Purge
	_ = store.Set(ctx, flow.Key{UserID: 2}, "step", []byte(`{}`), now.Add(-time.Hour))
	purged, err := mgr.Purge(ctx)
	if err != nil || purged != 1 {
		t.Fatalf("Purge failed: %v, count=%d", err, purged)
	}

	// 6. SetMessageID and GetByMessage
	keyMsg := flow.Key{UserID: 2001, ChatID: -200, ThreadID: 10}
	if err := mgr.Set(ctx, keyMsg, "step_msg", payload, 10*time.Minute); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := mgr.SetMessageID(ctx, keyMsg, 9999); err != nil {
		t.Fatalf("SetMessageID failed: %v", err)
	}
	var msgPayload samplePayload
	stByMsg, err := mgr.GetByMessage(ctx, -200, 10, 9999, &msgPayload)
	if err != nil {
		t.Fatalf("GetByMessage failed: %v", err)
	}
	if stByMsg.Key.UserID != 2001 || stByMsg.MessageID != 9999 || msgPayload.Note != "eggs" {
		t.Fatalf("unexpected state by message: %+v", stByMsg)
	}

	// Non-existent message
	_, err = mgr.GetByMessage(ctx, -200, 10, 8888, nil)
	if !errors.Is(err, flow.ErrNoFlow) {
		t.Fatalf("expected ErrNoFlow, got %v", err)
	}
}
