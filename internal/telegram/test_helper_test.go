package telegram_test

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/px1/home-ctl/internal/chores"
	"github.com/px1/home-ctl/internal/config"
	"github.com/px1/home-ctl/internal/flow"
	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/i18n"
	"github.com/px1/home-ctl/internal/reminders"
	"github.com/px1/home-ctl/internal/shopping"
	"github.com/px1/home-ctl/internal/storage/sqlite"
	"github.com/px1/home-ctl/internal/telegram"
)

type sentMessage struct {
	MessageID int64
	ChatID    int64
	ThreadID  int64
	Text      string
	Markup    *telegram.InlineKeyboardMarkup
}

type editedMessage struct {
	ChatID    int64
	MessageID int64
	Text      string
	Markup    *telegram.InlineKeyboardMarkup
}

type callbackAnswer struct {
	QueryID string
	Text    string
	Alert   bool
}

type setMyCommandsCall struct {
	Scope        telegram.BotCommandScope
	LanguageCode string
	Commands     []telegram.BotCommand
}

type fakeClient struct {
	mu                 sync.Mutex
	nextMsgID          int64
	sentMessages       []sentMessage
	editedMessages     []editedMessage
	callbackAnswers    []callbackAnswer
	setMyCommandsCalls []setMyCommandsCall
	onSetMyCommands    func(scope telegram.BotCommandScope, lang string, cmds []telegram.BotCommand) error
	onEditMessageText  func(ctx context.Context, chatID int64, messageID int64, text string, opts *telegram.EditOptions) error
	deletedMessages    []int64
	clearedKeyboards   []int64
	onDeleteMessage    func(chatID int64, messageID int64) error
}

func newFakeClient() *fakeClient {
	return &fakeClient{nextMsgID: 100}
}

func (f *fakeClient) SendMessage(_ context.Context, chatID int64, text string, opts *telegram.SendOptions) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextMsgID++
	var threadID int64
	var markup *telegram.InlineKeyboardMarkup
	if opts != nil {
		threadID = opts.ThreadID
		markup = opts.ReplyMarkup
	}
	f.sentMessages = append(f.sentMessages, sentMessage{
		MessageID: f.nextMsgID,
		ChatID:    chatID,
		ThreadID:  threadID,
		Text:      text,
		Markup:    markup,
	})
	return f.nextMsgID, nil
}

func (f *fakeClient) EditMessageText(ctx context.Context, chatID int64, messageID int64, text string, opts *telegram.EditOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.onEditMessageText != nil {
		if err := f.onEditMessageText(ctx, chatID, messageID, text, opts); err != nil {
			return err
		}
	}
	var markup *telegram.InlineKeyboardMarkup
	if opts != nil {
		markup = opts.ReplyMarkup
	}
	f.editedMessages = append(f.editedMessages, editedMessage{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
		Markup:    markup,
	})
	return nil
}

func (f *fakeClient) EditMessageReplyMarkup(_ context.Context, _ int64, messageID int64, _ *telegram.InlineKeyboardMarkup) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clearedKeyboards = append(f.clearedKeyboards, messageID)
	return nil
}

func (f *fakeClient) DeleteMessage(_ context.Context, chatID int64, messageID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.onDeleteMessage != nil {
		if err := f.onDeleteMessage(chatID, messageID); err != nil {
			return err
		}
	}
	f.deletedMessages = append(f.deletedMessages, messageID)
	return nil
}

// DeletedMessages returns the ids of messages deleted through DeleteMessage.
func (f *fakeClient) DeletedMessages() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.deletedMessages...)
}

// ClearedKeyboards returns the ids of messages whose keyboard was removed via EditMessageReplyMarkup.
func (f *fakeClient) ClearedKeyboards() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.clearedKeyboards...)
}

func (f *fakeClient) AnswerCallbackQuery(_ context.Context, callbackQueryID string, text string, showAlert bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callbackAnswers = append(f.callbackAnswers, callbackAnswer{
		QueryID: callbackQueryID,
		Text:    text,
		Alert:   showAlert,
	})
	return nil
}

func (f *fakeClient) GetUpdates(_ context.Context, _ int64, _ int) ([]telegram.Update, error) {
	return nil, nil
}

