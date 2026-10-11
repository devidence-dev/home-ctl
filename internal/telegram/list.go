package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/px1/home-ctl/internal/chores"
	"github.com/px1/home-ctl/internal/flow"
	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/reminders"
)

func (r *Router) handleChoresListCommand(ctx context.Context, member *household.Member, msg *Message) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	connected, err := r.household.IsConnected(ctx, member.ID)
	if err != nil || !connected {
		return r.sendUnconnectedNotice(ctx, locale, msg)
	}

	return r.sendPrivateChoresList(ctx, member, member.ID)
}

func (r *Router) handleRemindersListCommand(ctx context.Context, member *household.Member, msg *Message) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	connected, err := r.household.IsConnected(ctx, member.ID)
	if err != nil || !connected {
		return r.sendUnconnectedNotice(ctx, locale, msg)
	}

	return r.sendPrivateRemindersList(ctx, member, member.ID)
}

// sendUnconnectedNotice tells a member in a topic how to connect. When the bot username is known,
// the "Connect with the bot" button opens the private chat and sends /start connect in one tap.
func (r *Router) sendUnconnectedNotice(ctx context.Context, locale string, msg *Message) error {
	opts := &SendOptions{ThreadID: msg.MessageThreadID}
	if r.cfg.BotUsername != "" {
		opts.ReplyMarkup = &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{
			Text: r.i18n.T(locale, "general.connect_button"),
			URL:  "https://t.me/" + r.cfg.BotUsername + "?start=connect",
		}}}}
	}
	_, err := r.client.SendMessage(ctx, msg.Chat.ID, r.i18n.T(locale, "general.unconnected_notice"), opts)
	return err
}

func (r *Router) handlePrivateList(ctx context.Context, member *household.Member, msg *Message) error {
	return r.sendPrivateChoresList(ctx, member, msg.Chat.ID)
}

func (r *Router) sendPrivateChoresList(ctx context.Context, member *household.Member, targetChatID int64) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	res, err := r.chores.ListMyOpen(ctx, member.ID)
	if err != nil {
		return err
	}

	text, markup := r.formatChoresList(locale, res)
	_, err = r.client.SendMessage(ctx, targetChatID, text, &SendOptions{
		ReplyMarkup: markup,
	})
	return err
}

func (r *Router) sendPrivateRemindersList(ctx context.Context, member *household.Member, targetChatID int64) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	res, err := r.reminders.ListMyOpen(ctx, member.ID)
	if err != nil {
		return err
	}

	text, markup := r.formatRemindersList(locale, res)
	_, err = r.client.SendMessage(ctx, targetChatID, text, &SendOptions{
		ReplyMarkup: markup,
	})
	return err
}

// listItem is one entry of a private list, resolved from its domain.
type listItem struct {
	emoji, name, recurrence, callback string
	dueAt                             time.Time
}

// listViewSwitch is the row that moves a private list between chores and reminders.
// The list title already names the current view, so neither button is marked.
func (r *Router) listViewSwitch(locale string) []InlineKeyboardButton {
	return []InlineKeyboardButton{
		{Text: r.i18n.T(locale, "lists.view_chores"), CallbackData: "list:ch"},
		{Text: r.i18n.T(locale, "lists.view_reminders"), CallbackData: "list:re"},
	}
}

// listGroup is a traffic-light day group of a private list. The values are in display order.
type listGroup int

const (
	groupOverdue listGroup = iota
	groupToday
	groupTomorrow
	groupUpcoming
)

// listGroupKeys is the header key of each listGroup.
var listGroupKeys = [...]string{"lists.group_overdue", "lists.group_today", "lists.group_tomorrow", "lists.group_upcoming"}

// listGroupOf assigns a due time to its group. An item earlier today that is already past is overdue.
func (r *Router) listGroupOf(due time.Time) listGroup {
	switch {
	case due.Before(r.clock.Now()):
		return groupOverdue
	case due.Before(r.dayAt(1, timeOfDay{})):
		return groupToday
	case due.Before(r.dayAt(2, timeOfDay{})):
		return groupTomorrow
	}
	return groupUpcoming
}

