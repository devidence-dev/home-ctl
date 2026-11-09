package main_test

import (
	"bytes"
	"context"
	"database/sql"
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
	MessageID int64
	ChatID    int64
	ThreadID  int64
	Text      string
	Markup    *telegram.InlineKeyboardMarkup
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
		MessageID: f.nextID,
		ChatID:    chatID,
		ThreadID:  threadID,
		Text:      text,
		Markup:    markup,
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

func (f *fakeTG) EditMessageReplyMarkup(_ context.Context, _ int64, _ int64, _ *telegram.InlineKeyboardMarkup) error {
	return nil
}

func (f *fakeTG) DeleteMessage(_ context.Context, _ int64, _ int64) error {
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

func (f *fakeTG) GetMe(_ context.Context) (telegram.User, error) {
	return telegram.User{ID: 999000, FirstName: "home-ctl", Username: "home_ctl_bot"}, nil
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

const (
	e2eChatID           int64 = -1001234567890
	e2eShoppingTopicID  int64 = 42
	e2eChoresTopicID    int64 = 51
	e2eRemindersTopicID int64 = 63
)

// e2eEnv holds the shared state the sequential workflow steps build on.
type e2eEnv struct {
	ctx    context.Context
	dbPath string
	loc    *time.Location
	clock  *simClock
	client *fakeTG
	bundle *i18n.Bundle
	users  []config.UserConfig
	db     *sql.DB
	hhSvc  *household.Service
	shSvc  *shopping.Service
	chSvc  *chores.Service
	reSvc  *reminders.Service
	sched  *scheduler.Scheduler
	router *telegram.Router
}

// e2eBuildServices (re)creates all services on top of env.db, as a restart does.
func e2eBuildServices(env *e2eEnv) {
	hhStore := sqlite.NewHouseholdStore(env.db)
	shStore := sqlite.NewShoppingStore(env.db)
	chStore := sqlite.NewChoresStore(env.db)
	reStore := sqlite.NewRemindersStore(env.db)
	flStore := sqlite.NewFlowStore(env.db)
	notifStore := sqlite.NewNotificationStore(env.db)

	env.hhSvc = household.NewService(hhStore)
	_ = env.hhSvc.SyncFromConfig(env.ctx, env.users, "en")

	env.shSvc = shopping.NewService(shStore, env.hhSvc)
	env.chSvc = chores.NewService(chStore, env.hhSvc, env.clock)
	env.reSvc = reminders.NewService(reStore, env.hhSvc, env.clock)
	flMgr := flow.NewManager(flStore, env.clock)

	tgNotifier := telegram.NewNotifier(env.client, env.hhSvc, env.bundle, e2eChatID, "en")
	env.sched = scheduler.New(scheduler.Config{
		ChoresStore:      chStore,
		RemindersStore:   reStore,
		RemindersService: env.reSvc,
		NotifStore:       notifStore,
		Notifier:         tgNotifier,
		Onboarding:       env.hhSvc,
		Clock:            env.clock,
		ReminderInterval: 1 * time.Hour,
	})

	env.router = telegram.NewRouter(
		telegram.RouterConfig{
			ChatID:           e2eChatID,
			ShoppingTopicID:  e2eShoppingTopicID,
			ChoresTopicID:    e2eChoresTopicID,
			RemindersTopicID: e2eRemindersTopicID,
			DefaultLocale:    "en",
			FlowTTL:          30 * time.Minute,
		},
		telegram.RouterDeps{
			Client:    env.client,
			Household: env.hhSvc,
			Shopping:  env.shSvc,
			Chores:    env.chSvc,
			Reminders: env.reSvc,
			Flow:      flMgr,
			I18n:      env.bundle,
			Clock:     env.clock,
		},
	)
}

// e2eSetupEnv opens a fresh database, runs migrations and builds the first set of services.
func e2eSetupEnv(t *testing.T) *e2eEnv {
	t.Helper()
	tempDir := t.TempDir()

	loc, err := time.LoadLocation("America/Guayaquil")
	if err != nil {
		t.Fatalf("failed to load timezone: %v", err)
	}

	env := &e2eEnv{
		ctx:    context.Background(),
		dbPath: filepath.Join(tempDir, "e2e_homectl.db"),
		loc:    loc,
		clock: &simClock{
			now: time.Date(2026, 10, 5, 8, 0, 0, 0, loc),
			loc: loc,
		},
		client: newFakeTG(),
		users: []config.UserConfig{
			{ID: 101, Name: "Alice", Username: "alice", Locale: "en"},
			{ID: 102, Name: "Bob", Username: "bob", Locale: "es"},
		},
	}

	localesDir := filepath.Clean("../../locales")
	env.bundle, err = i18n.NewBundle(localesDir, "en", i18n.DefaultRequiredKeys, nil)
	if err != nil {
		t.Fatalf("failed to load i18n: %v", err)
	}

	// Step 1: Initialize Database
	env.db, err = sqlite.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := sqlite.RunMigrations(env.db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	e2eBuildServices(env)
	return env
}

// e2eOnboard runs /start for Alice and Bob in private chats.
func e2eOnboard(t *testing.T, env *e2eEnv) {
	t.Helper()
	ctx := env.ctx
	router := env.router
	hhSvc := env.hhSvc

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
}

// e2eShoppingWorkflow runs Workflow 1: add an item, then Bob purchases it.
func e2eShoppingWorkflow(t *testing.T, env *e2eEnv) {
	t.Helper()
	ctx := env.ctx
	router := env.router
	client := env.client

	// ========================================================
	// Workflow 1: Shopping Success Scenario
	// ========================================================
	// 1. Alice sends /add in Shopping topic
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 3,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: e2eShoppingTopicID,
			Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	})
	// 2. Alice sends name "Milk"
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 4,
		Message: &telegram.Message{
			MessageID:       11,
			MessageThreadID: e2eShoppingTopicID,
			Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Milk",
		},
	})
	// 3. Alice sends emoji "🥛"
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 5,
		Message: &telegram.Message{
			MessageID:       12,
			MessageThreadID: e2eShoppingTopicID,
			Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "🥛",
		},
	})
	// 4. Alice adds Milk to list
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 6,
		Message: &telegram.Message{
			MessageID:       13,
			MessageThreadID: e2eShoppingTopicID,
			Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	})
	addMenu := client.LastSent()
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 7,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_add_milk",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       addMenu.MessageID,
				MessageThreadID: e2eShoppingTopicID,
				Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			},
			Data: "fl:sel:1",
		},
	})

	// 5. Bob runs /list in Shopping topic
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 8,
		Message: &telegram.Message{
			MessageID:       15,
			MessageThreadID: e2eShoppingTopicID,
			Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
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
				MessageThreadID: e2eShoppingTopicID,
				Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
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
				MessageThreadID: e2eShoppingTopicID,
				Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			},
			Data: "sh:p:1",
		},
	})
	ans := client.LastAnswer()
	if ans == nil || !strings.Contains(ans.Text, "already purchased") {
		t.Fatalf("expected already purchased notice, got: %+v", ans)
	}
}

