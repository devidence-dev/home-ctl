package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
	RecurrenceType     string    `json:"recurrence_type,omitempty"`
	RecurrenceInterval *int      `json:"recurrence_interval,omitempty"`
	RecurrenceUnit     *string   `json:"recurrence_unit,omitempty"`
	RecurrenceAnchor   *string   `json:"recurrence_anchor,omitempty"`
	DueDate            string    `json:"due_date,omitempty"`
	DueHour            int       `json:"due_hour,omitempty"`
	DueMinute          int       `json:"due_minute,omitempty"`
	DueAt              time.Time `json:"due_at,omitempty"`
	CalYear            int       `json:"cal_year,omitempty"`
	CalMonth           int       `json:"cal_month,omitempty"`
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
	st, _ := r.flow.Get(ctx, flowKey, nil)
	prompt := r.i18n.T(locale, "reminders.prompt_title")
	return r.showFlowMessage(ctx, st, prompt, r.cancelOnlyMarkup(locale), true)
}

func (r *Router) promptReminderTarget(ctx context.Context, state *flow.State, locale string) error {
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
	return r.showFlowMessage(ctx, state, prompt, &InlineKeyboardMarkup{InlineKeyboard: rows})
}

func (r *Router) promptReminderRecurrenceType(ctx context.Context, state *flow.State, locale string) error {
	prompt, markup := RenderRecurrenceTypeStep(r.i18n, locale)
	return r.showFlowMessage(ctx, state, prompt, markup)
}

func (r *Router) promptReminderInterval(ctx context.Context, state *flow.State, locale string, payload reminderFlowPayload) error {
	prompt, markup := RenderIntervalStep(r.i18n, locale, payload.RecurrenceType)
	return r.showFlowMessage(ctx, state, prompt, markup)
}

func (r *Router) promptReminderDate(ctx context.Context, state *flow.State, locale string, payload *reminderFlowPayload) error {
	now := r.clock.Now().In(r.clock.Location())
	if payload.CalYear == 0 {
		payload.CalYear = now.Year()
		payload.CalMonth = int(now.Month())
	}
	prompt, markup := RenderCalendarStep(r.i18n, locale, payload.CalYear, time.Month(payload.CalMonth), now, r.clock.Location())
	return r.showFlowMessage(ctx, state, prompt, markup)
}

func (r *Router) promptReminderHour(ctx context.Context, state *flow.State, locale string) error {
	prompt, markup := RenderHourStep(r.i18n, locale)
	return r.showFlowMessage(ctx, state, prompt, markup)
}

func (r *Router) promptReminderMinute(ctx context.Context, state *flow.State, locale string) error {
	prompt, markup := RenderMinuteStep(r.i18n, locale)
	return r.showFlowMessage(ctx, state, prompt, markup)
}

func (r *Router) promptReminderAnchor(ctx context.Context, state *flow.State, locale string) error {
	prompt, markup := RenderAnchorStep(r.i18n, locale)
	return r.showFlowMessage(ctx, state, prompt, markup)
}

