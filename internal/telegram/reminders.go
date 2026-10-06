package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/px1/home-ctl/internal/flow"
	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/recurrence"
)

type reminderFlowPayload struct {
	DefID              int64     `json:"def_id,omitempty"`
	Title              string    `json:"title,omitempty"`
	Emoji              string    `json:"emoji,omitempty"`
	TargetUserID       int64     `json:"target_user_id,omitempty"`
	TargetUserName     string    `json:"target_user_name,omitempty"`
	DueAt              time.Time `json:"due_at,omitempty"`
	RecurrenceInterval *int      `json:"recurrence_interval,omitempty"`
	RecurrenceUnit     *string   `json:"recurrence_unit,omitempty"`
	RecurrenceAnchor   *string   `json:"recurrence_anchor,omitempty"`
}

func (r *Router) handleRemindersAdd(ctx context.Context, member *household.Member, msg *Message) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	flowKey := flow.Key{
		UserID:   member.ID,
		ChatID:   msg.Chat.ID,
		ThreadID: msg.MessageThreadID,
	}

	_ = r.flow.Set(ctx, flowKey, "reminder_title", reminderFlowPayload{}, r.cfg.FlowTTL)
	prompt := r.i18n.T(locale, "reminders.prompt_title")
	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"}},
		},
	}
	_, err := r.client.SendMessage(ctx, msg.Chat.ID, prompt, &SendOptions{
		ThreadID:    msg.MessageThreadID,
		ReplyMarkup: markup,
	})
	return err
}

