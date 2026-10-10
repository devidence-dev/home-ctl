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
		notice := r.i18n.T(locale, "general.unconnected_notice")
		_, err := r.client.SendMessage(ctx, msg.Chat.ID, notice, &SendOptions{
			ThreadID: msg.MessageThreadID,
		})
		return err
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
		notice := r.i18n.T(locale, "general.unconnected_notice")
		_, err := r.client.SendMessage(ctx, msg.Chat.ID, notice, &SendOptions{
			ThreadID: msg.MessageThreadID,
		})
		return err
	}

	return r.sendPrivateRemindersList(ctx, member, member.ID)
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

// listViewSwitch is the row that moves a private list between chores ("ch") and reminders ("re").
// The current view is marked; its button re-renders the same view.
func (r *Router) listViewSwitch(locale, view string) []InlineKeyboardButton {
	chores := r.i18n.T(locale, "lists.view_chores")
	reminders := r.i18n.T(locale, "lists.view_reminders")
	if view == "ch" {
		chores = "• " + chores
	} else {
		reminders = "• " + reminders
	}
	return []InlineKeyboardButton{
		{Text: chores, CallbackData: "list:ch"},
		{Text: reminders, CallbackData: "list:re"},
	}
}

// renderPrivateList renders the title, every entry in the body and one numbered button per entry,
// followed by the view switch. The "+N more" line closes the body.
func (r *Router) renderPrivateList(locale, view, title string, remaining int, items []listItem) (string, *InlineKeyboardMarkup) {
	var sb strings.Builder
	sb.WriteString(title)
	rows := [][]InlineKeyboardButton{}
	for i, item := range items {
		n := i + 1
		sb.WriteString("\n\n")
		sb.WriteString(r.renderListEntry(locale, n, item))
		rows = append(rows, []InlineKeyboardButton{
			{Text: fmt.Sprintf("%d. %s %s", n, item.emoji, item.name), CallbackData: item.callback},
		})
	}
	if remaining > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(r.i18n.T(locale, "chores.remaining_count", remaining))
	}
	rows = append(rows, r.listViewSwitch(locale, view))
	return sb.String(), &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// renderListEntry renders "n. emoji name" and, below it, the due date with recurrence or the overdue marker.
