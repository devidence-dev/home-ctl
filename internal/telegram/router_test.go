package telegram_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/flow"
	"github.com/px1/home-ctl/internal/telegram"
)

func TestRouter_TopicRouting(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// 1. /add in Shopping topic (42) starts shopping flow
	uShopping := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uShopping); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "item") {
		t.Fatalf("expected shopping prompt, got: %+v", last)
	}

	// 2. /add in Chores topic (51) starts chores flow
	uChores := telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID:       11,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uChores); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "chore") {
		t.Fatalf("expected chore prompt, got: %+v", last)
	}

	// 3. /add in Reminders topic (63) starts reminders flow
	uReminders := telegram.Update{
		UpdateID: 3,
		Message: &telegram.Message{
			MessageID:       12,
			MessageThreadID: 63,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uReminders); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "reminder") {
		t.Fatalf("expected reminder prompt, got: %+v", last)
	}
}

func TestRouter_UnmappedTopic(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()
	u := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       20,
			MessageThreadID: 999, // Unmapped topic
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "not configured") {
		t.Fatalf("expected unmapped topic notice, got: %+v", last)
	}

	// /list in unmapped topic also returns unmapped topic notice
	uList := telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID:       21,
			MessageThreadID: 999, // Unmapped topic
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/list",
		},
	}
	if err := env.router.HandleUpdate(ctx, uList); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	expectedNotice := env.i18n.T("en", "general.unmapped_topic")
	if last == nil || last.Text != expectedNotice {
		t.Fatalf("expected unmapped topic notice %q for /list, got: %+v", expectedNotice, last)
	}
}

func TestRouter_WrongChat(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()
	sentBefore := len(env.client.sentMessages)

	// Command from wrong group chat ID
	u := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       30,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -999999999, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(env.client.sentMessages) != sentBefore {
		t.Errorf("expected wrong chat message to be ignored, but message was sent")
	}
}

func TestRouter_UnauthorizedUser(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()
	sentBefore := len(env.client.sentMessages)

	// Unauthorized user sends /add in shopping topic
	uGroup := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       40,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 555000111, FirstName: "Stranger"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uGroup); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.client.sentMessages) != sentBefore {
		t.Errorf("expected unauthorized group message to produce no output")
	}
	_, err = env.flow.Get(ctx, flow.Key{UserID: 555000111, ChatID: -1001234567890, ThreadID: 42}, nil)
	if !errors.Is(err, flow.ErrNoFlow) {
		t.Errorf("expected no flow created for unauthorized user, got %v", err)
	}

	// Unauthorized user sends /start in private chat -> gets not configured reply
	uPrivate := telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 41,
			Chat:      telegram.Chat{ID: 555000111, Type: "private"},
			From:      &telegram.User{ID: 555000111, FirstName: "Stranger"},
			Text:      "/start",
		},
	}
	if err := env.router.HandleUpdate(ctx, uPrivate); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "not an authorized") {
		t.Fatalf("expected not authorized notice, got: %+v", last)
	}
}

func TestRouter_MalformedCallback(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// Malformed callback
	u := telegram.Update{
		UpdateID: 1,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_malformed",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID: 50,
				Chat:      telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "invalid:callback:data:too:many:parts",
		},
	}
	if err := env.router.HandleUpdate(ctx, u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	answer := env.client.LastAnswer()
	if answer == nil || answer.QueryID != "cb_malformed" {
		t.Fatalf("expected callback answer for malformed callback, got: %+v", answer)
	}

	// Oversized callback (>64 bytes)
	uOver := telegram.Update{
		UpdateID: 2,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_over",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID: 51,
				Chat:      telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: strings.Repeat("x", 65),
		},
	}
	if err := env.router.HandleUpdate(ctx, uOver); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	answer = env.client.LastAnswer()
	if answer == nil || answer.QueryID != "cb_over" {
		t.Fatalf("expected callback answer for oversized callback, got: %+v", answer)
	}
}

