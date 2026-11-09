package telegram_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/px1/home-ctl/internal/flow"
	"github.com/px1/home-ctl/internal/telegram"
)

const staleTestChatID = int64(-1001234567890)

func pressOnMessage(ctx context.Context, env *testEnv, updateID, userID, threadID, msgID int64, data string) error {
	return env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: updateID,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   fmt.Sprintf("cb_stale_%d", updateID),
			From: telegram.User{ID: userID, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       msgID,
				MessageThreadID: threadID,
				Chat:            telegram.Chat{ID: staleTestChatID, Type: "supergroup"},
				Text:            "old flow message",
			},
			Data: data,
		},
	})
}

func sendText(ctx context.Context, env *testEnv, updateID, userID, threadID int64, text string) error {
	return env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: updateID,
		Message: &telegram.Message{
			MessageID:       updateID + 1000,
			MessageThreadID: threadID,
			Chat:            telegram.Chat{ID: staleTestChatID, Type: "supergroup"},
			From:            &telegram.User{ID: userID, FirstName: "Alice"},
			Text:            text,
		},
	})
}

// flowStaleScenario describes one flow kind (chores or reminders) used by the stale-callback test.
type flowStaleScenario struct {
	name      string
	threadID  int64
	target    string // assignee/target callback
	titleA    string
	titleB    string
	date      string // callback for the first due date
	openCount func(env *testEnv) (int, error)
	startB    func(ctx context.Context, env *testEnv, base int64) error
}

func flowStaleChoreOpenCount(env *testEnv) (int, error) {
	res, err := env.chores.ListMyOpen(context.Background(), 101)
	if err != nil {
		return 0, err
	}
	return len(res.Occurrences), nil
}

func flowStaleReminderOpenCount(env *testEnv) (int, error) {
	res, err := env.reminders.ListMyOpen(context.Background(), 102)
	if err != nil {
		return 0, err
	}
	return len(res.Occurrences), nil
}

func flowStaleStartChoreFlowB(ctx context.Context, env *testEnv, base int64) error {
	// Catalog now has chore A: /add shows the selection menu, then "add new" and a name.
	if err := sendText(ctx, env, base, 101, 51, "/add"); err != nil {
		return err
	}
	menu := env.client.LastSent()
	if err := pressOnMessage(ctx, env, base+1, 101, 51, menu.MessageID, "fl:new"); err != nil {
		return err
	}
	return sendText(ctx, env, base+2, 101, 51, "Second chore B")
}

func flowStaleStartReminderFlowB(ctx context.Context, env *testEnv, base int64) error {
	if err := sendText(ctx, env, base, 101, 63, "/add"); err != nil {
		return err
	}
	return sendText(ctx, env, base+1, 101, 63, "Second reminder B")
}

// A callback may only mutate the flow shown in the message that produced it. After flow A is confirmed,
// pressing its old buttons takes the stale path and must not touch the confirmed result or the newer flow B
// the same user started in the same chat and topic.
func TestFlowStaleCallback_ConfirmedFlowCannotDriveNewerFlow(t *testing.T) {
	scenarios := []flowStaleScenario{
		{
			name:      "chores",
			threadID:  51,
			target:    "fl:usr:101",
			titleA:    "Stale chore A",
			titleB:    "Second chore B",
			date:      "fl:d:2026-10-14",
			openCount: flowStaleChoreOpenCount,
			startB:    flowStaleStartChoreFlowB,
		},
		{
			name:      "reminders",
			threadID:  63,
			target:    "fl:usr:102",
			titleA:    "Stale reminder A",
			titleB:    "Second reminder B",
			date:      "fl:d:2026-10-09",
			openCount: flowStaleReminderOpenCount,
			startB:    flowStaleStartReminderFlowB,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			flowStaleRunScenario(t, sc)
		})
	}
}

// flowStaleRunScenario runs flow A to confirmation, then checks that its old callbacks cannot touch flow B.
func flowStaleRunScenario(t *testing.T, sc flowStaleScenario) {
	t.Helper()
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()
	key := flowKeyFor(101, staleTestChatID, sc.threadID)

	// Flow A: create to confirmation.
	msgA := flowStaleCreateFlowA(ctx, t, env, sc)
	flowStaleAssertConfirmed(ctx, t, env, sc, key, msgA)

	wantExpired := env.i18n.T("en", "general.flow_expired")
	flowStaleAssertRepeatedConfirm(ctx, t, env, sc, msgA, wantExpired)

	// 4. Flow B starts in the same chat and topic.
	stateB, msgB := flowStaleStartFlowB(ctx, t, env, sc, key, msgA)
	editsBefore := len(env.client.EditedMessages())
	sentBefore := len(env.client.SentMessages())

	// 5. Old callbacks from message A must not touch flow B.
	flowStaleAssertStalePresses(ctx, t, env, sc, msgA, wantExpired)
	flowStaleAssertFlowBUntouched(ctx, t, env, sc, key, stateB, msgB, editsBefore, sentBefore)
}

// flowStaleCreateFlowA drives flow A from /add to confirmation and returns the id of its message.
func flowStaleCreateFlowA(ctx context.Context, t *testing.T, env *testEnv, sc flowStaleScenario) int64 {
	t.Helper()
	if err := sendText(ctx, env, 1, 101, sc.threadID, "/add"); err != nil {
		t.Fatalf("/add failed: %v", err)
	}
	if err := sendText(ctx, env, 2, 101, sc.threadID, sc.titleA); err != nil {
		t.Fatalf("title failed: %v", err)
	}
	// The typed title re-anchors flow A below the text: its buttons live on the new message.
	msgA := env.client.LastSent().MessageID
	for i, data := range []string{sc.target, "fl:rt:once", sc.date, "fl:h:10", "fl:mi:30"} {
		if err := pressOnMessage(ctx, env, int64(10+i), 101, sc.threadID, msgA, data); err != nil {
			t.Fatalf("step %q failed: %v", data, err)
		}
	}
	return msgA
}