func (r *Router) renderListEntry(locale string, n int, item listItem) string {
	date := r.formatDateTime(locale, item.dueAt)
	detail := r.i18n.T(locale, "lists.due", date)
	switch {
	case item.dueAt.Before(r.clock.Now()):
		detail = r.i18n.T(locale, "lists.overdue", date)
	case item.recurrence != "":
		detail = r.i18n.T(locale, "lists.due_recurring", date, item.recurrence)
	}
	return fmt.Sprintf("%d. %s %s\n%s", n, item.emoji, item.name, detail)
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
		return r.i18n.T(locale, "chores.list_empty"), &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{r.listViewSwitch(locale, "ch")}}
	}

	items := make([]listItem, 0, len(res.Occurrences))
	for _, occ := range res.Occurrences {
		item := listItem{
			emoji:    "🧹",
			name:     r.i18n.T(locale, "chores.fallback_name"),
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
	return r.renderPrivateList(locale, "ch", r.i18n.T(locale, "chores.list_title"), res.RemainingCount, items)
}

func (r *Router) formatRemindersList(locale string, res reminders.ListResult) (string, *InlineKeyboardMarkup) {
	if len(res.Occurrences) == 0 {
		return r.i18n.T(locale, "reminders.list_empty"), &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{r.listViewSwitch(locale, "re")}}
	}

	items := make([]listItem, 0, len(res.Occurrences))
	for _, occ := range res.Occurrences {
		item := listItem{
			emoji:    "⏰",
			name:     r.i18n.T(locale, "reminders.fallback_title"),
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
	return r.renderPrivateList(locale, "re", r.i18n.T(locale, "reminders.list_title"), res.RemainingCount, items)
}

func (r *Router) handleChoreCallback(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	switch parsed.Kind {
	case CallbackListChores:
		// Back to chores list
		res, err := r.chores.ListMyOpen(ctx, member.ID)
		if err != nil {
			return err
		}
		text, markup := r.formatChoresList(locale, res)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{
			ReplyMarkup: markup,
		})

	case CallbackChoreDetail:
		occ, err := r.chores.GetOccurrence(ctx, parsed.TargetID)
		if err != nil || occ == nil || occ.Status != "open" {
			// Stale detail: re-render current list
			res, _ := r.chores.ListMyOpen(ctx, member.ID)
			text, markup := r.formatChoresList(locale, res)
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
			return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{
				ReplyMarkup: markup,
			})
		}

		name := r.i18n.T(locale, "chores.fallback_name")
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
					{Text: r.i18n.T(locale, "buttons.back"), CallbackData: "list:ch"},
				},
			},
		}

		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, detailText, &EditOptions{
			ReplyMarkup: markup,
		})

	case CallbackChoreComplete:
		now := r.clock.Now().UTC()
		occ, _, err := r.chores.Complete(ctx, parsed.ActorID, parsed.TargetID, now)
		if err != nil {
			if errors.Is(err, chores.ErrNotAssignee) {
				_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "chores.not_assignee"), true)
				return nil
			}
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.userErrorText(locale, err), true)
			return nil
		}

		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.done"), false)

		// Post group topic update
		name := r.i18n.T(r.cfg.DefaultLocale, "chores.fallback_name")
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
		if cq.Message != nil && cq.Message.Chat.Type == "private" {
			res, _ := r.chores.ListMyOpen(ctx, member.ID)
			text, markup := r.formatChoresList(locale, res)
			return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{
				ReplyMarkup: markup,
			})
		}
		return nil

	case CallbackChorePostpone:
		if parsed.Payload == "" {
			// Show postponement presets menu
			rows := [][]InlineKeyboardButton{
				{
					{Text: r.i18n.T(locale, "buttons.postpone_30m"), CallbackData: fmt.Sprintf("ch:p:%d:30m", parsed.TargetID)},
					{Text: r.i18n.T(locale, "buttons.postpone_1h"), CallbackData: fmt.Sprintf("ch:p:%d:1h", parsed.TargetID)},
					{Text: r.i18n.T(locale, "buttons.postpone_3h"), CallbackData: fmt.Sprintf("ch:p:%d:3h", parsed.TargetID)},
				},
				{
					{Text: r.i18n.T(locale, "buttons.postpone_tomorrow"), CallbackData: fmt.Sprintf("ch:p:%d:tom", parsed.TargetID)},
					{Text: r.i18n.T(locale, "buttons.back"), CallbackData: fmt.Sprintf("ch:d:%d", parsed.TargetID)},
				},
				{
					{Text: r.i18n.T(locale, "buttons.postpone_custom"), CallbackData: fmt.Sprintf("ch:p:%d:custom", parsed.TargetID)},
				},
			}
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
			return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, r.i18n.T(locale, "general.postpone_prompt"), &EditOptions{
				ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
			})
		}

		// Preset picked
		if parsed.Payload == "custom" {
			return r.startTypedPostpone(ctx, member, cq, "chore_postpone_typed", parsed.TargetID, locale)
		}

		loc := r.clock.Location()
		now := r.clock.Now().In(loc)
		var newDue time.Time
		switch parsed.Payload {
		case "30m":
			newDue = now.Add(30 * time.Minute)
		case "1h":
			newDue = now.Add(1 * time.Hour)
		case "3h":
			newDue = now.Add(3 * time.Hour)
		case "tom":
			newDue = r.dayAt(1, r.defaultTime)
		default:
			newDue = now.Add(1 * time.Hour)
		}

		occ, err := r.chores.Postpone(ctx, parsed.ActorID, parsed.TargetID, newDue)
		if err != nil {
			if errors.Is(err, chores.ErrNotAssignee) {
				_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "chores.not_assignee"), true)
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

		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "chores.postponed_notice"), false)

		r.postChorePostponed(ctx, member, occ, newDue)

		// Return to list view
		if cq.Message != nil && cq.Message.Chat.Type == "private" {
			res, _ := r.chores.ListMyOpen(ctx, member.ID)
			text, markup := r.formatChoresList(locale, res)
			return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{
				ReplyMarkup: markup,
			})
		}
	}

	return nil
}

