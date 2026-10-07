package main_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/chores"
	"github.com/px1/home-ctl/internal/config"
	"github.com/px1/home-ctl/internal/flow"
	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/i18n"
	"github.com/px1/home-ctl/internal/reminders"
	"github.com/px1/home-ctl/internal/scheduler"
	"github.com/px1/home-ctl/internal/shopping"
	"github.com/px1/home-ctl/internal/storage/sqlite"
	"github.com/px1/home-ctl/internal/telegram"
)

type sentMsg struct {
	ChatID   int64
	ThreadID int64
	Text     string
	Markup   *telegram.InlineKeyboardMarkup
}

type editedMsg struct {
	ChatID    int64
	MessageID int64
	Text      string
	Markup    *telegram.InlineKeyboardMarkup
}

type cbAns struct {
	ID    string
	Text  string
	Alert bool
}

type fakeTG struct {
	mu        sync.Mutex
	nextID    int64
	sent      []sentMsg
	edited    []editedMsg
	cbAnswers []cbAns
}

func newFakeTG() *fakeTG {
	return &fakeTG{nextID: 100}
}

func (f *fakeTG) SendMessage(_ context.Context, chatID int64, text string, opts *telegram.SendOptions) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	var threadID int64
	var markup *telegram.InlineKeyboardMarkup
	if opts != nil {
		threadID = opts.ThreadID
		markup = opts.ReplyMarkup
	}
	f.sent = append(f.sent, sentMsg{
		ChatID:   chatID,
		ThreadID: threadID,
		Text:     text,
		Markup:   markup,
	})
	return f.nextID, nil
}

func (f *fakeTG) EditMessageText(_ context.Context, chatID int64, messageID int64, text string, opts *telegram.EditOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var markup *telegram.InlineKeyboardMarkup
	if opts != nil {
		markup = opts.ReplyMarkup
	}
	f.edited = append(f.edited, editedMsg{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
		Markup:    markup,
	})
	return nil
}

func (f *fakeTG) AnswerCallbackQuery(_ context.Context, callbackQueryID string, text string, showAlert bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cbAnswers = append(f.cbAnswers, cbAns{
		ID:    callbackQueryID,
		Text:  text,
		Alert: showAlert,
	})
	return nil
}

func (f *fakeTG) GetUpdates(_ context.Context, _ int64, _ int) ([]telegram.Update, error) {
	return nil, nil
}

func (f *fakeTG) SetMyCommands(_ context.Context, _ telegram.BotCommandScope, _ string, _ []telegram.BotCommand) error {
	return nil
}

func (f *fakeTG) LastSent() *sentMsg {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 {
		return nil
	}
	cp := f.sent[len(f.sent)-1]
	return &cp
}

func (f *fakeTG) LastAnswer() *cbAns {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.cbAnswers) == 0 {
		return nil
	}
	cp := f.cbAnswers[len(f.cbAnswers)-1]
	return &cp
}

type simClock struct {
	mu  sync.Mutex
	now time.Time
	loc *time.Location
}

func (c *simClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *simClock) Location() *time.Location {
	return c.loc
}