func (r *Router) promptReminderTarget(ctx context.Context, chatID, threadID int64, locale string, _ reminderFlowPayload) error {
	members, err := r.household.ListActiveMembers(ctx)
	if err != nil {
		return err
	}

	var rows [][]InlineKeyboardButton
	for _, m := range members {
		name := m.Name
		if name == "" {
			name = m.Username
		}
		rows = append(rows, []InlineKeyboardButton{
			{Text: name, CallbackData: fmt.Sprintf("fl:usr:%d", m.ID)},
		})
	}
	rows = append(rows, []InlineKeyboardButton{
		{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
	})

	prompt := r.i18n.T(locale, "reminders.prompt_target")
	_, err = r.client.SendMessage(ctx, chatID, prompt, &SendOptions{
		ThreadID:    threadID,
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

func (r *Router) promptReminderDueDate(ctx context.Context, chatID, threadID int64, locale string, payload reminderFlowPayload) error {
	rows := r.dueDateRows(locale)

	prompt := r.i18n.T(locale, "reminders.prompt_due_date", payload.Title)
	_, err := r.client.SendMessage(ctx, chatID, prompt, &SendOptions{
		ThreadID:    threadID,
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

func (r *Router) promptReminderRecurrence(ctx context.Context, chatID, threadID int64, locale string, payload reminderFlowPayload) error {
	rows := [][]InlineKeyboardButton{
		{
			{Text: r.i18n.T(locale, "buttons.rec_none"), CallbackData: "fl:rec:none"},
			{Text: r.i18n.T(locale, "buttons.rec_daily"), CallbackData: "fl:rec:daily"},
		},
		{
			{Text: r.i18n.T(locale, "buttons.rec_weekly"), CallbackData: "fl:rec:weekly"},
			{Text: r.i18n.T(locale, "buttons.rec_monthly"), CallbackData: "fl:rec:monthly"},
		},
		{
			{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
		},
	}

	prompt := r.i18n.T(locale, "reminders.prompt_recurrence", payload.Title)
	_, err := r.client.SendMessage(ctx, chatID, prompt, &SendOptions{
		ThreadID:    threadID,
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

func (r *Router) promptReminderAnchor(ctx context.Context, chatID, threadID int64, locale string, payload reminderFlowPayload) error {
	rows := [][]InlineKeyboardButton{
		{
			{Text: r.i18n.T(locale, "buttons.anchor_scheduled"), CallbackData: "fl:anc:scheduled"},
			{Text: r.i18n.T(locale, "buttons.anchor_completion"), CallbackData: "fl:anc:completion"},
		},
		{
			{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
		},
	}

	prompt := r.i18n.T(locale, "reminders.prompt_anchor", payload.Title)
	_, err := r.client.SendMessage(ctx, chatID, prompt, &SendOptions{
		ThreadID:    threadID,
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

func (r *Router) promptReminderConfirm(ctx context.Context, chatID, threadID int64, locale string, payload reminderFlowPayload) error {
	dueStr := r.formatDateTime(locale, payload.DueAt)

	prompt := r.i18n.T(locale, "reminders.prompt_confirm", payload.Title, payload.TargetUserName, dueStr)
	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: r.i18n.T(locale, "buttons.confirm"), CallbackData: "fl:ok"},
				{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
			},
		},
	}

	_, err := r.client.SendMessage(ctx, chatID, prompt, &SendOptions{
		ThreadID:    threadID,
		ReplyMarkup: markup,
	})
	return err
}

func (r *Router) handleRemindersFlowText(ctx context.Context, member *household.Member, msg *Message, state *flow.State) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	flowKey := state.Key
	text := strings.TrimSpace(msg.Text)

	var payload reminderFlowPayload
	_ = json.Unmarshal(state.Payload, &payload)

	switch state.Step {
	case "reminder_title":
		if text == "" {
			return nil
		}
		payload.Title = text
		payload.Emoji = "⏰"
		_ = r.flow.Set(ctx, flowKey, "reminder_target", payload, r.cfg.FlowTTL)
		return r.promptReminderTarget(ctx, msg.Chat.ID, msg.MessageThreadID, locale, payload)

	case "reminder_typed_due":
		dueAt, err := parseDateTime(text, r.clock)
		if err != nil {
			guidance := r.i18n.T(locale, "general.invalid_date_format")
			if isPastDue(err) {
				guidance = r.i18n.T(locale, "general.past_due_time") + " " + guidance
			}
			_, _ = r.client.SendMessage(ctx, msg.Chat.ID, guidance, &SendOptions{
				ThreadID: msg.MessageThreadID,
			})
			return nil
		}

		payload.DueAt = dueAt
		_ = r.flow.Set(ctx, flowKey, "reminder_recurrence", payload, r.cfg.FlowTTL)
		return r.promptReminderRecurrence(ctx, msg.Chat.ID, msg.MessageThreadID, locale, payload)
	}

	return nil
}

func (r *Router) handleRemindersFlowCallback(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	flowKey := state.Key
	var payload reminderFlowPayload
	_ = json.Unmarshal(state.Payload, &payload)

	chatID := cq.Message.Chat.ID
	threadID := cq.Message.MessageThreadID

	switch parsed.Kind {
	case CallbackFlowCancel:
		_ = r.flow.Clear(ctx, flowKey)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.cancelled"), false)
		_, err := r.client.SendMessage(ctx, chatID, r.i18n.T(locale, "reminders.flow_cancelled"), &SendOptions{
			ThreadID: threadID,
		})
		return err

	case CallbackFlowUser:
		payload.TargetUserID = parsed.TargetID
		m, err := r.household.Authorize(ctx, parsed.TargetID)
		if err == nil && m != nil {
			payload.TargetUserName = m.Name
			if payload.TargetUserName == "" {
				payload.TargetUserName = m.Username
			}
		}
		_ = r.flow.Set(ctx, flowKey, "reminder_due", payload, r.cfg.FlowTTL)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return r.promptReminderDueDate(ctx, chatID, threadID, locale, payload)

	case CallbackFlowDue:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if parsed.Payload == "custom" {
			_ = r.flow.Set(ctx, flowKey, "reminder_typed_due", payload, r.cfg.FlowTTL)
			prompt := r.i18n.T(locale, "general.invalid_date_format")
			_, err := r.client.SendMessage(ctx, chatID, prompt, &SendOptions{
				ThreadID: threadID,
			})
			return err
		}

		payload.DueAt = r.resolveDuePreset(parsed.Payload)
		_ = r.flow.Set(ctx, flowKey, "reminder_recurrence", payload, r.cfg.FlowTTL)
		return r.promptReminderRecurrence(ctx, chatID, threadID, locale, payload)

	case CallbackFlowRecurrence:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if parsed.Payload == "none" {
			payload.RecurrenceInterval = nil
			payload.RecurrenceUnit = nil
			payload.RecurrenceAnchor = nil
			_ = r.flow.Set(ctx, flowKey, "reminder_confirm", payload, r.cfg.FlowTTL)
			return r.promptReminderConfirm(ctx, chatID, threadID, locale, payload)
		}

		interval := 1
		unit := string(recurrence.UnitDay)
		switch parsed.Payload {
		case "daily":
			unit = string(recurrence.UnitDay)
		case "weekly":
			unit = string(recurrence.UnitWeek)
		case "monthly":
			unit = string(recurrence.UnitMonth)
		}
		payload.RecurrenceInterval = &interval
		payload.RecurrenceUnit = &unit
		_ = r.flow.Set(ctx, flowKey, "reminder_anchor", payload, r.cfg.FlowTTL)
		return r.promptReminderAnchor(ctx, chatID, threadID, locale, payload)

	case CallbackFlowAnchor:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		anchor := string(recurrence.AnchorScheduled)
		if parsed.Payload == "completion" {
			anchor = string(recurrence.AnchorCompletion)
		}
		payload.RecurrenceAnchor = &anchor
		_ = r.flow.Set(ctx, flowKey, "reminder_confirm", payload, r.cfg.FlowTTL)
		return r.promptReminderConfirm(ctx, chatID, threadID, locale, payload)

	case CallbackFlowConfirm:
		_ = r.flow.Clear(ctx, flowKey)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.confirmed"), false)

		var rule *recurrence.Rule
		if payload.RecurrenceInterval != nil {
			rule = &recurrence.Rule{
				Interval: *payload.RecurrenceInterval,
				Unit:     recurrence.Unit(*payload.RecurrenceUnit),
				Anchor:   recurrence.Anchor(*payload.RecurrenceAnchor),
			}
		}

		def, err := r.reminders.CreateDefinition(ctx, member.ID, payload.TargetUserID, payload.Title, payload.Emoji, "", rule)
		if err != nil {
			_, _ = r.client.SendMessage(ctx, chatID, r.i18n.T(locale, "reminders.create_failed"), &SendOptions{
				ThreadID: threadID,
			})
			return err
		}

		_, err = r.reminders.ScheduleOccurrence(ctx, member.ID, def.ID, payload.DueAt)
		if err != nil {
			_, _ = r.client.SendMessage(ctx, chatID, r.i18n.T(locale, "reminders.schedule_failed"), &SendOptions{
				ThreadID: threadID,
			})
			return err
		}

		_, err = r.client.SendMessage(ctx, chatID, r.i18n.T(locale, "reminders.scheduled"), &SendOptions{
			ThreadID: threadID,
		})
		return err
	}

	return nil
}