// e2eRestart closes the database, reopens it from disk and rebuilds services,
// then verifies the shopping state persisted.
func e2eRestart(t *testing.T, env *e2eEnv) {
	t.Helper()
	ctx := env.ctx

	// ========================================================
	// RESTART IN THE MIDDLE
	// ========================================================
	_ = env.db.Close()

	// Re-open SQLite from disk file
	db, err := sqlite.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to re-open sqlite after restart: %v", err)
	}
	env.db = db
	t.Cleanup(func() { _ = env.db.Close() })

	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations on re-opened db: %v", err)
	}

	// Re-initialize services
	e2eBuildServices(env)

	// Verify persistence across restart
	items, err := env.shSvc.ListItems(ctx)
	if err != nil || len(items) != 1 || items[0].Name != "Milk" {
		t.Fatalf("catalog item did not persist across restart: %+v", items)
	}
	pending, err := env.shSvc.ListPending(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending list should be empty after purchase: %+v", pending)
	}
}

// e2eScheduleChore runs the first half of Workflow 2 and returns the chore occurrence ID.
func e2eScheduleChore(t *testing.T, env *e2eEnv) int64 {
	t.Helper()
	ctx := env.ctx
	router := env.router
	client := env.client

	// ========================================================
	// Workflow 2: Chore Weekly with Done
	// ========================================================
	// Alice schedules weekly chore "Vacuum carpets" assigned to Alice due next Monday
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 11,
		Message: &telegram.Message{
			MessageID:       20,
			MessageThreadID: e2eChoresTopicID,
			Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	})
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 12,
		Message: &telegram.Message{
			MessageID:       21,
			MessageThreadID: e2eChoresTopicID,
			Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Vacuum carpets",
		},
	})
	choreMenu := client.LastSent()
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 13,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_assign_alice",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       choreMenu.MessageID,
				MessageThreadID: e2eChoresTopicID,
				Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			},
			Data: "fl:usr:101",
		},
	})
	// Weekly recurrence
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 14,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_rec_week",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       choreMenu.MessageID,
				MessageThreadID: e2eChoresTopicID,
				Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			},
			Data: "fl:rt:weekly",
		},
	})
	// Interval: every 1 week
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 15,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_iv_1",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       choreMenu.MessageID,
				MessageThreadID: e2eChoresTopicID,
				Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			},
			Data: "fl:iv:1",
		},
	})
	// Type date
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 16,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_due_type",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       choreMenu.MessageID,
				MessageThreadID: e2eChoresTopicID,
				Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			},
			Data: "fl:d:type",
		},
	})
	// Typed date: next Monday 10:00 (2026-10-12 10:00)
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 17,
		Message: &telegram.Message{
			MessageID:       23,
			MessageThreadID: e2eChoresTopicID,
			Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "2026-10-12 10:00",
		},
	})
	// The typed date re-anchors the flow below the user's text: later button presses target the new message.
	choreMenu = client.LastSent()
	// Scheduled anchor
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 18,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_anc_sched",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       choreMenu.MessageID,
				MessageThreadID: e2eChoresTopicID,
				Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			},
			Data: "fl:anc:scheduled",
		},
	})
	// Confirm
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 19,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_ok_vacuum",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       choreMenu.MessageID,
				MessageThreadID: e2eChoresTopicID,
				Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			},
			Data: "fl:ok",
		},
	})

	// Verify occurrence created for Oct 12 10:00
	aliceChores, err := env.chSvc.ListMyOpen(ctx, 101)
	if err != nil || len(aliceChores.Occurrences) != 1 {
		t.Fatalf("expected 1 open chore for Alice, got: %+v", aliceChores)
	}
	return aliceChores.Occurrences[0].ID
}