func (r *Router) promptReminderConfirm(ctx context.Context, state *flow.State, locale string, payload reminderFlowPayload) error {
	recType := payload.RecurrenceType
	if recType == "" && payload.RecurrenceUnit != nil {
		switch recurrence.Unit(*payload.RecurrenceUnit) {
		case recurrence.UnitDay:
			recType = "daily"
		case recurrence.UnitWeek:
			recType = "weekly"
		case recurrence.UnitMonth:
			recType = "monthly"
		}
	}
	interval := 1
	if payload.RecurrenceInterval != nil {
		interval = *payload.RecurrenceInterval
	}
	anchor := ""
	if payload.RecurrenceAnchor != nil {
		anchor = *payload.RecurrenceAnchor
	}

	prompt, markup := RenderConfirmationStep(
		r.i18n,
		locale,
		payload.Title,
		r.i18n.T(locale, "picker.summary_target"),
		payload.TargetUserName,
		payload.DueAt,
		recType,
		interval,
		anchor,
		r.clock.Now(),
		r.clock.Location(),
	)
	return r.showFlowMessage(ctx, state, prompt, markup)
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
		return r.promptReminderTarget(ctx, state, locale)

	case "reminder_typed_interval":
		n, err := ValidateTypedInterval(text)
		if err != nil {
			guidance := r.i18n.T(locale, "picker.invalid_interval") + "\n\n" + r.i18n.T(locale, "picker.prompt_type_interval")
			return r.showFlowMessage(ctx, state, guidance, r.cancelOnlyMarkup(locale))
		}
		payload.RecurrenceInterval = &n
		_ = r.flow.Set(ctx, flowKey, "reminder_date", payload, r.cfg.FlowTTL)
		return r.promptReminderDate(ctx, state, locale, &payload)

	case "reminder_typed_date":
		dateOnly, dateTime, isDateTime, err := r.parseTypedDateOrTime(text)
		if err != nil {
			guidance := r.i18n.T(locale, "general.invalid_date_format") + "\n\n" + r.i18n.T(locale, "picker.prompt_type_date")
			return r.showFlowMessage(ctx, state, guidance, r.cancelOnlyMarkup(locale))
		}
		if isDateTime {
			now := r.clock.Now().In(r.clock.Location())
			if !dateTime.After(now) {
				guidance := r.i18n.T(locale, "general.past_due_time") + "\n\n" + r.i18n.T(locale, "picker.prompt_type_date")
				return r.showFlowMessage(ctx, state, guidance, r.cancelOnlyMarkup(locale))
			}
			payload.DueAt = dateTime
			payload.DueDate = dateTime.Format("2006-01-02")
			payload.DueHour = dateTime.Hour()
			payload.DueMinute = dateTime.Minute()

			isRecurring := payload.RecurrenceType != "" && payload.RecurrenceType != "once"
			if isRecurring {
				_ = r.flow.Set(ctx, flowKey, "reminder_anchor", payload, r.cfg.FlowTTL)
				return r.promptReminderAnchor(ctx, state, locale)
			}
			_ = r.flow.Set(ctx, flowKey, "reminder_confirm", payload, r.cfg.FlowTTL)
			return r.promptReminderConfirm(ctx, state, locale, payload)
		}
		payload.DueDate = dateOnly
		_ = r.flow.Set(ctx, flowKey, "reminder_hour", payload, r.cfg.FlowTTL)
		return r.promptReminderHour(ctx, state, locale)

	case "reminder_typed_minute":
		m, err := ValidateTypedMinute(text)
		if err != nil {
			guidance := r.i18n.T(locale, "picker.invalid_minute") + "\n\n" + r.i18n.T(locale, "picker.prompt_type_minute")
			return r.showFlowMessage(ctx, state, guidance, r.cancelOnlyMarkup(locale))
		}
		loc := r.clock.Location()
		dateT, _ := time.ParseInLocation("2006-01-02", payload.DueDate, loc)
		dueAt := time.Date(dateT.Year(), dateT.Month(), dateT.Day(), payload.DueHour, m, 0, 0, loc)
		now := r.clock.Now().In(loc)
		if !dueAt.After(now) {
			payload.DueHour = 0
			payload.DueMinute = 0
			_ = r.flow.Set(ctx, flowKey, "reminder_hour", payload, r.cfg.FlowTTL)
			hourPrompt, hourMarkup := RenderHourStep(r.i18n, locale)
			hourPrompt = r.i18n.T(locale, "general.past_due_time") + "\n\n" + hourPrompt
			return r.showFlowMessage(ctx, state, hourPrompt, hourMarkup)
		}
		payload.DueAt = dueAt
		payload.DueMinute = m

		isRecurring := payload.RecurrenceType != "" && payload.RecurrenceType != "once"
		if isRecurring {
			_ = r.flow.Set(ctx, flowKey, "reminder_anchor", payload, r.cfg.FlowTTL)
			return r.promptReminderAnchor(ctx, state, locale)
		}
		_ = r.flow.Set(ctx, flowKey, "reminder_confirm", payload, r.cfg.FlowTTL)
		return r.promptReminderConfirm(ctx, state, locale, payload)
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
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		_ = r.flow.Set(ctx, flowKey, "reminder_recurrence_type", payload, r.cfg.FlowTTL)
		return r.promptReminderRecurrenceType(ctx, state, locale)

	case CallbackFlowRecurrenceType:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		payload.RecurrenceType = parsed.Payload
		if parsed.Payload == "once" {
			payload.RecurrenceInterval = nil
			payload.RecurrenceUnit = nil
			payload.RecurrenceAnchor = nil
			_ = r.flow.Set(ctx, flowKey, "reminder_date", payload, r.cfg.FlowTTL)
			return r.promptReminderDate(ctx, state, locale, &payload)
		}
		var unit string
		switch parsed.Payload {
		case "daily":
			unit = string(recurrence.UnitDay)
		case "weekly":
			unit = string(recurrence.UnitWeek)
		case "monthly":
			unit = string(recurrence.UnitMonth)
		}
		payload.RecurrenceUnit = &unit
		_ = r.flow.Set(ctx, flowKey, "reminder_interval", payload, r.cfg.FlowTTL)
		return r.promptReminderInterval(ctx, state, locale, payload)

	case CallbackFlowInterval:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if parsed.Payload == "o" {
			_ = r.flow.Set(ctx, flowKey, "reminder_typed_interval", payload, r.cfg.FlowTTL)
			prompt := r.i18n.T(locale, "picker.prompt_type_interval")
			return r.showFlowMessage(ctx, state, prompt, r.cancelOnlyMarkup(locale))
		}
		n := int(parsed.TargetID)
		payload.RecurrenceInterval = &n
		_ = r.flow.Set(ctx, flowKey, "reminder_date", payload, r.cfg.FlowTTL)
		return r.promptReminderDate(ctx, state, locale, &payload)

	case CallbackFlowCalendar:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		calParts := strings.Split(parsed.Payload, "-")
		if len(calParts) == 2 {
			y, _ := strconv.Atoi(calParts[0])
			m, _ := strconv.Atoi(calParts[1])
			payload.CalYear = y
			payload.CalMonth = m
		}
		_ = r.flow.Set(ctx, flowKey, "reminder_date", payload, r.cfg.FlowTTL)
		return r.promptReminderDate(ctx, state, locale, &payload)

	case CallbackFlowDate:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		nowInLoc := r.clock.Now().In(r.clock.Location())
		switch parsed.Payload {
		case "type":
			_ = r.flow.Set(ctx, flowKey, "reminder_typed_date", payload, r.cfg.FlowTTL)
			prompt := r.i18n.T(locale, "picker.prompt_type_date")
			return r.showFlowMessage(ctx, state, prompt, r.cancelOnlyMarkup(locale))
		case "today":
			payload.DueDate = nowInLoc.Format("2006-01-02")
		case "tomorrow":
			payload.DueDate = nowInLoc.AddDate(0, 0, 1).Format("2006-01-02")
		default:
			payload.DueDate = parsed.Payload
		}
		_ = r.flow.Set(ctx, flowKey, "reminder_hour", payload, r.cfg.FlowTTL)
		return r.promptReminderHour(ctx, state, locale)

	case CallbackFlowHour:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		payload.DueHour = int(parsed.TargetID)
		_ = r.flow.Set(ctx, flowKey, "reminder_minute", payload, r.cfg.FlowTTL)
		return r.promptReminderMinute(ctx, state, locale)

	case CallbackFlowMinute:
		if parsed.Payload == "o" {
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
			_ = r.flow.Set(ctx, flowKey, "reminder_typed_minute", payload, r.cfg.FlowTTL)
			prompt := r.i18n.T(locale, "picker.prompt_type_minute")
			return r.showFlowMessage(ctx, state, prompt, r.cancelOnlyMarkup(locale))
		}
		minute := int(parsed.TargetID)
		loc := r.clock.Location()
		dateT, _ := time.ParseInLocation("2006-01-02", payload.DueDate, loc)
		dueAt := time.Date(dateT.Year(), dateT.Month(), dateT.Day(), payload.DueHour, minute, 0, 0, loc)
		now := r.clock.Now().In(loc)
		if !dueAt.After(now) {
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.past_due_time"), true)
			payload.DueHour = 0
			payload.DueMinute = 0
			_ = r.flow.Set(ctx, flowKey, "reminder_hour", payload, r.cfg.FlowTTL)
			hourPrompt, hourMarkup := RenderHourStep(r.i18n, locale)
			return r.showFlowMessage(ctx, state, hourPrompt, hourMarkup)
		}
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		payload.DueAt = dueAt
		payload.DueMinute = minute

		isRecurring := payload.RecurrenceType != "" && payload.RecurrenceType != "once"
		if isRecurring {
			_ = r.flow.Set(ctx, flowKey, "reminder_anchor", payload, r.cfg.FlowTTL)
			return r.promptReminderAnchor(ctx, state, locale)
		}
		_ = r.flow.Set(ctx, flowKey, "reminder_confirm", payload, r.cfg.FlowTTL)
		return r.promptReminderConfirm(ctx, state, locale, payload)

	case CallbackFlowAnchor:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		anc := parsed.Payload
		payload.RecurrenceAnchor = &anc
		_ = r.flow.Set(ctx, flowKey, "reminder_confirm", payload, r.cfg.FlowTTL)
		return r.promptReminderConfirm(ctx, state, locale, payload)

	case CallbackFlowConfirm:
		_ = r.flow.Clear(ctx, flowKey)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.confirmed"), false)

		var rule *recurrence.Rule
		if payload.RecurrenceInterval != nil && payload.RecurrenceUnit != nil {
			anchor := recurrence.AnchorScheduled
			if payload.RecurrenceAnchor != nil {
				anchor = recurrence.Anchor(*payload.RecurrenceAnchor)
			}
			rule = &recurrence.Rule{
				Interval: *payload.RecurrenceInterval,
				Unit:     recurrence.Unit(*payload.RecurrenceUnit),
				Anchor:   anchor,
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
		return nil
	}

	return nil
}