func (f *fakeClient) SetMyCommands(_ context.Context, scope telegram.BotCommandScope, languageCode string, commands []telegram.BotCommand) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setMyCommandsCalls = append(f.setMyCommandsCalls, setMyCommandsCall{
		Scope:        scope,
		LanguageCode: languageCode,
		Commands:     commands,
	})
	if f.onSetMyCommands != nil {
		return f.onSetMyCommands(scope, languageCode, commands)
	}
	return nil
}

func (f *fakeClient) LastSent() *sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sentMessages) == 0 {
		return nil
	}
	cp := f.sentMessages[len(f.sentMessages)-1]
	return &cp
}

func (f *fakeClient) LastEdited() *editedMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.editedMessages) == 0 {
		return nil
	}
	cp := f.editedMessages[len(f.editedMessages)-1]
	return &cp
}

func (f *fakeClient) SentMessages() []sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]sentMessage, len(f.sentMessages))
	copy(cp, f.sentMessages)
	return cp
}

func (f *fakeClient) EditedMessages() []editedMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]editedMessage, len(f.editedMessages))
	copy(cp, f.editedMessages)
	return cp
}

func (f *fakeClient) LastAnswer() *callbackAnswer {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.callbackAnswers) == 0 {
		return nil
	}
	cp := f.callbackAnswers[len(f.callbackAnswers)-1]
	return &cp
}

type testClock struct {
	now time.Time
	loc *time.Location
}

func (tc *testClock) Now() time.Time {
	return tc.now
}

func (tc *testClock) Location() *time.Location {
	return tc.loc
}

type testEnv struct {
	router    *telegram.Router
	client    *fakeClient
	clock     *testClock
	household *household.Service
	shopping  *shopping.Service
	chores    *chores.Service
	reminders *reminders.Service
	flow      *flow.Manager
	i18n      *i18n.Bundle
}

func setupTestEnv() (*testEnv, error) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		return nil, err
	}

	if err := sqlite.RunMigrations(db); err != nil {
		return nil, err
	}

	loc, err := time.LoadLocation("America/Guayaquil")
	if err != nil {
		loc = time.UTC
	}
	clock := &testClock{
		now: time.Date(2026, 10, 6, 12, 0, 0, 0, loc),
		loc: loc,
	}

	// Stores
	hhStore := sqlite.NewHouseholdStore(db)
	shStore := sqlite.NewShoppingStore(db)
	chStore := sqlite.NewChoresStore(db)
	reStore := sqlite.NewRemindersStore(db)
	flStore := sqlite.NewFlowStore(db)

	hhSvc := household.NewService(hhStore)
	users := []config.UserConfig{
		{ID: 101, Name: "Alice", Username: "alice", Locale: "en"},
		{ID: 102, Name: "Bob", Username: "bob", Locale: "en"},
		{ID: 103, Name: "Charlie", Username: "charlie", Locale: "es"},
	}
	if err := hhSvc.SyncFromConfig(ctx, users, "en"); err != nil {
		return nil, err
	}

	// Connect Alice and Bob
	_ = hhStore.RecordOnboarding(ctx, 101, clock.now)
	_ = hhStore.RecordOnboarding(ctx, 102, clock.now)
	// Charlie is NOT connected!

	shSvc := shopping.NewService(shStore, hhSvc)
	chSvc := chores.NewService(chStore, hhSvc, clock)
	reSvc := reminders.NewService(reStore, hhSvc, clock)
	flMgr := flow.NewManager(flStore, clock)

	localesDir := filepath.Clean("../../locales")
	bundle, err := i18n.NewBundle(localesDir, "en", i18n.DefaultRequiredKeys, nil)
	if err != nil {
		return nil, err
	}

	client := newFakeClient()
	cfg := telegram.RouterConfig{
		ChatID:           -1001234567890,
		ShoppingTopicID:  42,
		ChoresTopicID:    51,
		RemindersTopicID: 63,
		DefaultLocale:    "en",
		FlowTTL:          30 * time.Minute,
	}

	router := telegram.NewRouter(cfg, client, hhSvc, shSvc, chSvc, reSvc, flMgr, bundle, clock, nil)

	return &testEnv{
		router:    router,
		client:    client,
		clock:     clock,
		household: hhSvc,
		shopping:  shSvc,
		chores:    chSvc,
		reminders: reSvc,
		flow:      flMgr,
		i18n:      bundle,
	}, nil
}

func flowKeyFor(userID, chatID, threadID int64) flow.Key {
	return flow.Key{
		UserID:   userID,
		ChatID:   chatID,
		ThreadID: threadID,
	}
}
