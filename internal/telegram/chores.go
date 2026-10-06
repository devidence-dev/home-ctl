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
	"github.com/px1/home-ctl/internal/recurrence"
)

type choreFlowPayload struct {
	DefID              int64     `json:"def_id,omitempty"`
	Name               string    `json:"name,omitempty"`
	Emoji              string    `json:"emoji,omitempty"`
	AssigneeID         int64     `json:"assignee_id,omitempty"`
	AssigneeName       string    `json:"assignee_name,omitempty"`
	DueAt              time.Time `json:"due_at,omitempty"`
	RecurrenceInterval *int      `json:"recurrence_interval,omitempty"`
	RecurrenceUnit     *string   `json:"recurrence_unit,omitempty"`
	RecurrenceAnchor   *string   `json:"recurrence_anchor,omitempty"`
}

func parseDateTime(input string, clock Clock) (time.Time, error) {
	trimmed := strings.TrimSpace(input)
	loc := clock.Location()
	now := clock.Now().In(loc)

	// Try YYYY-MM-DD HH:MM
	if t, err := time.ParseInLocation("2006-01-02 15:04", trimmed, loc); err == nil {
		if t.Before(now) {
			return time.Time{}, chores.ErrPastDueTime
		}
		return t, nil
	}

	// Try HH:MM for today
	if t, err := time.ParseInLocation("15:04", trimmed, loc); err == nil {
		combined := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, loc)
		if combined.Before(now) {
			return time.Time{}, chores.ErrPastDueTime
		}
		return combined, nil
	}

	return time.Time{}, errors.New("invalid date format")
}

