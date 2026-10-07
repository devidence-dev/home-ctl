package telegram_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/telegram"
)

func TestChores_AddFlow_ConfirmAndCancel(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// 1. Alice starts /add in Chores topic (51)
	uAdd := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAdd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "name for the new chore") {
		t.Fatalf("expected chore name prompt, got: %+v", last)
	}

	// 2. Alice types "Sweep the floor"
	uName := telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID:       11,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Sweep the floor",
		},
	}
	if err := env.router.HandleUpdate(ctx, uName); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "assigned to") {
		t.Fatalf("expected assignee prompt, got: %+v", last)
	}

	// 3. Bob sends text in the Chores topic while Alice's flow is waiting -> Bob's message ignored!
	uBobText := telegram.Update{
		UpdateID: 3,
		Message: &telegram.Message{
			MessageID:       12,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 102, FirstName: "Bob"},
			Text:            "Hello everyone",
		},
	}
	_ = env.router.HandleUpdate(ctx, uBobText)

	// 4. Alice selects Alice (101) as assignee
	uAssign := telegram.Update{
		UpdateID: 4,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_assign",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       13,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:usr:101",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAssign); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "due date/time") {
		t.Fatalf("expected due date prompt, got: %+v", last)
	}

	// 5. Alice selects "Choose date/time" custom option
	uCustom := telegram.Update{
		UpdateID: 5,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_custom",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       14,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:due:custom",
		},
	}
	_ = env.router.HandleUpdate(ctx, uCustom)

	// 6. Alice types invalid date "banana" -> rejected with format guidance
	uBanana := telegram.Update{
		UpdateID: 6,
		Message: &telegram.Message{
			MessageID:       15,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "banana",
		},
	}
	_ = env.router.HandleUpdate(ctx, uBanana)
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "format") {
		t.Fatalf("expected format error, got: %+v", last)
	}

	// 7. Alice types past date "2020-01-01 10:00" -> rejected
	uPast := telegram.Update{
		UpdateID: 7,
		Message: &telegram.Message{
			MessageID:       16,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "2020-01-01 10:00",
		},
	}
	_ = env.router.HandleUpdate(ctx, uPast)
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "past") {
		t.Fatalf("expected past date error, got: %+v", last)
	}

	// 8. Alice types valid future date "2026-10-10 10:00"
	uValidDate := telegram.Update{
		UpdateID: 8,
		Message: &telegram.Message{
			MessageID:       17,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "2026-10-10 10:00",
		},
	}
	if err := env.router.HandleUpdate(ctx, uValidDate); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "recurrence") {
		t.Fatalf("expected recurrence prompt, got: %+v", last)
	}

	// 9. Alice selects "Weekly"
	uWeekly := telegram.Update{
		UpdateID: 9,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_weekly",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       18,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:rec:weekly",
		},
	}
	_ = env.router.HandleUpdate(ctx, uWeekly)
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "anchor") {
		t.Fatalf("expected anchor prompt, got: %+v", last)
	}

	// 10. Alice selects "Scheduled" anchor
	uScheduled := telegram.Update{
		UpdateID: 10,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_anchor",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       19,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:anc:scheduled",
		},
	}
	_ = env.router.HandleUpdate(ctx, uScheduled)
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "Confirm") {
		t.Fatalf("expected confirm prompt, got: %+v", last)
	}

	// Before confirming: verify NO chore occurrences exist in database!
	resBefore, err := env.chores.ListMyOpen(ctx, 101)
	if err != nil || len(resBefore.Occurrences) != 0 {
		t.Fatalf("no occurrences should exist before confirm, got: %+v", resBefore)
	}

	// 11. Alice confirms ("fl:ok")
	uConfirm := telegram.Update{
		UpdateID: 11,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_confirm",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       20,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:ok",
		},
	}
	if err := env.router.HandleUpdate(ctx, uConfirm); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "scheduled") {
		t.Fatalf("expected scheduled confirmation, got: %+v", last)
	}

	// After confirm: verify occurrence exists!
	resAfter, err := env.chores.ListMyOpen(ctx, 101)
	if err != nil || len(resAfter.Occurrences) != 1 {
		t.Fatalf("expected 1 occurrence, got: %+v", resAfter)
	}
	occ := resAfter.Occurrences[0]
	if occ.AssigneeID != 101 || occ.Status != "open" {
		t.Fatalf("unexpected occurrence: %+v", occ)
	}
}

func TestChores_AddFlow_CancelLeavesNoData(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// 1. /add in chores
	uAdd := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	_ = env.router.HandleUpdate(ctx, uAdd)

	// 2. Types name "Dust shelves"
	uName := telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID:       11,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Dust shelves",
		},
	}
	_ = env.router.HandleUpdate(ctx, uName)

	// 3. Alice presses Cancel at assignee step ("fl:c")
	uCancel := telegram.Update{
		UpdateID: 3,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_cancel",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       12,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:c",
		},
	}
	if err := env.router.HandleUpdate(ctx, uCancel); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "cancelled") {
		t.Fatalf("expected cancelled message, got: %+v", last)
	}

	// Verify no chore definition and no occurrences exist
	defs, err := env.chores.ListDefinitions(ctx)
	if err != nil || len(defs) != 0 {
		t.Fatalf("expected 0 definitions, got: %+v", defs)
	}
}

