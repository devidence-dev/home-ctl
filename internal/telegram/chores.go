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
		{Text: r.i18n.T(locale, i18nKeyButtonCancel), CallbackData: "fl:c"},
	})

	prompt := r.i18n.T(locale, "chores.prompt_select")
	return r.showFlowMessage(ctx, st, prompt, &InlineKeyboardMarkup{InlineKeyboard: rows}, true)
}

func (r *Router) promptChoreEmoji(ctx context.Context, state *flow.State, locale string, payload choreFlowPayload) error {
	suggestions := EmojiSuggestions(r.i18n, locale, "picker.emoji_suggestions_chores")
	prompt, markup := RenderEmojiStep(r.i18n, locale, suggestions)
	return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), markup)
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
		{Text: r.i18n.T(locale, i18nKeyButtonCancel), CallbackData: "fl:c"},
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
		recType = recurrenceTypeForUnit(*payload.RecurrenceUnit)
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

// promptChoreStep renders the step named by storeChoreStepAfterTime.
func (r *Router) promptChoreStep(ctx context.Context, state *flow.State, locale, step string, payload choreFlowPayload) error {
	if step == "chore_anchor" {
		return r.promptChoreAnchor(ctx, state, locale, payload)
	}
	return r.promptChoreConfirm(ctx, state, locale, payload)
}

// storeChoreStepAfterTime stores the step that follows a chosen due time and returns its name: the anchor
// step for a recurring chore without a definition anchor, otherwise confirmation.
func (r *Router) storeChoreStepAfterTime(ctx context.Context, state *flow.State, payload choreFlowPayload) string {
	step := "chore_confirm"
	isRecurring := payload.RecurrenceType != "" && payload.RecurrenceType != "once"
	hasDefRec := payload.DefID != 0 && payload.RecurrenceInterval != nil && payload.RecurrenceAnchor != nil
	if isRecurring && !hasDefRec {
		step = "chore_anchor"
	}
	_ = r.flow.Set(ctx, state.Key, step, payload, r.cfg.FlowTTL)
	return step
}

// applyChoreDefinition fills the flow payload from an existing chore definition.
func applyChoreDefinition(payload *choreFlowPayload, d chores.Definition) {
	payload.DefID = d.ID
	payload.Name = d.Name
	payload.Emoji = d.Emoji
	if d.RecurrenceInterval == nil {
		return
	}
	payload.RecurrenceInterval = d.RecurrenceInterval
	payload.RecurrenceUnit = d.RecurrenceUnit
	payload.RecurrenceAnchor = d.RecurrenceAnchor
	if d.RecurrenceUnit != nil {
		if recType := recurrenceTypeForUnit(*d.RecurrenceUnit); recType != "" {
			payload.RecurrenceType = recType
		}
	}
}

func (r *Router) handleChoresFlowText(ctx context.Context, member *household.Member, msg *Message, state *flow.State) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	text := strings.TrimSpace(msg.Text)

	var payload choreFlowPayload
	_ = json.Unmarshal(state.Payload, &payload)

	// Typed answers, accepted or rejected, re-anchor the flow below the user's message.
	switch state.Step {
	case "chore_new_name":
		return r.handleChoreTypedName(ctx, state, locale, text, payload)
	case "chore_typed_emoji":
		return r.handleChoreTypedEmoji(ctx, state, locale, text, payload)
	case "chore_typed_interval":
		return r.handleChoreTypedInterval(ctx, state, locale, text, payload)
	case "chore_typed_date":
		return r.handleChoreTypedDate(ctx, state, locale, text, payload)
	case "chore_typed_minute":
		return r.handleChoreTypedMinute(ctx, state, locale, text, payload)
	}

	return nil
}

func (r *Router) handleChoreTypedName(ctx context.Context, state *flow.State, locale, text string, payload choreFlowPayload) error {
	if text == "" {
		return nil
	}
	payload.Name = text
	_ = r.flow.Set(ctx, state.Key, "chore_emoji", payload, r.cfg.FlowTTL)
	return r.reanchorFlow(ctx, state, func() error {
		return r.promptChoreEmoji(ctx, state, locale, payload)
	})
}

func (r *Router) handleChoreTypedEmoji(ctx context.Context, state *flow.State, locale, text string, payload choreFlowPayload) error {
	emoji, err := ValidateTypedEmoji(text)
	if err != nil {
		return r.reanchorRejected(ctx, state, locale, choreContext(payload),
			r.i18n.T(locale, "picker.invalid_emoji"), r.i18n.T(locale, "picker.prompt_type_emoji"))
	}
	payload.Emoji = emoji
	_ = r.flow.Set(ctx, state.Key, "chore_assignee", payload, r.cfg.FlowTTL)
	return r.reanchorFlow(ctx, state, func() error {
		return r.promptChoreAssignee(ctx, state, locale, payload)
	})
}