// renderPrivateList renders the title, then the entries grouped by due day under their headers, one
// unnumbered button per entry in body order, followed by the view switch. The "+N more" line closes the body.
// Items arrive sorted by due time, so each group is contiguous and empty groups never appear.
func (r *Router) renderPrivateList(locale, title string, remaining int, items []listItem) (string, *InlineKeyboardMarkup) {
	var sb strings.Builder
	sb.WriteString(title)
	rows := [][]InlineKeyboardButton{}
	prev := listGroup(-1)
	for _, item := range items {
		sb.WriteString("\n\n")
		group := r.listGroupOf(item.dueAt)
		if group != prev {
			prev = group
			sb.WriteString(r.i18n.T(locale, listGroupKeys[group]) + "\n")
		}
		sb.WriteString(r.renderListEntry(locale, group, item))
		rows = append(rows, []InlineKeyboardButton{
			{Text: fmt.Sprintf("%s %s", item.emoji, item.name), CallbackData: item.callback},
		})
	}
	if remaining > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(r.i18n.T(locale, "chores.remaining_count", remaining))
	}
	rows = append(rows, r.listViewSwitch(locale))
	return sb.String(), &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// renderListEntry renders "emoji name" and, below it, the time line: "╰┈➤ 18:00". Overdue and upcoming
// items put the day in front ("╰┈➤ ayer · 18:00"); recurring items append the recurrence summary.
func (r *Router) renderListEntry(locale string, group listGroup, item listItem) string {
	loc := r.clock.Location()
	when := item.dueAt.In(loc).Format("15:04")
	if group == groupOverdue || group == groupUpcoming {
		when = r.i18n.FormatDay(locale, item.dueAt, r.clock.Now(), loc) + " · " + when
	}
	if item.recurrence != "" {
		when = r.i18n.T(locale, "lists.entry_recurrence", when, item.recurrence)
	}
	return fmt.Sprintf("%s %s\n%s", item.emoji, item.name, r.i18n.T(locale, "lists.entry_detail", when))
}

// listRecurrence returns the recurrence summary, or "" for a one-time definition.
func (r *Router) listRecurrence(locale string, interval *int, unit *string) string {
	if interval == nil || unit == nil {
		return ""
	}
	return r.recurrenceSummary(locale, *interval, *unit, nil)
}

func (r *Router) formatChoresList(locale string, res chores.ListResult) (string, *InlineKeyboardMarkup) {
	if len(res.Occurrences) == 0 {
		return r.i18n.T(locale, "chores.list_empty"), &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{r.listViewSwitch(locale)}}
	}

	items := make([]listItem, 0, len(res.Occurrences))
	for _, occ := range res.Occurrences {
		item := listItem{
			emoji:    "🧹",
			name:     r.i18n.T(locale, i18nKeyChoresFallbackName),
			callback: fmt.Sprintf("ch:d:%d", occ.ID),
			dueAt:    occ.DueAt,
		}
		if occ.Definition != nil {
			item.name = occ.Definition.Name
			item.emoji = occ.Definition.Emoji
			item.recurrence = r.listRecurrence(locale, occ.Definition.RecurrenceInterval, occ.Definition.RecurrenceUnit)
		}
		items = append(items, item)
	}
	return r.renderPrivateList(locale, r.i18n.T(locale, "chores.list_title"), res.RemainingCount, items)
}

func (r *Router) formatRemindersList(locale string, res reminders.ListResult) (string, *InlineKeyboardMarkup) {
	if len(res.Occurrences) == 0 {
		return r.i18n.T(locale, "reminders.list_empty"), &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{r.listViewSwitch(locale)}}
	}

	items := make([]listItem, 0, len(res.Occurrences))
	for _, occ := range res.Occurrences {
		item := listItem{
			emoji:    "⏰",
			name:     r.i18n.T(locale, i18nKeyRemindersFallbackTitle),
			callback: fmt.Sprintf("re:d:%d", occ.ID),
			dueAt:    occ.DueAt,
		}
		if occ.Definition != nil {
			item.name = occ.Definition.Title
			item.emoji = occ.Definition.Emoji
			item.recurrence = r.listRecurrence(locale, occ.Definition.RecurrenceInterval, occ.Definition.RecurrenceUnit)
		}
		items = append(items, item)
	}
	return r.renderPrivateList(locale, r.i18n.T(locale, "reminders.list_title"), res.RemainingCount, items)
}