func (r *Router) handleChoresAdd(ctx context.Context, member *household.Member, msg *Message) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	defs, err := r.chores.ListDefinitions(ctx)
	if err != nil {
		return err
	}

	flowKey := flow.Key{
		UserID:   member.ID,
		ChatID:   msg.Chat.ID,
		ThreadID: msg.MessageThreadID,
	}

	if len(defs) == 0 {
		_ = r.flow.Set(ctx, flowKey, "chore_new_name", choreFlowPayload{}, r.cfg.FlowTTL)
		prompt := r.i18n.T(locale, "chores.prompt_new_name")
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

	// Definitions exist: list them with "+ New chore" and "Cancel"
	_ = r.flow.Set(ctx, flowKey, "chore_select_def", choreFlowPayload{}, r.cfg.FlowTTL)
	var rows [][]InlineKeyboardButton
	for _, d := range defs {
		rows = append(rows, []InlineKeyboardButton{
			{
				Text:         fmt.Sprintf("%s %s", d.Emoji, d.Name),
				CallbackData: fmt.Sprintf("fl:sel:%d", d.ID),
			},
		})
	}
	rows = append(rows, []InlineKeyboardButton{
		{Text: r.i18n.T(locale, "buttons.new_chore"), CallbackData: "fl:new"},
		{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
	})

	prompt := r.i18n.T(locale, "chores.prompt_select")
	_, err = r.client.SendMessage(ctx, msg.Chat.ID, prompt, &SendOptions{
		ThreadID:    msg.MessageThreadID,
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

func (r *Router) promptChoreAssignee(ctx context.Context, chatID, threadID int64, locale string, payload choreFlowPayload) error {
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

	prompt := r.i18n.T(locale, "chores.prompt_assignee", payload.Name)
	_, err = r.client.SendMessage(ctx, chatID, prompt, &SendOptions{
		ThreadID:    threadID,
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

func (r *Router) promptChoreDueDate(ctx context.Context, chatID, threadID int64, locale string, payload choreFlowPayload) error {
	rows := r.dueDateRows(locale)

	prompt := r.i18n.T(locale, "chores.prompt_due_date", payload.Name)
	_, err := r.client.SendMessage(ctx, chatID, prompt, &SendOptions{
		ThreadID:    threadID,
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

func (r *Router) promptChoreRecurrence(ctx context.Context, chatID, threadID int64, locale string, payload choreFlowPayload) error {
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

	prompt := r.i18n.T(locale, "chores.prompt_recurrence", payload.Name)
	_, err := r.client.SendMessage(ctx, chatID, prompt, &SendOptions{
		ThreadID:    threadID,
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

func (r *Router) promptChoreAnchor(ctx context.Context, chatID, threadID int64, locale string, payload choreFlowPayload) error {
	rows := [][]InlineKeyboardButton{
		{
			{Text: r.i18n.T(locale, "buttons.anchor_scheduled"), CallbackData: "fl:anc:scheduled"},
			{Text: r.i18n.T(locale, "buttons.anchor_completion"), CallbackData: "fl:anc:completion"},
		},
		{
			{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
		},
	}

	prompt := r.i18n.T(locale, "chores.prompt_anchor", payload.Name)
	_, err := r.client.SendMessage(ctx, chatID, prompt, &SendOptions{
		ThreadID:    threadID,
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

func (r *Router) promptChoreConfirm(ctx context.Context, chatID, threadID int64, locale string, payload choreFlowPayload) error {
	dueStr := r.formatDateTime(locale, payload.DueAt)
	recSummary := r.i18n.T(locale, "general.recurrence_one_off")
	if payload.RecurrenceInterval != nil && payload.RecurrenceUnit != nil {
		recSummary = r.recurrenceSummary(locale, *payload.RecurrenceInterval, *payload.RecurrenceUnit, payload.RecurrenceAnchor)
	}

	prompt := r.i18n.T(locale, "chores.prompt_confirm", payload.Name, payload.AssigneeName, dueStr, recSummary)
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

func (r *Router) handleChoresFlowText(ctx context.Context, member *household.Member, msg *Message, state *flow.State) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	flowKey := state.Key
	text := strings.TrimSpace(msg.Text)

	var payload choreFlowPayload
	_ = json.Unmarshal(state.Payload, &payload)

	switch state.Step {
	case "chore_new_name":
		if text == "" {
			return nil
		}
		payload.Name = text
		payload.Emoji = "🧹"
		_ = r.flow.Set(ctx, flowKey, "chore_assignee", payload, r.cfg.FlowTTL)
		return r.promptChoreAssignee(ctx, msg.Chat.ID, msg.MessageThreadID, locale, payload)

	case "chore_typed_due":
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
		_ = r.flow.Set(ctx, flowKey, "chore_recurrence", payload, r.cfg.FlowTTL)
		return r.promptChoreRecurrence(ctx, msg.Chat.ID, msg.MessageThreadID, locale, payload)
	}

	return nil
}

func (r *Router) handleChoresFlowCallback(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	flowKey := state.Key
	var payload choreFlowPayload
	_ = json.Unmarshal(state.Payload, &payload)

	chatID := cq.Message.Chat.ID
	threadID := cq.Message.MessageThreadID

	switch parsed.Kind {
	case CallbackFlowCancel:
		_ = r.flow.Clear(ctx, flowKey)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.cancelled"), false)
		_, err := r.client.SendMessage(ctx, chatID, r.i18n.T(locale, "chores.flow_cancelled"), &SendOptions{
			ThreadID: threadID,
		})
		return err

	case CallbackFlowNewItem:
		_ = r.flow.Set(ctx, flowKey, "chore_new_name", choreFlowPayload{}, r.cfg.FlowTTL)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		prompt := r.i18n.T(locale, "chores.prompt_new_name")
		markup := &InlineKeyboardMarkup{
			InlineKeyboard: [][]InlineKeyboardButton{
				{{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"}},
			},
		}
		_, err := r.client.SendMessage(ctx, chatID, prompt, &SendOptions{
			ThreadID:    threadID,
			ReplyMarkup: markup,
		})
		return err

	case CallbackFlowSelect:
		defs, _ := r.chores.ListDefinitions(ctx)
		for _, d := range defs {
			if d.ID == parsed.TargetID {
				payload.DefID = d.ID
				payload.Name = d.Name
				payload.Emoji = d.Emoji
				if d.RecurrenceInterval != nil {
					payload.RecurrenceInterval = d.RecurrenceInterval
					payload.RecurrenceUnit = d.RecurrenceUnit
					payload.RecurrenceAnchor = d.RecurrenceAnchor
				}
				break
			}
		}
		_ = r.flow.Set(ctx, flowKey, "chore_assignee", payload, r.cfg.FlowTTL)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return r.promptChoreAssignee(ctx, chatID, threadID, locale, payload)

	case CallbackFlowUser:
		payload.AssigneeID = parsed.TargetID
		m, err := r.household.Authorize(ctx, parsed.TargetID)
		if err == nil && m != nil {
			payload.AssigneeName = m.Name
			if payload.AssigneeName == "" {
				payload.AssigneeName = m.Username
			}
		}
		_ = r.flow.Set(ctx, flowKey, "chore_due", payload, r.cfg.FlowTTL)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return r.promptChoreDueDate(ctx, chatID, threadID, locale, payload)

	case CallbackFlowDue:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if parsed.Payload == "custom" {
			_ = r.flow.Set(ctx, flowKey, "chore_typed_due", payload, r.cfg.FlowTTL)
			prompt := r.i18n.T(locale, "general.invalid_date_format")
			_, err := r.client.SendMessage(ctx, chatID, prompt, &SendOptions{
				ThreadID: threadID,
			})
			return err
		}

		payload.DueAt = r.resolveDuePreset(parsed.Payload)
		if payload.DefID != 0 && payload.RecurrenceInterval != nil {
			// Pre-existing recurrence on definition
			_ = r.flow.Set(ctx, flowKey, "chore_confirm", payload, r.cfg.FlowTTL)
			return r.promptChoreConfirm(ctx, chatID, threadID, locale, payload)
		}

		_ = r.flow.Set(ctx, flowKey, "chore_recurrence", payload, r.cfg.FlowTTL)
		return r.promptChoreRecurrence(ctx, chatID, threadID, locale, payload)

	case CallbackFlowRecurrence:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if parsed.Payload == "none" {
			payload.RecurrenceInterval = nil
			payload.RecurrenceUnit = nil
			payload.RecurrenceAnchor = nil
			_ = r.flow.Set(ctx, flowKey, "chore_confirm", payload, r.cfg.FlowTTL)
			return r.promptChoreConfirm(ctx, chatID, threadID, locale, payload)
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
		_ = r.flow.Set(ctx, flowKey, "chore_anchor", payload, r.cfg.FlowTTL)
		return r.promptChoreAnchor(ctx, chatID, threadID, locale, payload)

	case CallbackFlowAnchor:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		anchor := string(recurrence.AnchorScheduled)
		if parsed.Payload == "completion" {
			anchor = string(recurrence.AnchorCompletion)
		}
		payload.RecurrenceAnchor = &anchor
		_ = r.flow.Set(ctx, flowKey, "chore_confirm", payload, r.cfg.FlowTTL)
		return r.promptChoreConfirm(ctx, chatID, threadID, locale, payload)

	case CallbackFlowConfirm:
		// Permanent records are written ONLY at Confirm!
		_ = r.flow.Clear(ctx, flowKey)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.confirmed"), false)

		defID := payload.DefID
		if defID == 0 {
			var rule *recurrence.Rule
			if payload.RecurrenceInterval != nil {
				rule = &recurrence.Rule{
					Interval: *payload.RecurrenceInterval,
					Unit:     recurrence.Unit(*payload.RecurrenceUnit),
					Anchor:   recurrence.Anchor(*payload.RecurrenceAnchor),
				}
			}
			def, err := r.chores.CreateDefinition(ctx, member.ID, payload.Name, payload.Emoji, "", rule)
			if err != nil {
				_, _ = r.client.SendMessage(ctx, chatID, r.i18n.T(locale, "chores.create_failed"), &SendOptions{
					ThreadID: threadID,
				})
				return err
			}
			defID = def.ID
		}

		_, err := r.chores.ScheduleOccurrence(ctx, member.ID, defID, payload.AssigneeID, payload.DueAt)
		if err != nil {
			_, _ = r.client.SendMessage(ctx, chatID, r.i18n.T(locale, "chores.schedule_failed"), &SendOptions{
				ThreadID: threadID,
			})
			return err
		}

		_, err = r.client.SendMessage(ctx, chatID, r.i18n.T(locale, "chores.scheduled"), &SendOptions{
			ThreadID: threadID,
		})
		return err
	}

	return nil
}