func (r *Router) handleChoreTypedInterval(ctx context.Context, state *flow.State, locale, text string, payload choreFlowPayload) error {
	n, err := ValidateTypedInterval(text)
	if err != nil {
		return r.reanchorRejected(ctx, state, locale, choreContext(payload),
			r.i18n.T(locale, "picker.invalid_interval"), r.i18n.T(locale, "picker.prompt_type_interval"))
	}
	payload.RecurrenceInterval = &n
	_ = r.flow.Set(ctx, state.Key, "chore_date", payload, r.cfg.FlowTTL)
	return r.reanchorFlow(ctx, state, func() error {
		return r.promptChoreDate(ctx, state, locale, &payload)
	})
}

func (r *Router) handleChoreTypedDate(ctx context.Context, state *flow.State, locale, text string, payload choreFlowPayload) error {
	dateOnly, dateTime, isDateTime, err := r.parseTypedDateOrTime(text)
	if err != nil {
		return r.reanchorRejected(ctx, state, locale, choreContext(payload),
			r.i18n.T(locale, "general.invalid_date_format"), r.i18n.T(locale, i18nKeyPickerPromptTypeDate))
	}
	if !isDateTime {
		payload.DueDate = dateOnly
		_ = r.flow.Set(ctx, state.Key, "chore_hour", payload, r.cfg.FlowTTL)
		return r.reanchorFlow(ctx, state, func() error {
			return r.promptChoreHour(ctx, state, locale, payload, "")
		})
	}

	now := r.clock.Now().In(r.clock.Location())
	if !dateTime.After(now) {
		return r.reanchorRejected(ctx, state, locale, choreContext(payload),
			r.i18n.T(locale, "general.past_due_time"), r.i18n.T(locale, i18nKeyPickerPromptTypeDate))
	}
	payload.DueAt = dateTime
	payload.DueDate = dateTime.Format(time.DateOnly)
	payload.DueHour = dateTime.Hour()
	payload.DueMinute = dateTime.Minute()

	step := r.storeChoreStepAfterTime(ctx, state, payload)
	return r.reanchorFlow(ctx, state, func() error {
		return r.promptChoreStep(ctx, state, locale, step, payload)
	})
}

func (r *Router) handleChoreTypedMinute(ctx context.Context, state *flow.State, locale, text string, payload choreFlowPayload) error {
	m, err := ValidateTypedMinute(text)
	if err != nil {
		return r.reanchorRejected(ctx, state, locale, choreContext(payload),
			r.i18n.T(locale, "picker.invalid_minute"), r.i18n.T(locale, "picker.prompt_type_minute"))
	}
	loc := r.clock.Location()
	dueAt := pickedDueAt(payload.DueDate, payload.DueHour, m, loc)
	now := r.clock.Now().In(loc)
	if !dueAt.After(now) {
		payload.DueHour = 0
		payload.DueMinute = 0
		_ = r.flow.Set(ctx, state.Key, "chore_hour", payload, r.cfg.FlowTTL)
		return r.reanchorFlow(ctx, state, func() error {
			return r.promptChoreHour(ctx, state, locale, payload, r.i18n.T(locale, "picker.time_passed"))
		})
	}
	payload.DueAt = dueAt
	payload.DueMinute = m

	step := r.storeChoreStepAfterTime(ctx, state, payload)
	return r.reanchorFlow(ctx, state, func() error {
		return r.promptChoreStep(ctx, state, locale, step, payload)
	})
}

func (r *Router) handleChoresFlowCallback(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	var payload choreFlowPayload
	_ = json.Unmarshal(state.Payload, &payload)

	switch parsed.Kind {
	case CallbackFlowCancel:
		return r.handleChoreCancel(ctx, cq, state, locale)
	case CallbackFlowNewItem:
		return r.handleChoreNewItem(ctx, cq, state, locale)
	case CallbackFlowEmoji:
		return r.handleChoreEmojiPick(ctx, cq, parsed, state, locale, payload)
	case CallbackFlowSelect:
		return r.handleChoreSelect(ctx, cq, parsed, state, locale, payload)
	case CallbackFlowUser:
		return r.handleChoreUser(ctx, cq, parsed, state, locale, payload)
	case CallbackFlowRecurrenceType:
		return r.handleChoreRecurrenceType(ctx, cq, parsed, state, locale, payload)
	case CallbackFlowInterval:
		return r.handleChoreInterval(ctx, cq, parsed, state, locale, payload)
	case CallbackFlowCalendar:
		return r.handleChoreCalendar(ctx, cq, parsed, state, locale, payload)
	case CallbackFlowDate:
		return r.handleChoreDate(ctx, cq, parsed, state, locale, payload)
	case CallbackFlowHour:
		return r.handleChoreHour(ctx, cq, parsed, state, locale, payload)
	case CallbackFlowMinute:
		return r.handleChoreMinute(ctx, cq, parsed, state, locale, payload)
	case CallbackFlowAnchor:
		return r.handleChoreAnchor(ctx, cq, parsed, state, locale, payload)
	case CallbackFlowConfirm:
		return r.handleChoreConfirm(ctx, member, cq, state, locale, payload)
	}

	return nil
}