func (r *Router) handleChoreCallback(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	switch parsed.Kind {
	case CallbackListChores:
		return r.showChoresList(ctx, member, cq, locale)
	case CallbackChoreDetail:
		return r.showChoreDetail(ctx, member, cq, parsed.TargetID, locale)
	case CallbackChoreComplete:
		return r.completeChore(ctx, member, cq, parsed, locale)
	case CallbackChorePostpone:
		return r.postponeChore(ctx, member, cq, parsed, locale)
	}

	return nil
}

// showChoresList edits the message back to the member's open chores.
func (r *Router) showChoresList(ctx context.Context, member *household.Member, cq *CallbackQuery, locale string) error {
	res, err := r.chores.ListMyOpen(ctx, member.ID)
	if err != nil {
		return err
	}
	text, markup := r.formatChoresList(locale, res)
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{
		ReplyMarkup: markup,
	})
}

// showChoreDetail edits the message to one open chore; a stale chore re-renders the list instead.
func (r *Router) showChoreDetail(ctx context.Context, member *household.Member, cq *CallbackQuery, occID int64, locale string) error {
	occ, err := r.chores.GetOccurrence(ctx, occID)
	if err != nil || occ == nil || occ.Status != "open" {
		// Stale detail: re-render current list
		res, _ := r.chores.ListMyOpen(ctx, member.ID)
		text, markup := r.formatChoresList(locale, res)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{
			ReplyMarkup: markup,
		})
	}

	name := r.i18n.T(locale, i18nKeyChoresFallbackName)
	emoji := "🧹"
	recSummary := r.i18n.T(locale, "general.recurrence_one_off")
	if occ.Definition != nil {
		name = occ.Definition.Name
		emoji = occ.Definition.Emoji
		if occ.Definition.RecurrenceInterval != nil && occ.Definition.RecurrenceUnit != nil {
			recSummary = r.recurrenceSummary(locale, *occ.Definition.RecurrenceInterval, *occ.Definition.RecurrenceUnit, nil)
		}
	}

	dueStr := r.formatDateTime(locale, occ.DueAt)
	assigneeName := member.Name
	if assigneeName == "" {
		assigneeName = member.Username
	}

	detailText := r.i18n.T(locale, "chores.detail_view", emoji, name, dueStr, assigneeName, recSummary)
	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: r.i18n.T(locale, "buttons.done"), CallbackData: fmt.Sprintf("ch:c:%d", occ.ID)},
				{Text: r.i18n.T(locale, "buttons.postpone"), CallbackData: fmt.Sprintf("ch:p:%d", occ.ID)},
			},
			{
				{Text: r.i18n.T(locale, i18nKeyButtonBack), CallbackData: "list:ch"},
			},
		},
	}

	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, detailText, &EditOptions{
		ReplyMarkup: markup,
	})
}

func (r *Router) completeChore(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback, locale string) error {
	now := r.clock.Now().UTC()
	occ, _, err := r.chores.Complete(ctx, parsed.ActorID, parsed.TargetID, now)
	if err != nil {
		if errors.Is(err, chores.ErrNotAssignee) {
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, i18nKeyChoresNotAssignee), true)
			return nil
		}
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.userErrorText(locale, err), true)
		return nil
	}

	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.done"), false)

	// Post group topic update
	name := r.i18n.T(r.cfg.DefaultLocale, i18nKeyChoresFallbackName)
	emoji := "🧹"
	if occ != nil && occ.Definition != nil {
		name = occ.Definition.Name
		emoji = occ.Definition.Emoji
	}
	actorName := member.Name
	if actorName == "" {
		actorName = member.Username
	}
	groupMsg := r.i18n.T(r.cfg.DefaultLocale, "group.chore_completed", emoji, name, actorName)
	_, _ = r.client.SendMessage(ctx, r.cfg.ChatID, groupMsg, &SendOptions{
		ThreadID: r.cfg.ChoresTopicID,
	})

	// If this is in a private chat message, re-render the list view
	return r.refreshChoresList(ctx, member, cq, locale)
}