func TestChores_AddFlow_ExistingDefinition(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// Create definition beforehand
	def, err := env.chores.CreateDefinition(ctx, 101, "Mow lawn", "🌱", "", nil)
	if err != nil {
		t.Fatalf("failed to create def: %v", err)
	}

	// 1. /add in Chores topic -> lists existing defs
	uAdd := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAdd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2. Select Mow lawn ("fl:sel:<def.ID>")
	uSel := telegram.Update{
		UpdateID: 2,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_sel_lawn",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       11,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: fmt.Sprintf("fl:sel:%d", def.ID),
		},
	}
	_ = env.router.HandleUpdate(ctx, uSel)

	// 3. Assign to Bob (102)
	uAssign := telegram.Update{
		UpdateID: 3,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_assign_bob",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       12,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:usr:102",
		},
	}
	_ = env.router.HandleUpdate(ctx, uAssign)

	// 4. Due date preset "today_18"
	uDue := telegram.Update{
		UpdateID: 4,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_due_today",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       13,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:due:today_18",
		},
	}
	_ = env.router.HandleUpdate(ctx, uDue)

	// 5. Recurrence "daily"
	uDaily := telegram.Update{
		UpdateID: 5,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_rec_daily",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       14,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:rec:daily",
		},
	}
	_ = env.router.HandleUpdate(ctx, uDaily)

	// 6. Anchor "completion"
	uComp := telegram.Update{
		UpdateID: 6,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_anc_comp",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       15,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:anc:completion",
		},
	}
	_ = env.router.HandleUpdate(ctx, uComp)

	// 7. Confirm
	uConfirm := telegram.Update{
		UpdateID: 7,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_ok_lawn",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       16,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:ok",
		},
	}
	if err := env.router.HandleUpdate(ctx, uConfirm); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify occurrence exists for Bob
	res, err := env.chores.ListMyOpen(ctx, 102)
	if err != nil || len(res.Occurrences) != 1 {
		t.Fatalf("expected 1 chore for Bob, got: %+v", res)
	}
}

func TestFlow_Expired(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// Alice starts /add in chores
	uAdd := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	_ = env.router.HandleUpdate(ctx, uAdd)

	// Advance time past TTL (35 minutes)
	env.clock.now = env.clock.now.Add(35 * time.Minute)

	// Alice clicks a button on expired flow
	uClick := telegram.Update{
		UpdateID: 2,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_expired",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       11,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:new",
		},
	}
	_ = env.router.HandleUpdate(ctx, uClick)
	ans := env.client.LastAnswer()
	if ans == nil || !strings.Contains(ans.Text, "expired") {
		t.Fatalf("expected expired flow alert, got: %+v", ans)
	}
}