func (r *Router) handleChoreCancel(ctx context.Context, cq *CallbackQuery, state *flow.State, locale string) error {
	_ = r.flow.Clear(ctx, state.Key)
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.cancelled"), false)
	_, err := r.client.SendMessage(ctx, cq.Message.Chat.ID, r.i18n.T(locale, "chores.flow_cancelled"), &SendOptions{
		ThreadID: cq.Message.MessageThreadID,
	})
	return err
}

func (r *Router) handleChoreNewItem(ctx context.Context, cq *CallbackQuery, state *flow.State, locale string) error {
	_ = r.flow.Set(ctx, state.Key, "chore_new_name", choreFlowPayload{}, r.cfg.FlowTTL)
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	prompt := r.i18n.T(locale, "chores.prompt_new_name")
	return r.showFlowMessage(ctx, state, prompt, r.cancelOnlyMarkup(locale))
}

func (r *Router) handleChoreEmojiPick(ctx context.Context, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State, locale string, payload choreFlowPayload) error {
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	switch parsed.Payload {
	case "o":
		_ = r.flow.Set(ctx, state.Key, "chore_typed_emoji", payload, r.cfg.FlowTTL)
		prompt := r.i18n.T(locale, "picker.prompt_type_emoji")
		return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), r.cancelOnlyMarkup(locale))
	case "skip":
		payload.Emoji = "🧹"
	default:
		suggestions := EmojiSuggestions(r.i18n, locale, "picker.emoji_suggestions_chores")
		if int(parsed.TargetID) >= len(suggestions) {
			// Out-of-range index: nothing changes, the step stays as it is.
			return nil
		}
		payload.Emoji = suggestions[parsed.TargetID]
	}
	_ = r.flow.Set(ctx, state.Key, "chore_assignee", payload, r.cfg.FlowTTL)
	return r.promptChoreAssignee(ctx, state, locale, payload)
}

func (r *Router) handleChoreSelect(ctx context.Context, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State, locale string, payload choreFlowPayload) error {
	defs, _ := r.chores.ListDefinitions(ctx)
	for _, d := range defs {
		if d.ID == parsed.TargetID {
			applyChoreDefinition(&payload, d)
			break
		}
	}
	_ = r.flow.Set(ctx, state.Key, "chore_assignee", payload, r.cfg.FlowTTL)
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	return r.promptChoreAssignee(ctx, state, locale, payload)
}

func (r *Router) handleChoreUser(ctx context.Context, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State, locale string, payload choreFlowPayload) error {
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
		_ = r.flow.Set(ctx, state.Key, "chore_date", payload, r.cfg.FlowTTL)
		return r.promptChoreDate(ctx, state, locale, &payload)
	}
	_ = r.flow.Set(ctx, state.Key, "chore_recurrence_type", payload, r.cfg.FlowTTL)
	return r.promptChoreRecurrenceType(ctx, state, locale, payload)
}

func (r *Router) handleChoreRecurrenceType(ctx context.Context, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State, locale string, payload choreFlowPayload) error {
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	payload.RecurrenceType = parsed.Payload
	if parsed.Payload == "once" {
		payload.RecurrenceInterval = nil
		payload.RecurrenceUnit = nil
		payload.RecurrenceAnchor = nil
		_ = r.flow.Set(ctx, state.Key, "chore_date", payload, r.cfg.FlowTTL)
		return r.promptChoreDate(ctx, state, locale, &payload)
	}
	unit := recurrenceUnitForType(parsed.Payload)
	payload.RecurrenceUnit = &unit
	_ = r.flow.Set(ctx, state.Key, "chore_interval", payload, r.cfg.FlowTTL)
	return r.promptChoreInterval(ctx, state, locale, payload)
}

func (r *Router) handleChoreInterval(ctx context.Context, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State, locale string, payload choreFlowPayload) error {
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	if parsed.Payload == "o" {
		_ = r.flow.Set(ctx, state.Key, "chore_typed_interval", payload, r.cfg.FlowTTL)
		prompt := r.i18n.T(locale, "picker.prompt_type_interval")
		return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), r.cancelOnlyMarkup(locale))
	}
	n := int(parsed.TargetID)
	payload.RecurrenceInterval = &n
	_ = r.flow.Set(ctx, state.Key, "chore_date", payload, r.cfg.FlowTTL)
	return r.promptChoreDate(ctx, state, locale, &payload)
}