func (r *Router) handleReminderCallback(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	switch parsed.Kind {
	case CallbackListReminders:
		res, err := r.reminders.ListMyOpen(ctx, member.ID)
		if err != nil {
			return err
		}
		text, markup := r.formatRemindersList(locale, res)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{
			ReplyMarkup: markup,
		})

	case CallbackReminderDetail:
		occ, err := r.reminders.GetOccurrence(ctx, parsed.TargetID)
		if err != nil || occ == nil || occ.Status != "open" {
			res, _ := r.reminders.ListMyOpen(ctx, member.ID)
			text, markup := r.formatRemindersList(locale, res)
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
			return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{
				ReplyMarkup: markup,
			})
		}

		title := r.i18n.T(locale, "reminders.fallback_title")
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
					{Text: r.i18n.T(locale, "buttons.back"), CallbackData: "list:re"},
				},
			},
		}

		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, detailText, &EditOptions{
			ReplyMarkup: markup,
		})

	case CallbackReminderComplete:
		now := r.clock.Now().UTC()
		occ, _, err := r.reminders.Complete(ctx, parsed.ActorID, parsed.TargetID, now)
		if err != nil {
			if errors.Is(err, reminders.ErrNotTarget) {
				_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "reminders.not_target"), true)
				return nil
			}
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.userErrorText(locale, err), true)
			return nil
		}

		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.done"), false)

		title := r.i18n.T(r.cfg.DefaultLocale, "reminders.fallback_title")
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

		if cq.Message != nil && cq.Message.Chat.Type == "private" {
			res, _ := r.reminders.ListMyOpen(ctx, member.ID)
			text, markup := r.formatRemindersList(locale, res)
			return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{
				ReplyMarkup: markup,
			})
		}
		return nil

	case CallbackReminderPostpone:
		if parsed.Payload == "" {
			rows := [][]InlineKeyboardButton{
				{
					{Text: r.i18n.T(locale, "buttons.postpone_15m"), CallbackData: fmt.Sprintf("re:p:%d:15m", parsed.TargetID)},
					{Text: r.i18n.T(locale, "buttons.postpone_1h"), CallbackData: fmt.Sprintf("re:p:%d:1h", parsed.TargetID)},
					{Text: r.i18n.T(locale, "buttons.postpone_3h"), CallbackData: fmt.Sprintf("re:p:%d:3h", parsed.TargetID)},
				},
				{
					{Text: r.i18n.T(locale, "buttons.postpone_tomorrow"), CallbackData: fmt.Sprintf("re:p:%d:tom", parsed.TargetID)},
					{Text: r.i18n.T(locale, "buttons.back"), CallbackData: fmt.Sprintf("re:d:%d", parsed.TargetID)},
				},
				{
					{Text: r.i18n.T(locale, "buttons.postpone_custom"), CallbackData: fmt.Sprintf("re:p:%d:custom", parsed.TargetID)},
				},
			}
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
			return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, r.i18n.T(locale, "general.postpone_prompt"), &EditOptions{
				ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
			})
		}

		if parsed.Payload == "custom" {
			return r.startTypedPostpone(ctx, member, cq, "reminder_postpone_typed", parsed.TargetID, locale)
		}

		loc := r.clock.Location()
		now := r.clock.Now().In(loc)
		var newDue time.Time
		switch parsed.Payload {
		case "15m":
			newDue = now.Add(15 * time.Minute)
		case "1h":
			newDue = now.Add(1 * time.Hour)
		case "3h":
			newDue = now.Add(3 * time.Hour)
		case "tom":
			newDue = r.dayAt(1, r.defaultTime)
		default:
			newDue = now.Add(1 * time.Hour)
		}

		occ, err := r.reminders.Postpone(ctx, parsed.ActorID, parsed.TargetID, newDue)
		if err != nil {
			if errors.Is(err, reminders.ErrNotTarget) {
				_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "reminders.not_target"), true)
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

		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "reminders.postponed_notice"), false)

		r.postReminderPostponed(ctx, member, occ, newDue)

		if cq.Message != nil && cq.Message.Chat.Type == "private" {
			res, _ := r.reminders.ListMyOpen(ctx, member.ID)
			text, markup := r.formatRemindersList(locale, res)
			return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{
				ReplyMarkup: markup,
			})
		}
	}

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
	name := r.i18n.T(r.cfg.DefaultLocale, "chores.fallback_name")
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
	title := r.i18n.T(r.cfg.DefaultLocale, "reminders.fallback_title")
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
		{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
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
		return r.i18n.T(locale, "chores.not_assignee")
	case errors.Is(err, reminders.ErrNotTarget):
		return r.i18n.T(locale, "reminders.not_target")
	}
	return r.userErrorText(locale, err)
}