func (r *Router) postponeChore(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback, locale string) error {
	if parsed.Payload == "" {
		return r.showChorePostponePresets(ctx, cq, parsed.TargetID, locale)
	}

	// Preset picked
	if parsed.Payload == "custom" {
		return r.startTypedPostpone(ctx, member, cq, "chore_postpone_typed", parsed.TargetID, locale)
	}

	newDue := r.chorePostponeTime(parsed.Payload)
	occ, err := r.chores.Postpone(ctx, parsed.ActorID, parsed.TargetID, newDue)
	if err != nil {
		return r.answerChorePostponeError(ctx, member, cq, locale, err)
	}

	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "chores.postponed_notice"), false)

	r.postChorePostponed(ctx, member, occ, newDue)

	// Return to list view
	return r.refreshChoresList(ctx, member, cq, locale)
}

// showChorePostponePresets edits the message to the postponement presets for one chore.
func (r *Router) showChorePostponePresets(ctx context.Context, cq *CallbackQuery, occID int64, locale string) error {
	rows := [][]InlineKeyboardButton{
		{
			{Text: r.i18n.T(locale, "buttons.postpone_30m"), CallbackData: fmt.Sprintf("ch:p:%d:30m", occID)},
			{Text: r.i18n.T(locale, "buttons.postpone_1h"), CallbackData: fmt.Sprintf("ch:p:%d:1h", occID)},
			{Text: r.i18n.T(locale, "buttons.postpone_3h"), CallbackData: fmt.Sprintf("ch:p:%d:3h", occID)},
		},
		{
			{Text: r.i18n.T(locale, "buttons.postpone_tomorrow"), CallbackData: fmt.Sprintf("ch:p:%d:tom", occID)},
			{Text: r.i18n.T(locale, i18nKeyButtonBack), CallbackData: fmt.Sprintf("ch:d:%d", occID)},
		},
		{
			{Text: r.i18n.T(locale, "buttons.postpone_custom"), CallbackData: fmt.Sprintf("ch:p:%d:custom", occID)},
		},
	}
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, r.i18n.T(locale, "general.postpone_prompt"), &EditOptions{
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

// chorePostponeTime returns the due time for a chore postponement preset.
func (r *Router) chorePostponeTime(preset string) time.Time {
	now := r.clock.Now().In(r.clock.Location())
	switch preset {
	case "30m":
		return now.Add(30 * time.Minute)
	case "1h":
		return now.Add(1 * time.Hour)
	case "3h":
		return now.Add(3 * time.Hour)
	case "tom":
		return r.dayAt(1, r.defaultTime)
	default:
		return now.Add(1 * time.Hour)
	}
}

// answerChorePostponeError tells the member why a chore could not be postponed.
func (r *Router) answerChorePostponeError(ctx context.Context, member *household.Member, cq *CallbackQuery, locale string, err error) error {
	if errors.Is(err, chores.ErrNotAssignee) {
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, i18nKeyChoresNotAssignee), true)
		return nil
	}
	if errors.Is(err, chores.ErrOccurrenceResolved) {
		// Stale button: tell the user and show the current list instead of the dead detail.
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.userErrorText(locale, err), true)
		return r.refreshChoresList(ctx, member, cq, locale)
	}
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.userErrorText(locale, err), true)
	return nil
}

func (r *Router) handleReminderCallback(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	switch parsed.Kind {
	case CallbackListReminders:
		return r.showRemindersList(ctx, member, cq, locale)
	case CallbackReminderDetail:
		return r.showReminderDetail(ctx, member, cq, parsed.TargetID, locale)
	case CallbackReminderComplete:
		return r.completeReminder(ctx, member, cq, parsed, locale)
	case CallbackReminderPostpone:
		return r.postponeReminder(ctx, member, cq, parsed, locale)
	}

	return nil
}

// showRemindersList edits the message back to the member's open reminders.
func (r *Router) showRemindersList(ctx context.Context, member *household.Member, cq *CallbackQuery, locale string) error {
	res, err := r.reminders.ListMyOpen(ctx, member.ID)
	if err != nil {
		return err
	}
	text, markup := r.formatRemindersList(locale, res)
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{
		ReplyMarkup: markup,
	})
}