// flowStaleAssertConfirmed checks that confirming flow A closes its keyboard, clears its state and creates one occurrence.
func flowStaleAssertConfirmed(ctx context.Context, t *testing.T, env *testEnv, sc flowStaleScenario, key flow.Key, msgA int64) {
	t.Helper()
	// 1. Confirm closes the keyboard of flow A's message.
	if err := pressOnMessage(ctx, env, 20, 101, sc.threadID, msgA, "fl:ok"); err != nil {
		t.Fatalf("confirm failed: %v", err)
	}
	edit := env.client.LastEdited()
	if edit == nil || edit.MessageID != msgA || len(edit.Markup.InlineKeyboard) != 0 {
		t.Fatalf("expected closed keyboard on message %d after confirm, got %+v", msgA, edit)
	}

	// 2. The flow state is removed.
	if _, err := env.flow.Get(ctx, key, nil); !errors.Is(err, flow.ErrNoFlow) {
		t.Fatalf("expected no flow after confirm, got err=%v", err)
	}
	if _, err := env.flow.GetByMessage(ctx, staleTestChatID, sc.threadID, msgA, nil); !errors.Is(err, flow.ErrNoFlow) {
		t.Fatalf("expected no flow for message %d after confirm, got err=%v", msgA, err)
	}
	created, err := sc.openCount(env)
	if err != nil || created != 1 {
		t.Fatalf("expected 1 open occurrence after confirm, got %d (err=%v)", created, err)
	}
}

// flowStaleAssertRepeatedConfirm checks that pressing the old Confirm again alerts as expired and creates nothing.
func flowStaleAssertRepeatedConfirm(ctx context.Context, t *testing.T, env *testEnv, sc flowStaleScenario, msgA int64, wantExpired string) {
	t.Helper()
	// 3. Pressing the old Confirm again takes the expired path and creates nothing.
	sentBefore := len(env.client.SentMessages())
	if err := pressOnMessage(ctx, env, 21, 101, sc.threadID, msgA, "fl:ok"); err != nil {
		t.Fatalf("repeated confirm failed: %v", err)
	}
	ans := env.client.LastAnswer()
	if ans == nil || ans.Text != wantExpired || !ans.Alert {
		t.Fatalf("expected expired alert %q, got %+v", wantExpired, ans)
	}
	if n, _ := sc.openCount(env); n != 1 {
		t.Fatalf("expected still 1 open occurrence after repeated confirm, got %d", n)
	}
	if len(env.client.SentMessages()) != sentBefore {
		t.Fatalf("repeated confirm must not send messages")
	}
}

// flowStaleStartFlowB starts flow B in the same chat and topic and returns its state and message id.
func flowStaleStartFlowB(ctx context.Context, t *testing.T, env *testEnv, sc flowStaleScenario, key flow.Key, msgA int64) (*flow.State, int64) {
	t.Helper()
	if err := sc.startB(ctx, env, 30); err != nil {
		t.Fatalf("starting flow B failed: %v", err)
	}
	stateB, err := env.flow.Get(ctx, key, nil)
	if err != nil || stateB == nil {
		t.Fatalf("expected active flow B, got err=%v", err)
	}
	msgB := stateB.MessageID
	if msgB == 0 || msgB == msgA {
		t.Fatalf("flow B must have its own message id, got A=%d B=%d", msgA, msgB)
	}
	return stateB, msgB
}

// flowStaleAssertStalePresses checks that every old flow A button answers with the expired alert.
func flowStaleAssertStalePresses(ctx context.Context, t *testing.T, env *testEnv, sc flowStaleScenario, msgA int64, wantExpired string) {
	t.Helper()
	for i, data := range []string{"fl:ok", "fl:c", sc.target, "fl:h:11", "fl:mi:45", "fl:d:2026-10-20"} {
		if err := pressOnMessage(ctx, env, int64(40+i), 101, sc.threadID, msgA, data); err != nil {
			t.Fatalf("stale press %q failed: %v", data, err)
		}
		ans := env.client.LastAnswer()
		if ans == nil || ans.Text != wantExpired {
			t.Fatalf("stale press %q: expected expired alert, got %+v", data, ans)
		}
	}
}

// flowStaleAssertFlowBUntouched checks that flow B is unchanged and that the stale presses sent, edited and created nothing.
func flowStaleAssertFlowBUntouched(ctx context.Context, t *testing.T, env *testEnv, sc flowStaleScenario, key flow.Key, stateB *flow.State, msgB int64, editsBefore, sentBefore int) {
	t.Helper()
	after, err := env.flow.Get(ctx, key, nil)
	if err != nil || after == nil {
		t.Fatalf("flow B must still exist after stale presses, got err=%v", err)
	}
	if after.Step != stateB.Step || after.MessageID != msgB || !bytes.Equal(after.Payload, stateB.Payload) {
		t.Fatalf("flow B changed: before=%+v after=%+v", stateB, after)
	}
	for _, em := range env.client.EditedMessages()[editsBefore:] {
		if em.MessageID == msgB {
			t.Fatalf("stale presses must not edit flow B's message, got edit %+v", em)
		}
	}
	if len(env.client.SentMessages()) != sentBefore {
		t.Fatalf("stale presses must not send messages")
	}
	if n, _ := sc.openCount(env); n != 1 {
		t.Fatalf("stale presses must not create occurrences, got %d", n)
	}
}
