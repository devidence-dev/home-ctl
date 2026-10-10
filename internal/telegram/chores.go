package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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

// choreContext returns the values already chosen, shown as the header of each step after the title.
func choreContext(p choreFlowPayload) PickerContext {
	return PickerContext{Emoji: p.Emoji, Title: p.Name, Person: p.AssigneeName, Date: p.DueDate}
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
		st, _ := r.flow.Get(ctx, flowKey, nil)
		prompt := r.i18n.T(locale, "chores.prompt_new_name")
		return r.showFlowMessage(ctx, st, prompt, r.cancelOnlyMarkup(locale), true)
	}

	// Definitions exist: list them with "+ New chore" and "Cancel"
	_ = r.flow.Set(ctx, flowKey, "chore_select_def", choreFlowPayload{}, r.cfg.FlowTTL)
	st, _ := r.flow.Get(ctx, flowKey, nil)
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
	return r.showFlowMessage(ctx, st, prompt, &InlineKeyboardMarkup{InlineKeyboard: rows}, true)
}

func (r *Router) promptChoreAssignee(ctx context.Context, state *flow.State, locale string, payload choreFlowPayload) error {
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
	return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), &InlineKeyboardMarkup{InlineKeyboard: rows})
}

func (r *Router) promptChoreRecurrenceType(ctx context.Context, state *flow.State, locale string, payload choreFlowPayload) error {
	prompt, markup := RenderRecurrenceTypeStep(r.i18n, locale)
	return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), markup)
}

func (r *Router) promptChoreInterval(ctx context.Context, state *flow.State, locale string, payload choreFlowPayload) error {
	prompt, markup := RenderIntervalStep(r.i18n, locale, payload.RecurrenceType)
	return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), markup)
}

func (r *Router) promptChoreDate(ctx context.Context, state *flow.State, locale string, payload *choreFlowPayload) error {
	now := r.clock.Now().In(r.clock.Location())
	if payload.CalYear == 0 {
		payload.CalYear = now.Year()
		payload.CalMonth = int(now.Month())
	}
	prompt, markup := RenderCalendarStep(r.i18n, locale, payload.CalYear, time.Month(payload.CalMonth), now, r.clock.Location())
	return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(*payload), prompt), markup)
}

// promptChoreHour shows the hour step; notice explains a rejected time and may be empty.
func (r *Router) promptChoreHour(ctx context.Context, state *flow.State, locale string, payload choreFlowPayload, notice string) error {
	prompt, markup := RenderHourStep(r.i18n, locale, payload.DueDate, r.clock.Now(), r.clock.Location())
	return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), notice, prompt), markup)
}

func (r *Router) promptChoreMinute(ctx context.Context, state *flow.State, locale string, payload choreFlowPayload) error {
	prompt, markup := RenderMinuteStep(r.i18n, locale, payload.DueDate, payload.DueHour, r.clock.Now(), r.clock.Location())
	return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), markup)
}

func (r *Router) promptChoreAnchor(ctx context.Context, state *flow.State, locale string, payload choreFlowPayload) error {
	prompt, markup := RenderAnchorStep(r.i18n, locale)
	return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), markup)
}