// showReminderDetail edits the message to one open reminder; a stale reminder re-renders the list instead.
func (r *Router) showReminderDetail(ctx context.Context, member *household.Member, cq *CallbackQuery, occID int64, locale string) error {
	occ, err := r.reminders.GetOccurrence(ctx, occID)
	if err != nil || occ == nil || occ.Status != "open" {
		res, _ := r.reminders.ListMyOpen(ctx, member.ID)
		text, markup := r.formatRemindersList(locale, res)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{
			ReplyMarkup: markup,
		})
	}

	title := r.i18n.T(locale, i18nKeyRemindersFallbackTitle)
	emoji := "⏰"
	recSummary := r.i18n.T(locale, "general.recurrence_one_off")
	if occ.Definition != nil {
		title = occ.Definition.Title
		emoji = occ.Definition.Emoji
		if occ.Definition.RecurrenceInterval != nil && occ.Definition.RecurrenceUnit != nil {
			recSummary = r.recurrenceSummary(locale, *occ.Definition.RecurrenceInterval, *occ.Definition.RecurrenceUnit, nil)
		}
	}

	dueStr := r.formatDateTime(locale, occ.DueAt)
	targetName := member.Name
	if targetName == "" {
		targetName = member.Username
	}

	detailText := r.i18n.T(locale, "reminders.detail_view", emoji, title, dueStr, targetName, recSummary)
	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: r.i18n.T(locale, "buttons.done"), CallbackData: fmt.Sprintf("re:c:%d", occ.ID)},
				{Text: r.i18n.T(locale, "buttons.postpone"), CallbackData: fmt.Sprintf("re:p:%d", occ.ID)},
			},
			{
				{Text: r.i18n.T(locale, i18nKeyButtonBack), CallbackData: "list:re"},
			},
		},
	}

	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, detailText, &EditOptions{
		ReplyMarkup: markup,
	})
}

func (r *Router) completeReminder(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback, locale string) error {
	now := r.clock.Now().UTC()
	occ, _, err := r.reminders.Complete(ctx, parsed.ActorID, parsed.TargetID, now)
	if err != nil {
		if errors.Is(err, reminders.ErrNotTarget) {
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, i18nKeyRemindersNotTarget), true)
			return nil
		}
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.userErrorText(locale, err), true)
		return nil
	}

	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.done"), false)

	title := r.i18n.T(r.cfg.DefaultLocale, i18nKeyRemindersFallbackTitle)
	emoji := "⏰"
	if occ != nil && occ.Definition != nil {
		title = occ.Definition.Title
		emoji = occ.Definition.Emoji
	}
	actorName := member.Name
	if actorName == "" {
		actorName = member.Username
	}
	groupMsg := r.i18n.T(r.cfg.DefaultLocale, "group.reminder_completed", emoji, title, actorName)
	_, _ = r.client.SendMessage(ctx, r.cfg.ChatID, groupMsg, &SendOptions{
		ThreadID: r.cfg.RemindersTopicID,
	})

	return r.refreshRemindersList(ctx, member, cq, locale)
}

func (r *Router) postponeReminder(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback, locale string) error {
	if parsed.Payload == "" {
		return r.showReminderPostponePresets(ctx, cq, parsed.TargetID, locale)
	}

	if parsed.Payload == "custom" {
		return r.startTypedPostpone(ctx, member, cq, "reminder_postpone_typed", parsed.TargetID, locale)
	}

	newDue := r.reminderPostponeTime(parsed.Payload)
	occ, err := r.reminders.Postpone(ctx, parsed.ActorID, parsed.TargetID, newDue)
	if err != nil {
		return r.answerReminderPostponeError(ctx, member, cq, locale, err)
	}

	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "reminders.postponed_notice"), false)

	r.postReminderPostponed(ctx, member, occ, newDue)

	return r.refreshRemindersList(ctx, member, cq, locale)
}

