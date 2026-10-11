package telegram

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/px1/home-ctl/internal/chores"
	"github.com/px1/home-ctl/internal/flow"
	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/i18n"
	"github.com/px1/home-ctl/internal/reminders"
	"github.com/px1/home-ctl/internal/shopping"
)

// Clock provides current time and location.
type Clock interface {
	Now() time.Time
	Location() *time.Location
}

// RouterConfig contains routing targets and configuration settings.
type RouterConfig struct {
	ChatID           int64
	ShoppingTopicID  int64
	ChoresTopicID    int64
	RemindersTopicID int64
	DefaultLocale    string
	FlowTTL          time.Duration
	// DefaultTimeOfDay ("HH:MM") is the time used for date-only choices such as "Tomorrow".
	DefaultTimeOfDay string
	// BotUsername builds the connect deep link; empty when it could not be read at startup.
	BotUsername string
}

// Router dispatches incoming Telegram updates to domain handlers.
type Router struct {
	cfg       RouterConfig
	client    Client
	household *household.Service
	shopping  *shopping.Service
	chores    *chores.Service
	reminders *reminders.Service
	flow      *flow.Manager
	i18n      *i18n.Bundle
	clock     Clock
	logger    *slog.Logger

	defaultTime timeOfDay
}

// RouterDeps groups the clients, domain services and infrastructure a Router depends on.
type RouterDeps struct {
	Client    Client
	Household *household.Service
	Shopping  *shopping.Service
	Chores    *chores.Service
	Reminders *reminders.Service
	Flow      *flow.Manager
	I18n      *i18n.Bundle
	Clock     Clock
	// Logger falls back to slog.Default when nil.
	Logger *slog.Logger
}

// NewRouter constructs a Router.
func NewRouter(cfg RouterConfig, deps RouterDeps) *Router {
	if cfg.DefaultLocale == "" {
		cfg.DefaultLocale = "en"
	}
	if cfg.FlowTTL == 0 {
		cfg.FlowTTL = 30 * time.Minute
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	defaultTime, err := parseTimeOfDay(cfg.DefaultTimeOfDay)
	if err != nil {
		if cfg.DefaultTimeOfDay != "" {
			logger.Warn("invalid default time of day, using fallback", "value", cfg.DefaultTimeOfDay, "fallback", fallbackDefaultTimeOfDay)
		}
		defaultTime, _ = parseTimeOfDay(fallbackDefaultTimeOfDay)
	}
	return &Router{
		cfg:       cfg,
		client:    deps.Client,
		household: deps.Household,
		shopping:  deps.Shopping,
		chores:    deps.Chores,
		reminders: deps.Reminders,
		flow:      deps.Flow,
		i18n:      deps.I18n,
		clock:     deps.Clock,
		logger:    logger,

		defaultTime: defaultTime,
	}
}

// HandleUpdate implements UpdateHandler.
func (r *Router) HandleUpdate(ctx context.Context, update Update) error {
	if update.Message != nil {
		return r.handleMessage(ctx, update.Message)
	}
	if update.CallbackQuery != nil {
		return r.handleCallbackQuery(ctx, update.CallbackQuery)
	}
	return nil
}

func (r *Router) handleMessage(ctx context.Context, msg *Message) error {
	if msg.From == nil {
		return nil
	}
	userID := msg.From.ID
	chatID := msg.Chat.ID
	isPrivate := msg.Chat.Type == "private"

	// Validate chat: if not private, it must match configured group ChatID
	if !isPrivate && chatID != r.cfg.ChatID {
		r.logger.Debug("ignoring message from unknown chat", "chat_id", chatID)
		return nil
	}

	// Authorize member
	member, err := r.household.Authorize(ctx, userID)
	if err != nil {
		r.rejectUnauthorizedMessage(ctx, msg, isPrivate)
		return nil
	}
	if isPrivate {
		r.recordOnboarding(ctx, userID)
	}

	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	text := strings.TrimSpace(msg.Text)

	// Check if user has an active flow awaiting text input
	flowKey := flow.Key{
		UserID:   userID,
		ChatID:   chatID,
		ThreadID: msg.MessageThreadID,
	}
	if state, err := r.flow.Get(ctx, flowKey, nil); err == nil && state != nil {
		return r.handleFlowText(ctx, member, msg, state)
	}

	// Command routing
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return nil
	}
	cmd := parts[0]
	if atIdx := strings.Index(cmd, "@"); atIdx != -1 {
		cmd = cmd[:atIdx]
	}

	if isPrivate {
		return r.handlePrivateCommand(ctx, member, msg, cmd)
	}
	return r.handleGroupCommand(ctx, member, msg, cmd, locale)
}

// rejectUnauthorizedMessage handles a message from a user who is not a configured member.
// Nothing is recorded; only an unconfigured user's /start in private chat gets a reply.
func (r *Router) rejectUnauthorizedMessage(ctx context.Context, msg *Message, isPrivate bool) {
	// If unconfigured user sends /start in private chat, reply that access is not configured
	if isPrivate && strings.HasPrefix(msg.Text, "/start") {
		_, _ = r.client.SendMessage(ctx, msg.Chat.ID, r.i18n.T(r.cfg.DefaultLocale, "general.not_member"), nil)
		return
	}
	// In all other cases, reject without state creation
	r.logger.Debug("ignoring message from unauthorized user", "user_id", msg.From.ID)
}