func (c *simClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *simClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

func TestEndToEnd_WorkflowsWithRestart(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "e2e_homectl.db")

	loc, err := time.LoadLocation("America/Guayaquil")
	if err != nil {
		t.Fatalf("failed to load timezone: %v", err)
	}

	clock := &simClock{
		now: time.Date(2026, 10, 5, 8, 0, 0, 0, loc),
		loc: loc,
	}

	client := newFakeTG()
	localesDir := filepath.Clean("../../locales")
	bundle, err := i18n.NewBundle(localesDir, "en", i18n.DefaultRequiredKeys, nil)
	if err != nil {
		t.Fatalf("failed to load i18n: %v", err)
	}

	users := []config.UserConfig{
		{ID: 101, Name: "Alice", Username: "alice", Locale: "en"},
		{ID: 102, Name: "Bob", Username: "bob", Locale: "es"},
	}

	const (
		chatID           int64 = -1001234567890
		shoppingTopicID  int64 = 42
		choresTopicID    int64 = 51
		remindersTopicID int64 = 63
	)

	// Step 1: Initialize Database
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	ctx := context.Background()

	// Helper to build services from active db
	buildServices := func() (*household.Service, *shopping.Service, *chores.Service, *reminders.Service, *scheduler.Scheduler, *telegram.Router) {
		hhStore := sqlite.NewHouseholdStore(db)
		shStore := sqlite.NewShoppingStore(db)
		chStore := sqlite.NewChoresStore(db)
		reStore := sqlite.NewRemindersStore(db)
		flStore := sqlite.NewFlowStore(db)
		notifStore := sqlite.NewNotificationStore(db)

		hhSvc := household.NewService(hhStore)
		_ = hhSvc.SyncFromConfig(ctx, users, "en")

		shSvc := shopping.NewService(shStore, hhSvc)
		chSvc := chores.NewService(chStore, hhSvc, clock)
		reSvc := reminders.NewService(reStore, hhSvc, clock)
		flMgr := flow.NewManager(flStore, clock)

		tgNotifier := telegram.NewNotifier(client, hhSvc, bundle, chatID, "en")
		sched := scheduler.New(scheduler.Config{
			ChoresStore:      chStore,
			RemindersStore:   reStore,
			RemindersService: reSvc,
			NotifStore:       notifStore,
			Notifier:         tgNotifier,
			Onboarding:       hhSvc,
			Clock:            clock,
			ReminderInterval: 1 * time.Hour,
		})

		router := telegram.NewRouter(
			telegram.RouterConfig{
				ChatID:           chatID,
				ShoppingTopicID:  shoppingTopicID,
				ChoresTopicID:    choresTopicID,
				RemindersTopicID: remindersTopicID,
				DefaultLocale:    "en",
				FlowTTL:          30 * time.Minute,
			},
			client,
			hhSvc,
			shSvc,
			chSvc,
			reSvc,
			flMgr,
			bundle,
			clock,
			nil,
		)

		return hhSvc, shSvc, chSvc, reSvc, sched, router
	}

	hhSvc, _, _, _, _, router := buildServices()

	// Alice and Bob onboard via /start
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 1,
			Chat:      telegram.Chat{ID: 101, Type: "private"},
			From:      &telegram.User{ID: 101, FirstName: "Alice"},
			Text:      "/start",
		},
	})
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 2,
			Chat:      telegram.Chat{ID: 102, Type: "private"},
			From:      &telegram.User{ID: 102, FirstName: "Bob"},
			Text:      "/start",
		},
	})

	aliceConn, _ := hhSvc.IsConnected(ctx, 101)
	bobConn, _ := hhSvc.IsConnected(ctx, 102)
	if !aliceConn || !bobConn {
		t.Fatalf("expected both Alice and Bob connected after /start")
	}

	// ========================================================
	// Workflow 1: Shopping Success Scenario
	// ========================================================
	// 1. Alice sends /add in Shopping topic
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 3,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: shoppingTopicID,
			Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	})
	// 2. Alice sends name "Milk"
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 4,
		Message: &telegram.Message{
			MessageID:       11,
			MessageThreadID: shoppingTopicID,
			Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Milk",
		},
	})
	// 3. Alice sends emoji "🥛"
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 5,
		Message: &telegram.Message{
			MessageID:       12,
			MessageThreadID: shoppingTopicID,
			Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "🥛",
		},
	})
	// 4. Alice adds Milk to list
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 6,
		Message: &telegram.Message{
			MessageID:       13,
			MessageThreadID: shoppingTopicID,
			Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	})
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 7,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_add_milk",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       14,
				MessageThreadID: shoppingTopicID,
				Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			},
			Data: "fl:sel:1",
		},
	})

	// 5. Bob runs /list in Shopping topic
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 8,
		Message: &telegram.Message{
			MessageID:       15,
			MessageThreadID: shoppingTopicID,
			Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			From:            &telegram.User{ID: 102, FirstName: "Bob"},
			Text:            "/list",
		},
	})

	// 6. Bob taps Milk to purchase ("sh:p:1")
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 9,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_buy_milk",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID:       16,
				MessageThreadID: shoppingTopicID,
				Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			},
			Data: "sh:p:1",
		},
	})

	lastMsg := client.LastSent()
	if lastMsg == nil || !strings.Contains(lastMsg.Text, "purchased by Bob") {
		t.Fatalf("expected group purchase announcement for Bob, got: %+v", lastMsg)
	}

	// Double-tap purchase idempotency
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 10,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_buy_milk_again",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       16,
				MessageThreadID: shoppingTopicID,
				Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			},
			Data: "sh:p:1",
		},
	})
	ans := client.LastAnswer()
	if ans == nil || !strings.Contains(ans.Text, "already purchased") {
		t.Fatalf("expected already purchased notice, got: %+v", ans)
	}

	// ========================================================
	// RESTART IN THE MIDDLE
	// ========================================================
	_ = db.Close()

	// Re-open SQLite from disk file
	db, err = sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to re-open sqlite after restart: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations on re-opened db: %v", err)
	}

	// Re-initialize services
	_, shSvc, chSvc, reSvc, sched, router := buildServices()

	// Verify persistence across restart
	items, err := shSvc.ListItems(ctx)
	if err != nil || len(items) != 1 || items[0].Name != "Milk" {
		t.Fatalf("catalog item did not persist across restart: %+v", items)
	}
	pending, err := shSvc.ListPending(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending list should be empty after purchase: %+v", pending)
	}

	// ========================================================
	// Workflow 2: Chore Weekly with Done
	// ========================================================
	// Alice schedules weekly chore "Vacuum carpets" assigned to Alice due next Monday
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 11,
		Message: &telegram.Message{
			MessageID:       20,
			MessageThreadID: choresTopicID,
			Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	})
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 12,
		Message: &telegram.Message{
			MessageID:       21,
			MessageThreadID: choresTopicID,
			Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Vacuum carpets",
		},
	})
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 13,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_assign_alice",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       22,
				MessageThreadID: choresTopicID,
				Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			},
			Data: "fl:usr:101",
		},
	})
	// Typed date: next Monday 10:00 (2026-10-12 10:00)
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 14,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_due_custom",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       23,
				MessageThreadID: choresTopicID,
				Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			},
			Data: "fl:due:custom",
		},
	})
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 15,
		Message: &telegram.Message{
			MessageID:       24,
			MessageThreadID: choresTopicID,
			Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "2026-10-12 10:00",
		},
	})
	// Weekly recurrence
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 16,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_rec_week",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       25,
				MessageThreadID: choresTopicID,
				Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			},
			Data: "fl:rec:weekly",
		},
	})
	// Scheduled anchor
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 17,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_anc_sched",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       26,
				MessageThreadID: choresTopicID,
				Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			},
			Data: "fl:anc:scheduled",
		},
	})
	// Confirm
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 18,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_ok_vacuum",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       27,
				MessageThreadID: choresTopicID,
				Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			},
			Data: "fl:ok",
		},
	})

	// Verify occurrence created for Oct 12 10:00
	aliceChores, err := chSvc.ListMyOpen(ctx, 101)
	if err != nil || len(aliceChores.Occurrences) != 1 {
		t.Fatalf("expected 1 open chore for Alice, got: %+v", aliceChores)
	}
	choreOccID := aliceChores.Occurrences[0].ID

	// Advance clock to due date
	clock.Set(time.Date(2026, 10, 12, 10, 0, 0, 0, loc))

	// Scheduler scans due occurrences -> private notification sent to Alice
	if err := sched.ScanAndNotify(ctx); err != nil {
		t.Fatalf("scheduler scan failed: %v", err)
	}
	lastMsg = client.LastSent()
	if lastMsg == nil || lastMsg.ChatID != 101 || !strings.Contains(lastMsg.Text, "Vacuum carpets") {
		t.Fatalf("expected private due notification for Alice, got: %+v", lastMsg)
	}

	// Alice completes the chore (tap Done: "ch:c:<choreOccID>")
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 19,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_done_vacuum",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID: 28,
				Chat:      telegram.Chat{ID: 101, Type: "private"},
			},
			Data: fmt.Sprintf("ch:c:%d", choreOccID),
		},
	})

	// Verify group update posted in Chores topic
	lastMsg = client.LastSent()
	if lastMsg == nil || lastMsg.ThreadID != choresTopicID || !strings.Contains(lastMsg.Text, "completed by Alice") {
		t.Fatalf("expected group completion update in Chores topic, got: %+v", lastMsg)
	}

	// Verify successor occurrence created for next Monday (Oct 19 10:00)
	aliceChoresAfter, err := chSvc.ListMyOpen(ctx, 101)
	if err != nil || len(aliceChoresAfter.Occurrences) != 1 {
		t.Fatalf("expected 1 successor occurrence for Alice, got: %+v", aliceChoresAfter)
	}
	successor := aliceChoresAfter.Occurrences[0]
	if successor.ID == choreOccID {
		t.Fatalf("successor occurrence should be a new ID, got original: %d", successor.ID)
	}
	expectedNext := time.Date(2026, 10, 19, 10, 0, 0, 0, loc).UTC()
	if !successor.DueAt.Equal(expectedNext) {
		t.Fatalf("expected successor due at %v, got %v", expectedNext, successor.DueAt)
	}

	// ========================================================
	// Workflow 3: Reminder with Postpone
	// ========================================================
	// Alice schedules reminder "Dentist appointment" for Bob due Friday 10:00 (2026-10-16 10:00)
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 20,
		Message: &telegram.Message{
			MessageID:       30,
			MessageThreadID: remindersTopicID,
			Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	})
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 21,
		Message: &telegram.Message{
			MessageID:       31,
			MessageThreadID: remindersTopicID,
			Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Dentist appointment",
		},
	})
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 22,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_target_bob_dentist",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       32,
				MessageThreadID: remindersTopicID,
				Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			},
			Data: "fl:usr:102",
		},
	})
	// Typed due date
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 23,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_due_custom_dentist",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       33,
				MessageThreadID: remindersTopicID,
				Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			},
			Data: "fl:due:custom",
		},
	})
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 24,
		Message: &telegram.Message{
			MessageID:       34,
			MessageThreadID: remindersTopicID,
			Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "2026-10-16 10:00",
		},
	})
	// Recurrence None
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 25,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_rec_none_dentist",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       35,
				MessageThreadID: remindersTopicID,
				Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			},
			Data: "fl:rec:none",
		},
	})
	// Confirm
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 26,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_ok_dentist",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       36,
				MessageThreadID: remindersTopicID,
				Chat:            telegram.Chat{ID: chatID, Type: "supergroup"},
			},
			Data: "fl:ok",
		},
	})

	bobReminders, err := reSvc.ListMyOpen(ctx, 102)
	if err != nil || len(bobReminders.Occurrences) != 1 {
		t.Fatalf("expected 1 reminder for Bob, got: %+v", bobReminders)
	}
	remOccID := bobReminders.Occurrences[0].ID

	// Advance clock to Friday 10:00
	clock.Set(time.Date(2026, 10, 16, 10, 0, 0, 0, loc))

	// Scheduler scans due reminders -> notification sent to Bob
	_ = sched.ScanAndNotify(ctx)
	lastMsg = client.LastSent()
	if lastMsg == nil || lastMsg.ChatID != 102 || !strings.Contains(lastMsg.Text, "Dentist appointment") {
		t.Fatalf("expected due notification for Bob, got: %+v", lastMsg)
	}

	// Bob postpones by 3 hours ("re:p:<remOccID>:3h")
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 27,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_p_3h_dentist",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID: 37,
				Chat:      telegram.Chat{ID: 102, Type: "private"},
			},
			Data: fmt.Sprintf("re:p:%d:3h", remOccID),
		},
	})

	// Verify group update in Reminders topic
	lastMsg = client.LastSent()
	if lastMsg == nil || lastMsg.ThreadID != remindersTopicID || !strings.Contains(lastMsg.Text, "postponed it until") {
		t.Fatalf("expected group postpone update in Reminders topic, got: %+v", lastMsg)
	}

	// Verify due_at is now 13:00 (10:00 + 3h)
	freshRem, err := reSvc.GetOccurrence(ctx, remOccID)
	if err != nil {
		t.Fatalf("failed to get reminder: %v", err)
	}
	expectedPostponedDue := time.Date(2026, 10, 16, 13, 0, 0, 0, loc).UTC()
	if !freshRem.DueAt.Equal(expectedPostponedDue) {
		t.Fatalf("expected due_at 13:00 (%v), got: %v", expectedPostponedDue, freshRem.DueAt)
	}

	// Advance clock by 1 hour (11:00) -> scheduler should NOT notify because postponed to 13:00
	clock.Advance(1 * time.Hour)
	sentBefore := len(client.sent)
	_ = sched.ScanAndNotify(ctx)
	if len(client.sent) != sentBefore {
		t.Fatalf("expected no notifications at 11:00 for postponed reminder")
	}

	// Advance clock to 13:00 -> scheduler notifies Bob!
	clock.Advance(2 * time.Hour)
	_ = sched.ScanAndNotify(ctx)
	lastMsg = client.LastSent()
	if lastMsg == nil || lastMsg.ChatID != 102 || !strings.Contains(lastMsg.Text, "Dentist appointment") {
		t.Fatalf("expected notification at postponed time 13:00 for Bob, got: %+v", lastMsg)
	}

	// Bob completes the reminder ("re:c:<remOccID>")
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 28,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_done_dentist",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID: 38,
				Chat:      telegram.Chat{ID: 102, Type: "private"},
			},
			Data: fmt.Sprintf("re:c:%d", remOccID),
		},
	})

	// Verify reminder completed and group update posted
	lastMsg = client.LastSent()
	if lastMsg == nil || lastMsg.ThreadID != remindersTopicID || !strings.Contains(lastMsg.Text, "completed by Bob") {
		t.Fatalf("expected completion update in Reminders topic, got: %+v", lastMsg)
	}

	bobRemindersFinal, _ := reSvc.ListMyOpen(ctx, 102)
	if len(bobRemindersFinal.Occurrences) != 0 {
		t.Fatalf("expected 0 open reminders for Bob after completion, got: %+v", bobRemindersFinal)
	}
}