// showReminderPostponePresets edits the message to the postponement presets for one reminder.
func (r *Router) showReminderPostponePresets(ctx context.Context, cq *CallbackQuery, occID int64, locale string) error {
	rows := [][]InlineKeyboardButton{
		{
			{Text: r.i18n.T(locale, "buttons.postpone_15m"), CallbackData: fmt.Sprintf("re:p:%d:15m", occID)},
			{Text: r.i18n.T(locale, "buttons.postpone_1h"), CallbackData: fmt.Sprintf("re:p:%d:1h", occID)},
			{Text: r.i18n.T(locale, "buttons.postpone_3h"), CallbackData: fmt.Sprintf("re:p:%d:3h", occID)},
		},
		{
			{Text: r.i18n.T(locale, "buttons.postpone_tomorrow"), CallbackData: fmt.Sprintf("re:p:%d:tom", occID)},
			{Text: r.i18n.T(locale, i18nKeyButtonBack), CallbackData: fmt.Sprintf("re:d:%d", occID)},
		},
		{
			{Text: r.i18n.T(locale, "buttons.postpone_custom"), CallbackData: fmt.Sprintf("re:p:%d:custom", occID)},
		},
	}
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, r.i18n.T(locale, "general.postpone_prompt"), &EditOptions{
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

// reminderPostponeTime returns the due time for a reminder postponement preset.
func (r *Router) reminderPostponeTime(preset string) time.Time {
	now := r.clock.Now().In(r.clock.Location())
	switch preset {
	case "15m":
		return now.Add(15 * time.Minute)
	case "1h":
		return now.Add(1 * time.Hour)
	case "3h":
		return now.Add(3 * time.Hour)
	case "tom":
		return r.dayAt(1, r.defaultTime)
	default:
		return now.Add(1 * time.Hour)
	}
}

// answerReminderPostponeError tells the member why a reminder could not be postponed.
func (r *Router) answerReminderPostponeError(ctx context.Context, member *household.Member, cq *CallbackQuery, locale string, err error) error {
	if errors.Is(err, reminders.ErrNotTarget) {
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, i18nKeyRemindersNotTarget), true)
		return nil
	}
	if errors.Is(err, reminders.ErrOccurrenceResolved) {
		// Stale button: tell the user and show the current list instead of the dead detail.
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.userErrorText(locale, err), true)
		return r.refreshRemindersList(ctx, member, cq, locale)
	}
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.userErrorText(locale, err), true)
	return nil
}

// refreshChoresList edits a private list message back to the member's current chore list.
func (r *Router) refreshChoresList(ctx context.Context, member *household.Member, cq *CallbackQuery, locale string) error {
	if cq.Message == nil || cq.Message.Chat.Type != "private" {
		return nil
	}
	res, _ := r.chores.ListMyOpen(ctx, member.ID)
	text, markup := r.formatChoresList(locale, res)
	return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{ReplyMarkup: markup})
}

// refreshRemindersList edits a private list message back to the member's current reminder list.
func (r *Router) refreshRemindersList(ctx context.Context, member *household.Member, cq *CallbackQuery, locale string) error {
	if cq.Message == nil || cq.Message.Chat.Type != "private" {
		return nil
	}
	res, _ := r.reminders.ListMyOpen(ctx, member.ID)
	text, markup := r.formatRemindersList(locale, res)
	return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{ReplyMarkup: markup})
}

// postChorePostponed announces a postponement in the Chores topic, in the default locale.
// It runs after the change is committed; a failed post never rolls anything back.
func (r *Router) postChorePostponed(ctx context.Context, member *household.Member, occ *chores.Occurrence, newDue time.Time) {
	name := r.i18n.T(r.cfg.DefaultLocale, i18nKeyChoresFallbackName)
	emoji := "🧹"
	if occ != nil && occ.Definition != nil {
		name = occ.Definition.Name
		emoji = occ.Definition.Emoji
	}
	actorName := member.Name
	if actorName == "" {
		actorName = member.Username
	}
	dueStr := r.formatDateTime(r.cfg.DefaultLocale, newDue)
	groupMsg := r.i18n.T(r.cfg.DefaultLocale, "group.chore_postponed", emoji, name, actorName, dueStr)
	_, _ = r.client.SendMessage(ctx, r.cfg.ChatID, groupMsg, &SendOptions{ThreadID: r.cfg.ChoresTopicID})
}