func TestRouter_ForwardedButtonActor(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// Create a chore assigned to Alice (101)
	def, err := env.chores.CreateDefinition(ctx, 101, "Clean kitchen", "🧹", "", nil)
	if err != nil {
		t.Fatalf("failed to create chore def: %v", err)
	}
	dueAt := env.clock.now.Add(2 * time.Hour)
	occ, err := env.chores.ScheduleOccurrence(ctx, 101, def.ID, 101, dueAt)
	if err != nil {
		t.Fatalf("failed to schedule chore: %v", err)
	}

	// Bob (102) clicks Alice's Done button forwarded from Alice's notification!
	uBobDone := telegram.Update{
		UpdateID: 1,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_bob_click",
			From: telegram.User{ID: 102, FirstName: "Bob"}, // Bob is the actor!
			Message: &telegram.Message{
				MessageID: 60,
				Chat:      telegram.Chat{ID: 102, Type: "private"},
			},
			Data: fmt.Sprintf("ch:c:%d", occ.ID),
		},
	}

	if err := env.router.HandleUpdate(ctx, uBobDone); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Bob should be rejected because he is not the assignee!
	answer := env.client.LastAnswer()
	if answer == nil || !strings.Contains(answer.Text, "Only the assigned member") {
		t.Fatalf("expected not assignee rejection, got: %+v", answer)
	}

	// Occurrence should still be open!
	freshOcc, err := env.chores.GetOccurrence(ctx, occ.ID)
	if err != nil || freshOcc.Status != "open" {
		t.Fatalf("occurrence should still be open, got: %+v", freshOcc)
	}
}

func TestRouter_MenuGrantsNoAuthorization(t *testing.T) {
	// Verifies that advertised menu commands do not grant authorization.
	// Non-member /add is rejected with no flow started, and /list in unmapped topic
	// returns general.unmapped_topic, even without any command registration having run.
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// 1. Non-member sends /add in group
	uNonMember := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       100,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 555000111, FirstName: "NonMember"},
			Text:            "/add",
		},
	}
	sentBefore := len(env.client.sentMessages)
	if err := env.router.HandleUpdate(ctx, uNonMember); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.client.sentMessages) != sentBefore {
		t.Errorf("expected no messages sent to non-member, got %d", len(env.client.sentMessages)-sentBefore)
	}
	_, err = env.flow.Get(ctx, flow.Key{UserID: 555000111, ChatID: -1001234567890, ThreadID: 42}, nil)
	if !errors.Is(err, flow.ErrNoFlow) {
		t.Errorf("expected no flow created for non-member, got %v", err)
	}

	// 2. Member sends /list in an unmapped topic
	uUnmappedList := telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID:       101,
			MessageThreadID: 8888, // Unmapped topic
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/list",
		},
	}
	if err := env.router.HandleUpdate(ctx, uUnmappedList); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	expectedNotice := env.i18n.T("en", "general.unmapped_topic")
	if last == nil || last.Text != expectedNotice {
		t.Fatalf("expected unmapped topic notice %q for /list, got: %+v", expectedNotice, last)
	}
}

func TestFlow_EditFailureIgnored_FlowStillCompletes(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()

	item, err := env.shopping.CreateItem(ctx, 101, "Apples", "🍎")
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

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
		t.Fatalf("unexpected error on /add: %v", err)
	}
	menu := env.client.LastSent()
	if menu == nil || menu.Markup == nil || len(menu.Markup.InlineKeyboard) == 0 {
		t.Fatalf("expected menu message with buttons, got: %+v", menu)
	}

	// Inject error in EditMessageText to simulate Telegram API rejection / network issue
	editAttempted := false
	env.client.onEditMessageText = func(_ context.Context, _ int64, _ int64, _ string, _ *telegram.EditOptions) error {
		editAttempted = true
		return errors.New("telegram api error: message is not modified")
	}

	uPick := telegram.Update{
		UpdateID: 2,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_pick",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       menu.MessageID,
				MessageThreadID: 42,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            menu.Text,
			},
			Data: fmt.Sprintf("fl:sel:%d", item.ID),
		},
	}
	// The handler must succeed without returning the edit error
	if err := env.router.HandleUpdate(ctx, uPick); err != nil {
		t.Fatalf("expected flow to complete despite edit failure, got error: %v", err)
	}
	if !editAttempted {
		t.Fatalf("expected EditMessageText to be attempted")
	}

	// Verify the domain action completed and item is pending
	pending, err := env.shopping.ListPending(ctx)
	if err != nil || len(pending) != 1 || pending[0].Item.Name != "Apples" {
		t.Fatalf("expected item to be added to pending list, got: %+v (err: %v)", pending, err)
	}

	// Verify the flow was cleared
	flowKey := flowKeyFor(101, -1001234567890, 42)
	state, err := env.flow.Get(ctx, flowKey, nil)
	if err == nil && state != nil {
		t.Fatalf("expected flow to be cleared, got: %+v", state)
	}
}