// e2eCompleteChore runs the second half of Workflow 2: due notification, Done tap, successor check.
func e2eCompleteChore(t *testing.T, env *e2eEnv, choreOccID int64) {
	t.Helper()
	ctx := env.ctx
	router := env.router
	client := env.client

	// Advance clock to due date
	env.clock.Set(time.Date(2026, 10, 12, 10, 0, 0, 0, env.loc))

	// Scheduler scans due occurrences -> private notification sent to Alice
	if err := env.sched.ScanAndNotify(ctx); err != nil {
		t.Fatalf("scheduler scan failed: %v", err)
	}
	lastMsg := client.LastSent()
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
	if lastMsg == nil || lastMsg.ThreadID != e2eChoresTopicID || !strings.Contains(lastMsg.Text, "completed by Alice") {
		t.Fatalf("expected group completion update in Chores topic, got: %+v", lastMsg)
	}

	// Verify successor occurrence created for next Monday (Oct 19 10:00)
	aliceChoresAfter, err := env.chSvc.ListMyOpen(ctx, 101)
	if err != nil || len(aliceChoresAfter.Occurrences) != 1 {
		t.Fatalf("expected 1 successor occurrence for Alice, got: %+v", aliceChoresAfter)
	}
	successor := aliceChoresAfter.Occurrences[0]
	if successor.ID == choreOccID {
		t.Fatalf("successor occurrence should be a new ID, got original: %d", successor.ID)
	}
	expectedNext := time.Date(2026, 10, 19, 10, 0, 0, 0, env.loc).UTC()
	if !successor.DueAt.Equal(expectedNext) {
		t.Fatalf("expected successor due at %v, got %v", expectedNext, successor.DueAt)
	}
}

// e2eScheduleReminder runs the first half of Workflow 3 and returns Bob's reminder occurrence ID.
func e2eScheduleReminder(t *testing.T, env *e2eEnv) int64 {
	t.Helper()
	ctx := env.ctx
	router := env.router
	client := env.client

	// ========================================================
	// Workflow 3: Reminder with Postpone
	// ========================================================
	// Alice schedules reminder "Dentist appointment" for Bob due Friday 10:00 (2026-10-16 10:00)
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 20,
		Message: &telegram.Message{
			MessageID:       30,
			MessageThreadID: e2eRemindersTopicID,
			Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	})
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 21,
		Message: &telegram.Message{
			MessageID:       31,
			MessageThreadID: e2eRemindersTopicID,
			Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Dentist appointment",
		},
	})
	reminderMenu := client.LastSent()
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 22,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_target_bob_dentist",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       reminderMenu.MessageID,
				MessageThreadID: e2eRemindersTopicID,
				Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			},
			Data: "fl:usr:102",
		},
	})
	// Recurrence type: One time
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 23,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_rec_once_dentist",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       reminderMenu.MessageID,
				MessageThreadID: e2eRemindersTopicID,
				Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			},
			Data: "fl:rt:once",
		},
	})
	// Type date
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 24,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_due_type_dentist",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       reminderMenu.MessageID,
				MessageThreadID: e2eRemindersTopicID,
				Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			},
			Data: "fl:d:type",
		},
	})
	// Typed date: Friday 10:00 (2026-10-16 10:00)
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 25,
		Message: &telegram.Message{
			MessageID:       33,
			MessageThreadID: e2eRemindersTopicID,
			Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "2026-10-16 10:00",
		},
	})
	// The typed date re-anchors the flow: the confirmation is the active message from here on.
	reminderMenu = client.LastSent()
	// Confirm
	_ = router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 26,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_ok_dentist",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       reminderMenu.MessageID,
				MessageThreadID: e2eRemindersTopicID,
				Chat:            telegram.Chat{ID: e2eChatID, Type: "supergroup"},
			},
			Data: "fl:ok",
		},
	})

	bobReminders, err := env.reSvc.ListMyOpen(ctx, 102)
	if err != nil || len(bobReminders.Occurrences) != 1 {
		t.Fatalf("expected 1 reminder for Bob, got: %+v", bobReminders)
	}
	return bobReminders.Occurrences[0].ID
}