// postReminderPostponed announces a postponement in the Reminders topic, in the default locale.
func (r *Router) postReminderPostponed(ctx context.Context, member *household.Member, occ *reminders.Occurrence, newDue time.Time) {
	title := r.i18n.T(r.cfg.DefaultLocale, i18nKeyRemindersFallbackTitle)
	emoji := "⏰"
	if occ != nil && occ.Definition != nil {
		title = occ.Definition.Title
		emoji = occ.Definition.Emoji
	}
	actorName := member.Name
	if actorName == "" {
		actorName = member.Username
	}
	dueStr := r.formatDateTime(r.cfg.DefaultLocale, newDue)
	groupMsg := r.i18n.T(r.cfg.DefaultLocale, "group.reminder_postponed", emoji, title, actorName, dueStr)
	_, _ = r.client.SendMessage(ctx, r.cfg.ChatID, groupMsg, &SendOptions{ThreadID: r.cfg.RemindersTopicID})
}

// postponePayload is the flow payload while waiting for a typed postponement time.
type postponePayload struct {
	OccID int64 `json:"occ_id"`
}

// startTypedPostpone asks the user to type a new date/time for an occurrence. The target is kept
// in persisted flow state keyed by (user, chat, topic) like every other multi-step interaction.
func (r *Router) startTypedPostpone(ctx context.Context, member *household.Member, cq *CallbackQuery, step string, occID int64, locale string) error {
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	if cq.Message == nil {
		return nil
	}
	key := flow.Key{UserID: member.ID, ChatID: cq.Message.Chat.ID, ThreadID: cq.Message.MessageThreadID}
	if err := r.flow.Set(ctx, key, step, postponePayload{OccID: occID}, r.cfg.FlowTTL); err != nil {
		r.logger.Error("failed to store postpone flow state", "err", err)
		_, sendErr := r.client.SendMessage(ctx, cq.Message.Chat.ID, r.i18n.T(locale, "general.internal_error"), &SendOptions{ThreadID: cq.Message.MessageThreadID})
		return sendErr
	}
	markup := &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{
		{Text: r.i18n.T(locale, i18nKeyButtonCancel), CallbackData: "fl:c"},
	}}}
	_, err := r.client.SendMessage(ctx, cq.Message.Chat.ID, r.i18n.T(locale, "general.postpone_typed_prompt"), &SendOptions{
		ThreadID:    cq.Message.MessageThreadID,
		ReplyMarkup: markup,
	})
	return err
}

// handleTypedPostponeText applies a typed postponement through the same domain commands as the presets.
func (r *Router) handleTypedPostponeText(ctx context.Context, member *household.Member, msg *Message, state *flow.State, locale string) error {
	var payload postponePayload
	if err := json.Unmarshal(state.Payload, &payload); err != nil {
		_ = r.flow.Clear(ctx, state.Key)
		return nil
	}
	reply := func(text string) error {
		_, err := r.client.SendMessage(ctx, msg.Chat.ID, text, &SendOptions{ThreadID: msg.MessageThreadID})
		return err
	}

	newDue, err := parseDateTime(strings.TrimSpace(msg.Text), r.clock)
	if err != nil {
		guidance := r.i18n.T(locale, "general.invalid_date_format")
		if isPastDue(err) {
			guidance = r.i18n.T(locale, "general.past_due_time") + " " + guidance
		}
		return reply(guidance) // keep the flow: the user can type again or cancel
	}

	_ = r.flow.Clear(ctx, state.Key)
	if strings.HasPrefix(state.Step, "chore_") {
		occ, err := r.chores.Postpone(ctx, member.ID, payload.OccID, newDue)
		if err != nil {
			return reply(r.postponeErrorText(locale, err))
		}
		r.postChorePostponed(ctx, member, occ, newDue)
	} else {
		occ, err := r.reminders.Postpone(ctx, member.ID, payload.OccID, newDue)
		if err != nil {
			return reply(r.postponeErrorText(locale, err))
		}
		r.postReminderPostponed(ctx, member, occ, newDue)
	}
	return reply(r.i18n.T(locale, "general.postponed_until", r.formatDateTime(locale, newDue)))
}

// postponeErrorText is userErrorText plus the ownership errors that carry their own wording.
func (r *Router) postponeErrorText(locale string, err error) string {
	switch {
	case errors.Is(err, chores.ErrNotAssignee):
		return r.i18n.T(locale, i18nKeyChoresNotAssignee)
	case errors.Is(err, reminders.ErrNotTarget):
		return r.i18n.T(locale, i18nKeyRemindersNotTarget)
	}
	return r.userErrorText(locale, err)
}
