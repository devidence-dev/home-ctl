package telegram_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/telegram"
)

// 7.1: Type date, Other for interval and Other for minute as typed steps that continue by editing the flow message;
// verify with tests for valid answers, invalid answers staying on the step with guidance, and another user's text being ignored.
func TestTypedSteps_Interval_Other(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	// 1. Alice starts chore /add, names it, assigns Alice, picks Weekly
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	initialMsg := env.client.LastSent()
	flowMsgID := initialMsg.MessageID

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 11, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "Water plants",
		},
	})
	_ = pressChoreCallback(ctx, env, 3, 101, "fl:usr:101")
	_ = pressChoreCallback(ctx, env, 4, 101, "fl:rt:weekly")

	// 2. Alice clicks "Other" for interval (fl:iv:o)
	_ = pressChoreCallback(ctx, env, 5, 101, "fl:iv:o")
	lastEdit := env.client.LastEdited()
	if lastEdit == nil || !strings.Contains(lastEdit.Text, "interval") {
		t.Fatalf("expected prompt for typed interval, got: %+v", lastEdit)
	}

	// 3. Bob sends text in the topic while Alice is waiting -> ignored!
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 6,
		Message: &telegram.Message{
			MessageID: 12, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 102, FirstName: "Bob"}, Text: "Hey Alice",
		},
	})
	state, err := env.flow.Get(ctx, flowKeyFor(101, -1001234567890, 51), nil)
	if err != nil || state == nil || state.Step != "chore_typed_interval" {
		t.Fatalf("expected Alice flow to remain on chore_typed_interval after Bob text, got %+v", state)
	}

	// 4. Alice sends invalid interval "0" -> stays on step with guidance
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 7,
		Message: &telegram.Message{
			MessageID: 13, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "0",
		},
	})
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || !strings.Contains(lastEdit.Text, "1 to 99") {
		t.Fatalf("expected guidance for invalid interval, got: %+v", lastEdit)
	}
	state, _ = env.flow.Get(ctx, flowKeyFor(101, -1001234567890, 51), nil)
	if state.Step != "chore_typed_interval" {
		t.Fatalf("expected flow to stay on chore_typed_interval, got %s", state.Step)
	}

	// Alice sends invalid interval "100" -> stays on step
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 8,
		Message: &telegram.Message{
			MessageID: 14, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "100",
		},
	})
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || !strings.Contains(lastEdit.Text, "1 to 99") {
		t.Fatalf("expected guidance for 100, got: %+v", lastEdit)
	}

	// Alice sends invalid interval "abc" -> stays on step
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 9,
		Message: &telegram.Message{
			MessageID: 15, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "abc",
		},
	})
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || !strings.Contains(lastEdit.Text, "1 to 99") {
		t.Fatalf("expected guidance for abc, got: %+v", lastEdit)
	}

	// 5. Alice sends valid interval "15" -> continues to date step, editing flowMsgID
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 10,
		Message: &telegram.Message{
			MessageID: 16, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "15",
		},
	})
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "first due date") {
		t.Fatalf("expected date prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}
	state, _ = env.flow.Get(ctx, flowKeyFor(101, -1001234567890, 51), nil)
	if state.Step != "chore_date" {
		t.Fatalf("expected flow to advance to chore_date, got %s", state.Step)
	}
}

func TestTypedSteps_Date_TypeDate(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	initialMsg := env.client.LastSent()
	flowMsgID := initialMsg.MessageID

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 11, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "Wash car",
		},
	})
	_ = pressChoreCallback(ctx, env, 3, 101, "fl:usr:101")
	_ = pressChoreCallback(ctx, env, 4, 101, "fl:rt:once")

	// Alice clicks "Type date" button
	_ = pressChoreCallback(ctx, env, 5, 101, "fl:d:type")
	lastEdit := env.client.LastEdited()
	if lastEdit == nil || !strings.Contains(lastEdit.Text, "YYYY-MM-DD") {
		t.Fatalf("expected prompt for typing date, got: %+v", lastEdit)
	}

	// Bob sends text in topic -> ignored!
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 6,
		Message: &telegram.Message{
			MessageID: 12, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 102, FirstName: "Bob"}, Text: "tomorrow",
		},
	})
	state, _ := env.flow.Get(ctx, flowKeyFor(101, -1001234567890, 51), nil)
	if state.Step != "chore_typed_date" {
		t.Fatalf("expected step to remain chore_typed_date, got %s", state.Step)
	}

	// Alice sends unparsable date "next friday" -> guidance, stays on step
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 7,
		Message: &telegram.Message{
			MessageID: 13, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "next friday",
		},
	})
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || !strings.Contains(lastEdit.Text, "format") {
		t.Fatalf("expected format guidance, got: %+v", lastEdit)
	}
	state, _ = env.flow.Get(ctx, flowKeyFor(101, -1001234567890, 51), nil)
	if state.Step != "chore_typed_date" {
		t.Fatalf("expected step to remain chore_typed_date, got %s", state.Step)
	}

	// Alice sends valid date only "2026-10-10" -> moves to hour step!
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 8,
		Message: &telegram.Message{
			MessageID: 14, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "2026-10-10",
		},
	})
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "hour") {
		t.Fatalf("expected hour prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}
	state, _ = env.flow.Get(ctx, flowKeyFor(101, -1001234567890, 51), nil)
	if state.Step != "chore_hour" {
		t.Fatalf("expected step chore_hour, got %s", state.Step)
	}
}

func TestTypedSteps_DateTime_AdvancesToConfirm(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	initialMsg := env.client.LastSent()
	flowMsgID := initialMsg.MessageID

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 11, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "Wash car",
		},
	})
	_ = pressChoreCallback(ctx, env, 3, 101, "fl:usr:101")
	_ = pressChoreCallback(ctx, env, 4, 101, "fl:rt:once")
	_ = pressChoreCallback(ctx, env, 5, 101, "fl:d:type")

	// Alice sends date AND time "2026-10-10 10:00" -> skips hour and minute, directly to confirm!
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 6,
		Message: &telegram.Message{
			MessageID: 12, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "2026-10-10 10:00",
		},
	})
	lastEdit := env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "Confirm") {
		t.Fatalf("expected confirm prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	_ = pressChoreCallback(ctx, env, 7, 101, "fl:ok")
	res, err := env.chores.ListMyOpen(ctx, 101)
	if err != nil || len(res.Occurrences) != 1 {
		t.Fatalf("expected 1 occurrence, got: %+v", res)
	}
	want := time.Date(2026, 10, 10, 10, 0, 0, 0, env.clock.loc).UTC()
	if !res.Occurrences[0].DueAt.Equal(want) {
		t.Fatalf("expected due %v, got %v", want, res.Occurrences[0].DueAt)
	}
}

func TestTypedSteps_Minute_Other(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	initialMsg := env.client.LastSent()
	flowMsgID := initialMsg.MessageID

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 11, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "Cook dinner",
		},
	})
	_ = pressChoreCallback(ctx, env, 3, 101, "fl:usr:101")
	_ = pressChoreCallback(ctx, env, 4, 101, "fl:rt:once")
	_ = pressChoreCallback(ctx, env, 5, 101, "fl:d:2026-10-10")
	_ = pressChoreCallback(ctx, env, 6, 101, "fl:h:10")

	// Alice clicks Other on minute step (fl:mi:o)
	_ = pressChoreCallback(ctx, env, 7, 101, "fl:mi:o")
	lastEdit := env.client.LastEdited()
	if lastEdit == nil || !strings.Contains(lastEdit.Text, "minute") {
		t.Fatalf("expected prompt for minute, got: %+v", lastEdit)
	}

	// Bob sends text in topic -> ignored!
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 8,
		Message: &telegram.Message{
			MessageID: 12, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 102, FirstName: "Bob"}, Text: "15",
		},
	})
	state, _ := env.flow.Get(ctx, flowKeyFor(101, -1001234567890, 51), nil)
	if state.Step != "chore_typed_minute" {
		t.Fatalf("expected step chore_typed_minute, got %s", state.Step)
	}

	// Alice sends invalid minute "75" -> stays on step with guidance
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 9,
		Message: &telegram.Message{
			MessageID: 13, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "75",
		},
	})
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || !strings.Contains(lastEdit.Text, "00 to 59") {
		t.Fatalf("expected 00 to 59 guidance, got: %+v", lastEdit)
	}
	state, _ = env.flow.Get(ctx, flowKeyFor(101, -1001234567890, 51), nil)
	if state.Step != "chore_typed_minute" {
		t.Fatalf("expected step chore_typed_minute, got %s", state.Step)
	}

	// Alice sends valid minute "07" -> advances to confirm on flowMsgID
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 10,
		Message: &telegram.Message{
			MessageID: 14, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "07",
		},
	})
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "Confirm") {
		t.Fatalf("expected confirm prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	_ = pressChoreCallback(ctx, env, 11, 101, "fl:ok")
	res, _ := env.chores.ListMyOpen(ctx, 101)
	if len(res.Occurrences) != 1 {
		t.Fatalf("expected 1 occurrence, got %d", len(res.Occurrences))
	}
	want := time.Date(2026, 10, 10, 10, 7, 0, 0, env.clock.loc).UTC()
	if !res.Occurrences[0].DueAt.Equal(want) {
		t.Fatalf("expected due %v, got %v", want, res.Occurrences[0].DueAt)
	}
}
