package telegram_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/px1/home-ctl/internal/telegram"
)

func TestShoppingFlows_AndList(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// 1. /add in Shopping topic with empty catalog -> prompts for new item name
	uAdd := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAdd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "name for the new item") {
		t.Fatalf("expected prompt for new item name, got: %+v", last)
	}

	// 2. Alice types item name "Eggs"
	uName := telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID:       11,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Eggs",
		},
	}
	if err := env.router.HandleUpdate(ctx, uName); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "emoji for the new item") {
		t.Fatalf("expected prompt for emoji, got: %+v", last)
	}

	// 3. Alice sends emoji "🥚"
	uEmoji := telegram.Update{
		UpdateID: 3,
		Message: &telegram.Message{
			MessageID:       12,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "🥚",
		},
	}
	if err := env.router.HandleUpdate(ctx, uEmoji); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "Created 🥚 Eggs") {
		t.Fatalf("expected item created message, got: %+v", last)
	}

	// Verify item exists in catalog
	items, err := env.shopping.ListItems(ctx)
	if err != nil || len(items) != 1 || items[0].Name != "Eggs" {
		t.Fatalf("item was not created in catalog: %+v", items)
	}

	// 4. Duplicate name rejected: Bob tries to create "eggs"
	uAdd2 := telegram.Update{
		UpdateID: 4,
		Message: &telegram.Message{
			MessageID:       13,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 102, FirstName: "Bob"},
			Text:            "/add",
		},
	}
	_ = env.router.HandleUpdate(ctx, uAdd2)
	// Click "Add new item"
	uNewBtn := telegram.Update{
		UpdateID: 5,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_new",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID:       14,
				MessageThreadID: 42,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:new",
		},
	}
	_ = env.router.HandleUpdate(ctx, uNewBtn)

	// Bob types "eggs"
	uBobName := telegram.Update{
		UpdateID: 6,
		Message: &telegram.Message{
			MessageID:       15,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 102, FirstName: "Bob"},
			Text:            "eggs",
		},
	}
	_ = env.router.HandleUpdate(ctx, uBobName)
	// Bob types emoji
	uBobEmoji := telegram.Update{
		UpdateID: 7,
		Message: &telegram.Message{
			MessageID:       16,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 102, FirstName: "Bob"},
			Text:            "🥚",
		},
	}
	_ = env.router.HandleUpdate(ctx, uBobEmoji)
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "already exists") {
		t.Fatalf("expected duplicate item error, got: %+v", last)
	}

	// 5. Select Eggs to add to pending list
	uAdd3 := telegram.Update{
		UpdateID: 8,
		Message: &telegram.Message{
			MessageID:       17,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	_ = env.router.HandleUpdate(ctx, uAdd3)

	// Select Eggs button
	uSelect := telegram.Update{
		UpdateID: 9,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_sel",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       18,
				MessageThreadID: 42,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:sel:1",
		},
	}
	_ = env.router.HandleUpdate(ctx, uSelect)
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "Added 1 item") {
		t.Fatalf("expected item added notice, got: %+v", last)
	}

	// 6. /list in Shopping topic -> renders pending Eggs
	uList := telegram.Update{
		UpdateID: 10,
		Message: &telegram.Message{
			MessageID:       19,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/list",
		},
	}
	if err := env.router.HandleUpdate(ctx, uList); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || last.Markup == nil || len(last.Markup.InlineKeyboard) == 0 {
		t.Fatalf("expected list buttons, got: %+v", last)
	}
	btn := last.Markup.InlineKeyboard[0][0]
	if btn.Text != "🥚 Eggs" || btn.CallbackData != "sh:p:1" {
		t.Fatalf("unexpected button: %+v", btn)
	}

	// 7. Bob purchases Eggs (tap button)
	uPurchase := telegram.Update{
		UpdateID: 11,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_buy",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID:       20,
				MessageThreadID: 42,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "sh:p:1",
		},
	}
	if err := env.router.HandleUpdate(ctx, uPurchase); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "purchased by Bob") {
		t.Fatalf("expected group purchase update, got: %+v", last)
	}

	// 8. Idempotent double tap / already purchased
	uPurchase2 := telegram.Update{
		UpdateID: 12,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_buy_again",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       20,
				MessageThreadID: 42,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "sh:p:1",
		},
	}
	_ = env.router.HandleUpdate(ctx, uPurchase2)
	ans := env.client.LastAnswer()
	if ans == nil || !strings.Contains(ans.Text, "already purchased") {
		t.Fatalf("expected already purchased notice, got: %+v", ans)
	}

	// 9. Shopping /list is now empty
	uListEmpty := telegram.Update{
		UpdateID: 13,
		Message: &telegram.Message{
			MessageID:       21,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/list",
		},
	}
	_ = env.router.HandleUpdate(ctx, uListEmpty)
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "empty") {
		t.Fatalf("expected empty list notice, got: %+v", last)
	}
}

// Two members tapping the same entry at the same moment: exactly one purchase and one group update.
func TestShoppingPurchase_ConcurrentTapsPostOneGroupUpdate(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()

	item, err := env.shopping.CreateItem(ctx, 101, "Eggs", "🥚")
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}
	if _, err := env.shopping.AddToList(ctx, 101, []int64{item.ID}); err != nil {
		t.Fatalf("AddToList failed: %v", err)
	}
	groupPostsBefore := len(env.client.sentMessages)

	tap := func(updateID, userID int64) telegram.Update {
		return telegram.Update{
			UpdateID: updateID,
			CallbackQuery: &telegram.CallbackQuery{
				ID:   fmt.Sprintf("cb_%d", updateID),
				From: telegram.User{ID: userID},
				Message: &telegram.Message{
					MessageID: 20, MessageThreadID: 42,
					Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				},
				Data: "sh:p:1",
			},
		}
	}

	var wg sync.WaitGroup
	for i, userID := range []int64{101, 102, 101, 102} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = env.router.HandleUpdate(ctx, tap(int64(100+i), userID))
		}()
	}
	wg.Wait()

	purchasedPosts := 0
	env.client.mu.Lock()
	for _, m := range env.client.sentMessages[groupPostsBefore:] {
		if strings.Contains(m.Text, "purchased by") {
			purchasedPosts++
		}
	}
	env.client.mu.Unlock()
	if purchasedPosts != 1 {
		t.Fatalf("expected exactly one group purchase update, got %d", purchasedPosts)
	}
	pending, err := env.shopping.ListPending(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("expected the entry to be purchased exactly once, pending=%+v err=%v", pending, err)
	}
}