func TestStartup_UnreachableAPI_LogsErrorsAndContinues(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	// Unreachable API base URL
	client := telegram.NewHTTPClientWithBaseURL("http://127.0.0.1:1")

	localesDir := filepath.Clean("../../locales")
	bundle, err := i18n.NewBundle(localesDir, "en", i18n.DefaultRequiredKeys, logger)
	if err != nil {
		t.Fatalf("failed to load i18n: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Run command registration against unreachable API
	telegram.RegisterCommands(ctx, client, bundle, -1003916572365, bundle.Locales(), logger)

	// Check that errors were logged naming scope and lang
	logStr := logBuf.String()
	if !strings.Contains(logStr, `"scope":"group"`) || !strings.Contains(logStr, `"scope":"private"`) {
		t.Fatalf("expected logged registration errors for group and private scopes, got: %s", logStr)
	}

	// 2. Verify dispatcher long-polling still starts
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	store := sqlite.NewTelegramStore(db)
	disp := telegram.NewDispatcher(client, store, telegram.UpdateHandlerFunc(func(_ context.Context, _ telegram.Update) error {
		return nil
	}), logger)

	pollCtx, pollCancel := context.WithCancel(context.Background())
	pollCancel() // cancel immediately so Start returns cleanly

	err = disp.Start(pollCtx)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected dispatcher to start and stop on cancellation, got: %v", err)
	}
}