func TestChoresFlow_KeyboardsClosedOnAssigneePick_Cancel_AndConfirm(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()

	// 1. Assignee pick closes its step message and advances the flow
	uAdd := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAdd); err != nil {
		t.Fatalf("unexpected error on /add: %v", err)
	}

	uName := telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID:       11,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Take out trash",
		},
	}
	if err := env.router.HandleUpdate(ctx, uName); err != nil {
		t.Fatalf("unexpected error on chore name: %v", err)
	}
	assigneePrompt := env.client.LastSent()
	if assigneePrompt == nil || assigneePrompt.Markup == nil || len(assigneePrompt.Markup.InlineKeyboard) == 0 {
		t.Fatalf("expected assignee prompt with buttons, got: %+v", assigneePrompt)
	}

	// Alice selects Bob (102) as assignee
	uPickAssignee := telegram.Update{
		UpdateID: 3,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_assign_bob",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       assigneePrompt.MessageID,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            assigneePrompt.Text,
			},
			Data: "fl:usr:102",
		},
	}
	if err := env.router.HandleUpdate(ctx, uPickAssignee); err != nil {
		t.Fatalf("unexpected error on assignee pick: %v", err)
	}
	lastEdited := env.client.LastEdited()
	if lastEdited == nil || lastEdited.MessageID != assigneePrompt.MessageID {
		t.Fatalf("expected assignee prompt message %d to be edited, got: %+v", assigneePrompt.MessageID, lastEdited)
	}
	if lastEdited.Markup == nil || len(lastEdited.Markup.InlineKeyboard) != 0 {
		t.Fatalf("expected empty keyboard on assignee step after pick, got: %+v", lastEdited.Markup)
	}
	if lastEdited.Text != assigneePrompt.Text {
		t.Fatalf("expected assignee prompt text %q preserved, got: %q", assigneePrompt.Text, lastEdited.Text)
	}

	// Continue to confirm: select due preset and recurrence
	duePrompt := env.client.LastSent()
	uDue := telegram.Update{
		UpdateID: 4,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_due",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       duePrompt.MessageID,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            duePrompt.Text,
			},
			Data: "fl:due:tomorrow_18",
		},
	}
	if err := env.router.HandleUpdate(ctx, uDue); err != nil {
		t.Fatalf("unexpected error on due preset: %v", err)
	}

	recPrompt := env.client.LastSent()
	uRec := telegram.Update{
		UpdateID: 5,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_rec",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       recPrompt.MessageID,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            recPrompt.Text,
			},
			Data: "fl:rec:none",
		},
	}
	if err := env.router.HandleUpdate(ctx, uRec); err != nil {
		t.Fatalf("unexpected error on recurrence: %v", err)
	}

	summaryMsg := env.client.LastSent()
	if summaryMsg == nil || !strings.Contains(summaryMsg.Text, "Confirm") {
		t.Fatalf("expected confirm summary message, got: %+v", summaryMsg)
	}

	// Confirm press closes summary message and creates chore once
	uConfirm := telegram.Update{
		UpdateID: 6,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_confirm",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       summaryMsg.MessageID,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            summaryMsg.Text,
			},
			Data: "fl:ok",
		},
	}
	if err := env.router.HandleUpdate(ctx, uConfirm); err != nil {
		t.Fatalf("unexpected error on confirm: %v", err)
	}
	lastEdited = env.client.LastEdited()
	if lastEdited == nil || lastEdited.MessageID != summaryMsg.MessageID {
		t.Fatalf("expected summary message %d to be edited on confirm, got: %+v", summaryMsg.MessageID, lastEdited)
	}
	if lastEdited.Markup == nil || len(lastEdited.Markup.InlineKeyboard) != 0 {
		t.Fatalf("expected empty keyboard on summary message after confirm, got: %+v", lastEdited.Markup)
	}
	if lastEdited.Text != summaryMsg.Text {
		t.Fatalf("expected summary text preserved, got: %q", lastEdited.Text)
	}

	// Verify chore created once
	resBob, err := env.chores.ListMyOpen(ctx, 102)
	if err != nil || len(resBob.Occurrences) != 1 {
		t.Fatalf("expected exactly 1 chore for Bob, got: %+v", resBob)
	}
	if resBob.Occurrences[0].Definition.Name != "Take out trash" {
		t.Fatalf("expected chore name 'Take out trash', got: %s", resBob.Occurrences[0].Definition.Name)
	}

	// 2. Cancel closes its step message and clears flow
	uAdd2 := telegram.Update{
		UpdateID: 7,
		Message: &telegram.Message{
			MessageID:       20,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAdd2); err != nil {
		t.Fatalf("unexpected error on second /add: %v", err)
	}

	uName2 := telegram.Update{
		UpdateID: 8,
		Message: &telegram.Message{
			MessageID:       21,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Mop floor",
		},
	}
	if err := env.router.HandleUpdate(ctx, uName2); err != nil {
		t.Fatalf("unexpected error on second chore name: %v", err)
	}
	assigneePrompt2 := env.client.LastSent()

	uCancel := telegram.Update{
		UpdateID: 9,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_cancel_chores",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       assigneePrompt2.MessageID,
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            assigneePrompt2.Text,
			},
			Data: "fl:c",
		},
	}
	if err := env.router.HandleUpdate(ctx, uCancel); err != nil {
		t.Fatalf("unexpected error on cancel: %v", err)
	}
	lastEdited = env.client.LastEdited()
	if lastEdited == nil || lastEdited.MessageID != assigneePrompt2.MessageID {
		t.Fatalf("expected assignee step message %d to be edited on cancel, got: %+v", assigneePrompt2.MessageID, lastEdited)
	}
	if lastEdited.Markup == nil || len(lastEdited.Markup.InlineKeyboard) != 0 {
		t.Fatalf("expected empty keyboard after cancel, got: %+v", lastEdited.Markup)
	}
	if lastEdited.Text != assigneePrompt2.Text {
		t.Fatalf("expected assignee prompt text preserved on cancel, got: %q", lastEdited.Text)
	}

	// Verify flow is cleared and no second chore was created
	flowKey := flowKeyFor(101, -1001234567890, 51)
	state, err := env.flow.Get(ctx, flowKey, nil)
	if err == nil && state != nil {
		t.Fatalf("expected flow to be cleared after cancel, got: %+v", state)
	}
	resBob, _ = env.chores.ListMyOpen(ctx, 102)
	if len(resBob.Occurrences) != 1 {
		t.Fatalf("expected still exactly 1 chore for Bob, got: %d", len(resBob.Occurrences))
	}
}