// e2ePostponeReminder runs the due notification, the 3h postpone and the repeat-suppression checks.
func e2ePostponeReminder(t *testing.T, env *e2eEnv, remOccID int64) {
	t.Helper()
	ctx := env.ctx
	router := env.router
	client := env.client

	// Advance clock to Friday 10:00
	env.clock.Set(time.Date(2026, 10, 16, 10, 0, 0, 0, env.loc))

	// Scheduler scans due reminders -> notification sent to Bob
	_ = env.sched.ScanAndNotify(ctx)
	lastMsg := client.LastSent()
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
	if lastMsg == nil || lastMsg.ThreadID != e2eRemindersTopicID || !strings.Contains(lastMsg.Text, "postponed it until") {
		t.Fatalf("expected group postpone update in Reminders topic, got: %+v", lastMsg)
	}

	// Verify due_at is now 13:00 (10:00 + 3h)
	freshRem, err := env.reSvc.GetOccurrence(ctx, remOccID)
	if err != nil {
		t.Fatalf("failed to get reminder: %v", err)
	}
	expectedPostponedDue := time.Date(2026, 10, 16, 13, 0, 0, 0, env.loc).UTC()
	if !freshRem.DueAt.Equal(expectedPostponedDue) {
		t.Fatalf("expected due_at 13:00 (%v), got: %v", expectedPostponedDue, freshRem.DueAt)
	}

	// Advance clock by 1 hour (11:00) -> scheduler should NOT notify because postponed to 13:00
	env.clock.Advance(1 * time.Hour)
	sentBefore := len(client.sent)
	_ = env.sched.ScanAndNotify(ctx)
	if len(client.sent) != sentBefore {
		t.Fatalf("expected no notifications at 11:00 for postponed reminder")
	}

	// Advance clock to 13:00 -> scheduler notifies Bob!
	env.clock.Advance(2 * time.Hour)
	_ = env.sched.ScanAndNotify(ctx)
	lastMsg = client.LastSent()
	if lastMsg == nil || lastMsg.ChatID != 102 || !strings.Contains(lastMsg.Text, "Dentist appointment") {
		t.Fatalf("expected notification at postponed time 13:00 for Bob, got: %+v", lastMsg)
	}
}

// e2eCompleteReminder runs Bob's Done tap and verifies no open reminders remain.
func e2eCompleteReminder(t *testing.T, env *e2eEnv, remOccID int64) {
	t.Helper()
	ctx := env.ctx
	router := env.router
	client := env.client

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
	lastMsg := client.LastSent()
	if lastMsg == nil || lastMsg.ThreadID != e2eRemindersTopicID || !strings.Contains(lastMsg.Text, "completed by Bob") {
		t.Fatalf("expected completion update in Reminders topic, got: %+v", lastMsg)
	}

	bobRemindersFinal, _ := env.reSvc.ListMyOpen(ctx, 102)
	if len(bobRemindersFinal.Occurrences) != 0 {
		t.Fatalf("expected 0 open reminders for Bob after completion, got: %+v", bobRemindersFinal)
	}
}

func TestEndToEnd_WorkflowsWithRestart(t *testing.T) {
	env := e2eSetupEnv(t)
	e2eOnboard(t, env)
	e2eShoppingWorkflow(t, env)

	e2eRestart(t, env)

	choreOccID := e2eScheduleChore(t, env)
	e2eCompleteChore(t, env, choreOccID)

	remOccID := e2eScheduleReminder(t, env)
	e2ePostponeReminder(t, env, remOccID)
	e2eCompleteReminder(t, env, remOccID)
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