func (r *Router) promptChoreConfirm(ctx context.Context, state *flow.State, locale string, payload choreFlowPayload) error {
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

	prompt, markup := RenderConfirmationStep(r.i18n, locale, ConfirmationSummary{
		Emoji:       payload.Emoji,
		Title:       payload.Name,
		PersonLabel: "picker.summary_assignee",
		PersonName:  payload.AssigneeName,
		DueAt:       payload.DueAt,
		RecType:     recType,
		Interval:    interval,
		Anchor:      anchor,
	}, r.clock.Now(), r.clock.Location())
	return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), markup)
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

	// Typed answers, accepted or rejected, re-anchor the flow below the user's message.
	switch state.Step {
	case "chore_new_name":
		if text == "" {
			return nil
		}
		payload.Name = text
		payload.Emoji = "🧹"
		_ = r.flow.Set(ctx, flowKey, "chore_assignee", payload, r.cfg.FlowTTL)
		return r.reanchorFlow(ctx, state, func() error {
			return r.promptChoreAssignee(ctx, state, locale, payload)
		})

	case "chore_typed_interval":
		n, err := ValidateTypedInterval(text)
		if err != nil {
			return r.reanchorRejected(ctx, state, locale, choreContext(payload),
				r.i18n.T(locale, "picker.invalid_interval"), r.i18n.T(locale, "picker.prompt_type_interval"))
		}
		payload.RecurrenceInterval = &n
		_ = r.flow.Set(ctx, flowKey, "chore_date", payload, r.cfg.FlowTTL)
		return r.reanchorFlow(ctx, state, func() error {
			return r.promptChoreDate(ctx, state, locale, &payload)
		})

	case "chore_typed_date":
		dateOnly, dateTime, isDateTime, err := r.parseTypedDateOrTime(text)
		if err != nil {
			return r.reanchorRejected(ctx, state, locale, choreContext(payload),
				r.i18n.T(locale, "general.invalid_date_format"), r.i18n.T(locale, "picker.prompt_type_date"))
		}
		if isDateTime {
			now := r.clock.Now().In(r.clock.Location())
			if !dateTime.After(now) {
				return r.reanchorRejected(ctx, state, locale, choreContext(payload),
					r.i18n.T(locale, "general.past_due_time"), r.i18n.T(locale, "picker.prompt_type_date"))
			}
			payload.DueAt = dateTime
			payload.DueDate = dateTime.Format("2006-01-02")
			payload.DueHour = dateTime.Hour()
			payload.DueMinute = dateTime.Minute()

			isRecurring := payload.RecurrenceType != "" && payload.RecurrenceType != "once"
			hasDefRec := payload.DefID != 0 && payload.RecurrenceInterval != nil && payload.RecurrenceAnchor != nil
			if isRecurring && !hasDefRec {
				_ = r.flow.Set(ctx, flowKey, "chore_anchor", payload, r.cfg.FlowTTL)
				return r.reanchorFlow(ctx, state, func() error {
					return r.promptChoreAnchor(ctx, state, locale, payload)
				})
			}
			_ = r.flow.Set(ctx, flowKey, "chore_confirm", payload, r.cfg.FlowTTL)
			return r.reanchorFlow(ctx, state, func() error {
				return r.promptChoreConfirm(ctx, state, locale, payload)
			})
		}
		payload.DueDate = dateOnly
		_ = r.flow.Set(ctx, flowKey, "chore_hour", payload, r.cfg.FlowTTL)
		return r.reanchorFlow(ctx, state, func() error {
			return r.promptChoreHour(ctx, state, locale, payload, "")
		})

	case "chore_typed_minute":
		m, err := ValidateTypedMinute(text)
		if err != nil {
			return r.reanchorRejected(ctx, state, locale, choreContext(payload),
				r.i18n.T(locale, "picker.invalid_minute"), r.i18n.T(locale, "picker.prompt_type_minute"))
		}
		loc := r.clock.Location()
		dateT, _ := time.ParseInLocation("2006-01-02", payload.DueDate, loc)
		dueAt := time.Date(dateT.Year(), dateT.Month(), dateT.Day(), payload.DueHour, m, 0, 0, loc)
		now := r.clock.Now().In(loc)
		if !dueAt.After(now) {
			payload.DueHour = 0
			payload.DueMinute = 0
			_ = r.flow.Set(ctx, flowKey, "chore_hour", payload, r.cfg.FlowTTL)
			return r.reanchorFlow(ctx, state, func() error {
				return r.promptChoreHour(ctx, state, locale, payload, r.i18n.T(locale, "picker.time_passed"))
			})
		}
		payload.DueAt = dueAt
		payload.DueMinute = m

		isRecurring := payload.RecurrenceType != "" && payload.RecurrenceType != "once"
		hasDefRec := payload.DefID != 0 && payload.RecurrenceInterval != nil && payload.RecurrenceAnchor != nil
		if isRecurring && !hasDefRec {
			_ = r.flow.Set(ctx, flowKey, "chore_anchor", payload, r.cfg.FlowTTL)
			return r.reanchorFlow(ctx, state, func() error {
				return r.promptChoreAnchor(ctx, state, locale, payload)
			})
		}
		_ = r.flow.Set(ctx, flowKey, "chore_confirm", payload, r.cfg.FlowTTL)
		return r.reanchorFlow(ctx, state, func() error {
			return r.promptChoreConfirm(ctx, state, locale, payload)
		})
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
		payload = choreFlowPayload{}
		_ = r.flow.Set(ctx, flowKey, "chore_new_name", payload, r.cfg.FlowTTL)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		prompt := r.i18n.T(locale, "chores.prompt_new_name")
		return r.showFlowMessage(ctx, state, prompt, r.cancelOnlyMarkup(locale))

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
					if d.RecurrenceUnit != nil {
						switch recurrence.Unit(*d.RecurrenceUnit) {
						case recurrence.UnitDay:
							payload.RecurrenceType = "daily"
						case recurrence.UnitWeek:
							payload.RecurrenceType = "weekly"
						case recurrence.UnitMonth:
							payload.RecurrenceType = "monthly"
						}
					}
				}
				break
			}
		}
		_ = r.flow.Set(ctx, flowKey, "chore_assignee", payload, r.cfg.FlowTTL)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return r.promptChoreAssignee(ctx, state, locale, payload)

	case CallbackFlowUser:
		payload.AssigneeID = parsed.TargetID
		m, err := r.household.Authorize(ctx, parsed.TargetID)
		if err == nil && m != nil {
			payload.AssigneeName = m.Name
			if payload.AssigneeName == "" {
				payload.AssigneeName = m.Username
			}
		}
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if payload.DefID != 0 && payload.RecurrenceInterval != nil {
			// Pre-existing recurrence on definition: skip recurrence steps
			_ = r.flow.Set(ctx, flowKey, "chore_date", payload, r.cfg.FlowTTL)
			return r.promptChoreDate(ctx, state, locale, &payload)
		}
		_ = r.flow.Set(ctx, flowKey, "chore_recurrence_type", payload, r.cfg.FlowTTL)
		return r.promptChoreRecurrenceType(ctx, state, locale, payload)

	case CallbackFlowRecurrenceType:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		payload.RecurrenceType = parsed.Payload
		if parsed.Payload == "once" {
			payload.RecurrenceInterval = nil
			payload.RecurrenceUnit = nil
			payload.RecurrenceAnchor = nil
			_ = r.flow.Set(ctx, flowKey, "chore_date", payload, r.cfg.FlowTTL)
			return r.promptChoreDate(ctx, state, locale, &payload)
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
		_ = r.flow.Set(ctx, flowKey, "chore_interval", payload, r.cfg.FlowTTL)
		return r.promptChoreInterval(ctx, state, locale, payload)

	case CallbackFlowInterval:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if parsed.Payload == "o" {
			_ = r.flow.Set(ctx, flowKey, "chore_typed_interval", payload, r.cfg.FlowTTL)
			prompt := r.i18n.T(locale, "picker.prompt_type_interval")
			return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), r.cancelOnlyMarkup(locale))
		}
		n := int(parsed.TargetID)
		payload.RecurrenceInterval = &n
		_ = r.flow.Set(ctx, flowKey, "chore_date", payload, r.cfg.FlowTTL)
		return r.promptChoreDate(ctx, state, locale, &payload)

	case CallbackFlowCalendar:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		calParts := strings.Split(parsed.Payload, "-")
		if len(calParts) == 2 {
			y, _ := strconv.Atoi(calParts[0])
			m, _ := strconv.Atoi(calParts[1])
			payload.CalYear = y
			payload.CalMonth = m
		}
		_ = r.flow.Set(ctx, flowKey, "chore_date", payload, r.cfg.FlowTTL)
		return r.promptChoreDate(ctx, state, locale, &payload)

	case CallbackFlowDate:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		nowInLoc := r.clock.Now().In(r.clock.Location())
		switch parsed.Payload {
		case "type":
			_ = r.flow.Set(ctx, flowKey, "chore_typed_date", payload, r.cfg.FlowTTL)
			prompt := r.i18n.T(locale, "picker.prompt_type_date")
			return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), r.cancelOnlyMarkup(locale))
		case "back":
			// Change date: back to the calendar on the chosen month, other choices kept.
			if day, err := time.ParseInLocation("2006-01-02", payload.DueDate, r.clock.Location()); err == nil {
				payload.CalYear = day.Year()
				payload.CalMonth = int(day.Month())
			}
			payload.DueDate = ""
			_ = r.flow.Set(ctx, flowKey, "chore_date", payload, r.cfg.FlowTTL)
			return r.promptChoreDate(ctx, state, locale, &payload)
		case "today":
			payload.DueDate = nowInLoc.Format("2006-01-02")
		case "tomorrow":
			payload.DueDate = nowInLoc.AddDate(0, 0, 1).Format("2006-01-02")
		default:
			payload.DueDate = parsed.Payload
		}
		_ = r.flow.Set(ctx, flowKey, "chore_hour", payload, r.cfg.FlowTTL)
		return r.promptChoreHour(ctx, state, locale, payload, "")

	case CallbackFlowHour:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		payload.DueHour = int(parsed.TargetID)
		_ = r.flow.Set(ctx, flowKey, "chore_minute", payload, r.cfg.FlowTTL)
		return r.promptChoreMinute(ctx, state, locale, payload)

	case CallbackFlowMinute:
		if parsed.Payload == "o" {
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
			_ = r.flow.Set(ctx, flowKey, "chore_typed_minute", payload, r.cfg.FlowTTL)
			prompt := r.i18n.T(locale, "picker.prompt_type_minute")
			return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), r.cancelOnlyMarkup(locale))
		}
		minute := int(parsed.TargetID)
		loc := r.clock.Location()
		dateT, _ := time.ParseInLocation("2006-01-02", payload.DueDate, loc)
		dueAt := time.Date(dateT.Year(), dateT.Month(), dateT.Day(), payload.DueHour, minute, 0, 0, loc)
		now := r.clock.Now().In(loc)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if !dueAt.After(now) {
			// The minute passed between rendering and pressing: explain it in the message.
			payload.DueHour = 0
			payload.DueMinute = 0
			_ = r.flow.Set(ctx, flowKey, "chore_hour", payload, r.cfg.FlowTTL)
			return r.promptChoreHour(ctx, state, locale, payload, r.i18n.T(locale, "picker.time_passed"))
		}
		payload.DueAt = dueAt
		payload.DueMinute = minute

		isRecurring := payload.RecurrenceType != "" && payload.RecurrenceType != "once"
		hasDefRec := payload.DefID != 0 && payload.RecurrenceInterval != nil && payload.RecurrenceAnchor != nil
		if isRecurring && !hasDefRec {
			_ = r.flow.Set(ctx, flowKey, "chore_anchor", payload, r.cfg.FlowTTL)
			return r.promptChoreAnchor(ctx, state, locale, payload)
		}
		_ = r.flow.Set(ctx, flowKey, "chore_confirm", payload, r.cfg.FlowTTL)
		return r.promptChoreConfirm(ctx, state, locale, payload)

	case CallbackFlowAnchor:
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		anc := parsed.Payload
		payload.RecurrenceAnchor = &anc
		_ = r.flow.Set(ctx, flowKey, "chore_confirm", payload, r.cfg.FlowTTL)
		return r.promptChoreConfirm(ctx, state, locale, payload)

	case CallbackFlowConfirm:
		_ = r.flow.Clear(ctx, flowKey)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.confirmed"), false)

		defID := payload.DefID
		if defID == 0 {
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
		return nil
	}

	return nil
}
