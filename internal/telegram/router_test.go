package telegram_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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
