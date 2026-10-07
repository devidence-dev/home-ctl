package telegram_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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
	if last == nil || !strings.Contains(last.Text, "Added to the shopping list") {
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

func TestShoppingFlow_KeyboardsClosedOnItemPick_Cancel_AndExpired(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()

	// Seed catalog items
	milk, err := env.shopping.CreateItem(ctx, 101, "Milk", "🥛")
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}
	bread, err := env.shopping.CreateItem(ctx, 101, "Bread", "🍞")
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	// 1. Item pick closes menu keyboard
	uAdd1 := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAdd1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	menu1 := env.client.LastSent()
	if menu1 == nil || menu1.Markup == nil || len(menu1.Markup.InlineKeyboard) == 0 {
		t.Fatalf("expected menu with buttons, got: %+v", menu1)
	}

	uPick := telegram.Update{
		UpdateID: 2,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_pick",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       menu1.MessageID,
				MessageThreadID: 42,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            menu1.Text,
			},
			Data: fmt.Sprintf("fl:sel:%d", milk.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uPick); err != nil {
		t.Fatalf("unexpected error on item pick: %v", err)
	}
	lastEdited := env.client.LastEdited()
	if lastEdited == nil || lastEdited.MessageID != menu1.MessageID {
		t.Fatalf("expected menu message %d to be edited, got: %+v", menu1.MessageID, lastEdited)
	}
	if lastEdited.Markup == nil || len(lastEdited.Markup.InlineKeyboard) != 0 {
		t.Fatalf("expected empty keyboard after item pick, got: %+v", lastEdited.Markup)
	}
	if lastEdited.Text != menu1.Text {
		t.Fatalf("expected menu text %q preserved, got: %q", menu1.Text, lastEdited.Text)
	}

	// 2. Cancel closes menu keyboard
	uAdd2 := telegram.Update{
		UpdateID: 3,
		Message: &telegram.Message{
			MessageID:       11,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAdd2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	menu2 := env.client.LastSent()

	uCancel := telegram.Update{
		UpdateID: 4,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_cancel",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       menu2.MessageID,
				MessageThreadID: 42,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            menu2.Text,
			},
			Data: "fl:c",
		},
	}
	if err := env.router.HandleUpdate(ctx, uCancel); err != nil {
		t.Fatalf("unexpected error on cancel: %v", err)
	}
	lastEdited = env.client.LastEdited()
	if lastEdited == nil || lastEdited.MessageID != menu2.MessageID {
		t.Fatalf("expected menu message %d to be edited on cancel, got: %+v", menu2.MessageID, lastEdited)
	}
	if lastEdited.Markup == nil || len(lastEdited.Markup.InlineKeyboard) != 0 {
		t.Fatalf("expected empty keyboard after cancel, got: %+v", lastEdited.Markup)
	}
	ans := env.client.LastAnswer()
	if ans == nil || !strings.Contains(ans.Text, "Cancelled") {
		t.Fatalf("expected cancelled callback answer, got: %+v", ans)
	}

	// 3. Expired press leaves menu message edited with an empty keyboard
	uAdd3 := telegram.Update{
		UpdateID: 5,
		Message: &telegram.Message{
			MessageID:       12,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAdd3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	menu3 := env.client.LastSent()

	// Advance clock beyond 30 min FlowTTL
	env.clock.now = env.clock.now.Add(35 * time.Minute)

	uExpired := telegram.Update{
		UpdateID: 6,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_expired",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       menu3.MessageID,
				MessageThreadID: 42,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            menu3.Text,
			},
			Data: fmt.Sprintf("fl:sel:%d", bread.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uExpired); err != nil {
		t.Fatalf("unexpected error on expired tap: %v", err)
	}
	lastEdited = env.client.LastEdited()
	if lastEdited == nil || lastEdited.MessageID != menu3.MessageID {
		t.Fatalf("expected menu message %d to be edited on expiry, got: %+v", menu3.MessageID, lastEdited)
	}
	if lastEdited.Markup == nil || len(lastEdited.Markup.InlineKeyboard) != 0 {
		t.Fatalf("expected empty keyboard after expiry, got: %+v", lastEdited.Markup)
	}
	ans = env.client.LastAnswer()
	if ans == nil || !ans.Alert || !strings.Contains(ans.Text, "expired") {
		t.Fatalf("expected expired alert, got: %+v", ans)
	}
}

func TestShoppingFlow_RejectedPressLeavesKeyboard_AndListPurchaseKeepsRemainingButtons(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()

	milk, err := env.shopping.CreateItem(ctx, 101, "Milk", "🥛")
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}
	bread, err := env.shopping.CreateItem(ctx, 101, "Bread", "🍞")
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	// Add Milk to pending list
	if _, err := env.shopping.AddToList(ctx, 101, []int64{milk.ID}); err != nil {
		t.Fatalf("AddToList failed: %v", err)
	}

	// 1. Rejected press: item already pending neither edits the message nor clears the flow
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
	menu := env.client.LastSent()
	editsBefore := len(env.client.EditedMessages())

	uTapDuplicate := telegram.Update{
		UpdateID: 2,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_dup",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       menu.MessageID,
				MessageThreadID: 42,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            menu.Text,
			},
			Data: fmt.Sprintf("fl:sel:%d", milk.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uTapDuplicate); err != nil {
		t.Fatalf("unexpected error on duplicate tap: %v", err)
	}
	ans := env.client.LastAnswer()
	if ans == nil || !ans.Alert || !strings.Contains(ans.Text, "already") {
		t.Fatalf("expected already pending alert, got: %+v", ans)
	}
	if len(env.client.EditedMessages()) != editsBefore {
		t.Fatalf("expected NO message edits on rejected tap, but got edits: %+v", env.client.EditedMessages())
	}

	// Verify flow is still active
	flowKey := flowKeyFor(101, -1001234567890, 42)
	state, err := env.flow.Get(ctx, flowKey, nil)
	if err != nil || state == nil || state.Step != "shopping_select_items" {
		t.Fatalf("expected flow to remain in shopping_select_items, got: %+v err: %v", state, err)
	}

	// 2. Add Bread so list has 2 items, then purchase Milk and verify remaining button for Bread stays
	if _, err := env.shopping.AddToList(ctx, 101, []int64{bread.ID}); err != nil {
		t.Fatalf("AddToList failed: %v", err)
	}

	uList := telegram.Update{
		UpdateID: 3,
		Message: &telegram.Message{
			MessageID:       20,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/list",
		},
	}
	if err := env.router.HandleUpdate(ctx, uList); err != nil {
		t.Fatalf("unexpected error on /list: %v", err)
	}
	listMsg := env.client.LastSent()
	if listMsg == nil || listMsg.Markup == nil || len(listMsg.Markup.InlineKeyboard) != 2 {
		t.Fatalf("expected list message with 2 items, got: %+v", listMsg)
	}

	pending, err := env.shopping.ListPending(ctx)
	if err != nil || len(pending) != 2 {
		t.Fatalf("expected 2 pending entries, got: %+v", pending)
	}
	var milkEntryID int64
	for _, entry := range pending {
		if entry.Item.ID == milk.ID {
			milkEntryID = entry.ID
		}
	}

	// Bob purchases Milk
	uPurchase := telegram.Update{
		UpdateID: 4,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_buy_milk",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID:       listMsg.MessageID,
				MessageThreadID: 42,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            listMsg.Text,
			},
			Data: fmt.Sprintf("sh:p:%d", milkEntryID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uPurchase); err != nil {
		t.Fatalf("unexpected error on purchase: %v", err)
	}
	lastEdited := env.client.LastEdited()
	if lastEdited == nil || lastEdited.MessageID != listMsg.MessageID {
		t.Fatalf("expected list message %d to be refreshed, got: %+v", listMsg.MessageID, lastEdited)
	}
	if lastEdited.Markup == nil || len(lastEdited.Markup.InlineKeyboard) != 1 {
		t.Fatalf("expected shopping list refresh to retain remaining button, got markup: %+v", lastEdited.Markup)
	}
	if lastEdited.Markup.InlineKeyboard[0][0].Text != "🍞 Bread" {
		t.Fatalf("expected Bread button to remain, got: %s", lastEdited.Markup.InlineKeyboard[0][0].Text)
	}
}