func TestFlowOwnership_EnforcementAndExpiry(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()

	// 1. Alice has a Chores flow with a flow message
	choreFlowKey := flowKeyFor(101, -1001234567890, 51)
	if err := env.flow.Set(ctx, choreFlowKey, "chore_select_def", nil, 10*time.Minute); err != nil {
		t.Fatalf("flow.Set failed: %v", err)
	}
	choreMsgID := int64(700)
	if err := env.flow.SetMessageID(ctx, choreFlowKey, choreMsgID); err != nil {
		t.Fatalf("SetMessageID failed: %v", err)
	}

	editsBefore := len(env.client.EditedMessages())

	// Bob (102) presses Cancel ("fl:c") on Alice's Chores flow message
	uBobCancel := telegram.Update{
		UpdateID: 10,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_bob_cancel",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID:       choreMsgID,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            "Select a chore",
			},
			Data: "fl:c",
		},
	}
	if err := env.router.HandleUpdate(ctx, uBobCancel); err != nil {
		t.Fatalf("HandleUpdate failed: %v", err)
	}
	ans := env.client.LastAnswer()
	if ans == nil || ans.Text != env.i18n.T("en", "picker.ownership_notice") {
		t.Fatalf("expected ownership notice, got %+v", ans)
	}
	if len(env.client.EditedMessages()) != editsBefore {
		t.Fatalf("expected no edits/keyboard closing for non-owner, got edits")
	}

	// Verify Alice's Chores flow is still intact
	aliceChoreState, err := env.flow.Get(ctx, choreFlowKey, nil)
	if err != nil || aliceChoreState.Step != "chore_select_def" {
		t.Fatalf("expected Alice chores flow to remain intact, got %+v, err=%v", aliceChoreState, err)
	}

	// 2. Alice starts a Shopping flow with items
	item, err := env.shopping.CreateItem(ctx, 101, "Milk", "🥛")
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}
	uAliceAdd := telegram.Update{
		UpdateID: 11,
		Message: &telegram.Message{
			MessageID:       20,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAliceAdd); err != nil {
		t.Fatalf("/add failed: %v", err)
	}
	shopMsg := env.client.LastSent()
	if shopMsg == nil {
		t.Fatalf("expected shopping menu sent")
	}

	editsBeforeShop := len(env.client.EditedMessages())

	// Bob presses "Milk" on Alice's Shopping menu
	uBobShop := telegram.Update{
		UpdateID: 12,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_bob_shop",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID:       shopMsg.MessageID,
				MessageThreadID: 42,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            shopMsg.Text,
			},
			Data: fmt.Sprintf("fl:sel:%d", item.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uBobShop); err != nil {
		t.Fatalf("HandleUpdate failed: %v", err)
	}
	ansShop := env.client.LastAnswer()
	if ansShop == nil || ansShop.Text != env.i18n.T("en", "picker.ownership_notice") {
		t.Fatalf("expected ownership notice for Bob on shopping menu, got %+v", ansShop)
	}
	if len(env.client.EditedMessages()) != editsBeforeShop {
		t.Fatalf("expected no edits/keyboard closing on Alice's menu, got edits")
	}

	// Verify nothing was added to shopping list
	pending, err := env.shopping.ListPending(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("expected pending list to remain empty, got %+v", pending)
	}

	// Verify Alice's shopping flow is intact
	shopFlowKey := flowKeyFor(101, -1001234567890, 42)
	aliceShopState, err := env.flow.Get(ctx, shopFlowKey, nil)
	if err != nil || aliceShopState.Step != "shopping_select_items" {
		t.Fatalf("expected Alice shopping flow to remain intact, got %+v", aliceShopState)
	}

	// 3. Alice pressing after expiry
	env.clock.now = env.clock.now.Add(2 * time.Hour)

	uAliceExpired := telegram.Update{
		UpdateID: 13,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_alice_exp",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       choreMsgID,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            "Select a chore",
			},
			Data: "fl:c",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAliceExpired); err != nil {
		t.Fatalf("HandleUpdate failed on expired: %v", err)
	}
	ansExp := env.client.LastAnswer()
	if ansExp == nil || !ansExp.Alert || ansExp.Text != env.i18n.T("en", "general.flow_expired") {
		t.Fatalf("expected flow_expired alert for Alice after expiry, got %+v", ansExp)
	}
	lastEdited := env.client.LastEdited()
	if lastEdited == nil || lastEdited.MessageID != choreMsgID {
		t.Fatalf("expected keyboard on chore message %d to be closed after expiry, got %+v", choreMsgID, lastEdited)
	}
	if lastEdited.Markup == nil || len(lastEdited.Markup.InlineKeyboard) != 0 {
		t.Fatalf("expected empty keyboard markup, got %+v", lastEdited.Markup)
	}
}