func (r *Router) handleChoreCalendar(ctx context.Context, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State, locale string, payload choreFlowPayload) error {
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	calParts := strings.Split(parsed.Payload, "-")
	if len(calParts) == 2 {
		y, _ := strconv.Atoi(calParts[0])
		m, _ := strconv.Atoi(calParts[1])
		payload.CalYear = y
		payload.CalMonth = m
	}
	_ = r.flow.Set(ctx, state.Key, "chore_date", payload, r.cfg.FlowTTL)
	return r.promptChoreDate(ctx, state, locale, &payload)
}

func (r *Router) handleChoreDate(ctx context.Context, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State, locale string, payload choreFlowPayload) error {
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	nowInLoc := r.clock.Now().In(r.clock.Location())
	switch parsed.Payload {
	case "type":
		_ = r.flow.Set(ctx, state.Key, "chore_typed_date", payload, r.cfg.FlowTTL)
		prompt := r.i18n.T(locale, i18nKeyPickerPromptTypeDate)
		return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), r.cancelOnlyMarkup(locale))
	case "back":
		// Change date: back to the calendar on the chosen month, other choices kept.
		if day, err := time.ParseInLocation(time.DateOnly, payload.DueDate, r.clock.Location()); err == nil {
			payload.CalYear = day.Year()
			payload.CalMonth = int(day.Month())
		}
		payload.DueDate = ""
		_ = r.flow.Set(ctx, state.Key, "chore_date", payload, r.cfg.FlowTTL)
		return r.promptChoreDate(ctx, state, locale, &payload)
	case "today":
		payload.DueDate = nowInLoc.Format(time.DateOnly)
	case "tomorrow":
		payload.DueDate = nowInLoc.AddDate(0, 0, 1).Format(time.DateOnly)
	default:
		payload.DueDate = parsed.Payload
	}
	_ = r.flow.Set(ctx, state.Key, "chore_hour", payload, r.cfg.FlowTTL)
	return r.promptChoreHour(ctx, state, locale, payload, "")
}

func (r *Router) handleChoreHour(ctx context.Context, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State, locale string, payload choreFlowPayload) error {
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	payload.DueHour = int(parsed.TargetID)
	_ = r.flow.Set(ctx, state.Key, "chore_minute", payload, r.cfg.FlowTTL)
	return r.promptChoreMinute(ctx, state, locale, payload)
}

func (r *Router) handleChoreMinute(ctx context.Context, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State, locale string, payload choreFlowPayload) error {
	if parsed.Payload == "o" {
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		_ = r.flow.Set(ctx, state.Key, "chore_typed_minute", payload, r.cfg.FlowTTL)
		prompt := r.i18n.T(locale, "picker.prompt_type_minute")
		return r.showFlowMessage(ctx, state, r.flowStepText(locale, choreContext(payload), prompt), r.cancelOnlyMarkup(locale))
	}
	minute := int(parsed.TargetID)
	loc := r.clock.Location()
	dueAt := pickedDueAt(payload.DueDate, payload.DueHour, minute, loc)
	now := r.clock.Now().In(loc)
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	if !dueAt.After(now) {
		// The minute passed between rendering and pressing: explain it in the message.
		payload.DueHour = 0
		payload.DueMinute = 0
		_ = r.flow.Set(ctx, state.Key, "chore_hour", payload, r.cfg.FlowTTL)
		return r.promptChoreHour(ctx, state, locale, payload, r.i18n.T(locale, "picker.time_passed"))
	}
	payload.DueAt = dueAt
	payload.DueMinute = minute

	step := r.storeChoreStepAfterTime(ctx, state, payload)
	return r.promptChoreStep(ctx, state, locale, step, payload)
}

func (r *Router) handleChoreAnchor(ctx context.Context, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State, locale string, payload choreFlowPayload) error {
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	anc := parsed.Payload
	payload.RecurrenceAnchor = &anc
	_ = r.flow.Set(ctx, state.Key, "chore_confirm", payload, r.cfg.FlowTTL)
	return r.promptChoreConfirm(ctx, state, locale, payload)
}

func (r *Router) handleChoreConfirm(ctx context.Context, member *household.Member, cq *CallbackQuery, state *flow.State, locale string, payload choreFlowPayload) error {
	_ = r.flow.Clear(ctx, state.Key)
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.confirmed"), false)

	chatID := cq.Message.Chat.ID
	threadID := cq.Message.MessageThreadID

	defID := payload.DefID
	if defID == 0 {
		def, err := r.chores.CreateDefinition(ctx, member.ID, payload.Name, payload.Emoji, "", recurrenceRule(payload.RecurrenceInterval, payload.RecurrenceUnit, payload.RecurrenceAnchor))
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