func (r *Router) handlePrivateCommand(ctx context.Context, member *household.Member, msg *Message, cmd string) error {
	switch cmd {
	case "/start":
		return r.handleStart(ctx, member, msg)
	case "/help":
		return r.handleHelp(ctx, member, msg)
	case "/status":
		return r.handleStatus(ctx, member, msg)
	case "/list":
		return r.handlePrivateList(ctx, member, msg)
	default:
		return r.handleHelp(ctx, member, msg)
	}
}

func (r *Router) handleGroupCommand(ctx context.Context, member *household.Member, msg *Message, cmd, locale string) error {
	switch cmd {
	case "/status":
		return r.handleStatus(ctx, member, msg)
	case "/help":
		return r.handleHelp(ctx, member, msg)
	case "/add":
		return r.handleTopicAdd(ctx, member, msg, locale)
	case "/list":
		return r.handleTopicList(ctx, member, msg, locale)
	}
	return nil
}

// handleTopicAdd routes /add to the domain handler mapped to the topic it was sent in.
func (r *Router) handleTopicAdd(ctx context.Context, member *household.Member, msg *Message, locale string) error {
	switch msg.MessageThreadID {
	case r.cfg.ShoppingTopicID:
		return r.handleShoppingAdd(ctx, member, msg)
	case r.cfg.ChoresTopicID:
		return r.handleChoresAdd(ctx, member, msg)
	case r.cfg.RemindersTopicID:
		return r.handleRemindersAdd(ctx, member, msg)
	default:
		r.replyUnmappedTopic(ctx, msg, locale)
		return nil
	}
}

// handleTopicList routes /list to the domain handler mapped to the topic it was sent in.
func (r *Router) handleTopicList(ctx context.Context, member *household.Member, msg *Message, locale string) error {
	switch msg.MessageThreadID {
	case r.cfg.ShoppingTopicID:
		return r.handleShoppingList(ctx, member, msg)
	case r.cfg.ChoresTopicID:
		return r.handleChoresListCommand(ctx, member, msg)
	case r.cfg.RemindersTopicID:
		return r.handleRemindersListCommand(ctx, member, msg)
	default:
		r.replyUnmappedTopic(ctx, msg, locale)
		return nil
	}
}

func (r *Router) replyUnmappedTopic(ctx context.Context, msg *Message, locale string) {
	_, _ = r.client.SendMessage(ctx, msg.Chat.ID, r.i18n.T(locale, "general.unmapped_topic"), &SendOptions{
		ThreadID: msg.MessageThreadID,
	})
}

// recordOnboarding connects an authorized member after any private interaction: a private message
// or button press proves the bot can write to them. Failures are logged; they never block the reply.
func (r *Router) recordOnboarding(ctx context.Context, userID int64) {
	if err := r.household.EnsureConnected(ctx, userID, r.clock.Now().UTC()); err != nil {
		r.logger.Error("failed to record onboarding", "user_id", userID, "err", err)
	}
}

func (r *Router) handleCallbackQuery(ctx context.Context, cq *CallbackQuery) error {
	actorID := cq.From.ID // ALWAYS cq.From.ID!
	if cq.Message != nil {
		isPrivate := cq.Message.Chat.Type == "private"
		if !isPrivate && cq.Message.Chat.ID != r.cfg.ChatID {
			return nil
		}
	}

	// Authorize member
	member, err := r.household.Authorize(ctx, actorID)
	if err != nil {
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(r.cfg.DefaultLocale, "general.not_member"), true)
		return nil
	}
	if cq.Message != nil && cq.Message.Chat.Type == "private" {
		r.recordOnboarding(ctx, actorID)
	}

	parsed, err := ParseCallbackData(cq.Data, actorID)
	if err != nil {
		r.logger.Warn("malformed callback query", "data", cq.Data, "err", err)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return nil
	}

	switch parsed.Kind {
	case CallbackShoppingPurchase:
		return r.handleShoppingPurchaseCallback(ctx, member, cq, parsed)

	case CallbackChoreComplete, CallbackChorePostpone, CallbackChoreDetail, CallbackListChores:
		return r.handleChoreCallback(ctx, member, cq, parsed)

	case CallbackReminderComplete, CallbackReminderPostpone, CallbackReminderDetail, CallbackListReminders:
		return r.handleReminderCallback(ctx, member, cq, parsed)

	case CallbackFlowCancel, CallbackFlowConfirm, CallbackFlowSelect, CallbackFlowUser, CallbackFlowDue, CallbackFlowRecurrence, CallbackFlowAnchor, CallbackFlowNewItem,
		CallbackFlowNoop, CallbackFlowRecurrenceType, CallbackFlowInterval, CallbackFlowCalendar, CallbackFlowDate, CallbackFlowHour, CallbackFlowMinute,
		CallbackFlowEmoji:
		return r.handleFlowCallback(ctx, member, cq, parsed)

	default:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return nil
	}
}
