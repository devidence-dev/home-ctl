package telegram

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/px1/home-ctl/internal/flow"
	"github.com/px1/home-ctl/internal/household"
)

func (r *Router) handleFlowText(ctx context.Context, member *household.Member, msg *Message, state *flow.State) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	if state.ExpiresAt.Before(r.clock.Now()) {
		_ = r.flow.Clear(ctx, state.Key)
		_, err := r.client.SendMessage(ctx, msg.Chat.ID, r.i18n.T(locale, "general.flow_expired"), &SendOptions{
			ThreadID: msg.MessageThreadID,
		})
		return err
	}

	if strings.HasSuffix(state.Step, "_postpone_typed") {
		return r.handleTypedPostponeText(ctx, member, msg, state, locale)
	}
	if strings.HasPrefix(state.Step, "shopping_") {
		return r.handleShoppingFlowText(ctx, member, msg, state)
	}
	if strings.HasPrefix(state.Step, "chore_") {
		return r.handleChoresFlowText(ctx, member, msg, state)
	}
	if strings.HasPrefix(state.Step, "reminder_") {
		return r.handleRemindersFlowText(ctx, member, msg, state)
	}

	return nil
}

// showFlowMessage edits the active flow message, or sends a replacement if editing fails
// (except "message is not modified"). If forceSend is true or state.MessageID is 0,
// it sends a new message and records its id in state and flow persistence.
func (r *Router) showFlowMessage(ctx context.Context, state *flow.State, text string, markup *InlineKeyboardMarkup, forceSend ...bool) error {
	isForceSend := len(forceSend) > 0 && forceSend[0]
	if !isForceSend && state.MessageID != 0 {
		err := r.client.EditMessageText(ctx, state.Key.ChatID, state.MessageID, text, &EditOptions{
			ReplyMarkup: markup,
		})
		if err == nil {
			return nil
		}
		if strings.Contains(strings.ToLower(err.Error()), "message is not modified") {
			return nil
		}
		r.logger.Debug("failed to edit flow message, falling back to send",
			"chat_id", state.Key.ChatID, "message_id", state.MessageID, "error", err)
	}

	msgID, err := r.client.SendMessage(ctx, state.Key.ChatID, text, &SendOptions{
		ThreadID:    state.Key.ThreadID,
		ReplyMarkup: markup,
	})
	if err != nil {
		return err
	}

	state.MessageID = msgID
	if setErr := r.flow.SetMessageID(ctx, state.Key, msgID); setErr != nil {
		r.logger.Debug("failed to store flow message id", "error", setErr)
	}
	return nil
}

// reanchorFlow shows a step below the user's typed answer. render sends the step as a new flow
// message (state.MessageID is cleared first), then the previous flow message is retired.
func (r *Router) reanchorFlow(ctx context.Context, state *flow.State, render func() error) error {
	prevID := state.MessageID
	state.MessageID = 0
	if err := render(); err != nil {
		state.MessageID = prevID
		return err
	}
	r.retireFlowMessage(ctx, state.Key.ChatID, prevID, state.MessageID)
	return nil
}

// retireFlowMessage deletes a superseded flow message, or only removes its buttons when deletion fails.
// Telegram failures are logged and never change flow or domain state.
func (r *Router) retireFlowMessage(ctx context.Context, chatID, prevID, currentID int64) {
	if prevID == 0 || prevID == currentID {
		return
	}
	err := r.client.DeleteMessage(ctx, chatID, prevID)
	if err == nil {
		return
	}
	r.logger.Debug("failed to delete superseded flow message, removing its buttons",
		"chat_id", chatID, "message_id", prevID, "error", err)
	noKeyboard := &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{}}
	if err := r.client.EditMessageReplyMarkup(ctx, chatID, prevID, noKeyboard); err != nil {
		r.logger.Debug("failed to remove buttons of superseded flow message",
			"chat_id", chatID, "message_id", prevID, "error", err)
	}
}

// reanchorRejected re-anchors the flow with a rejection notice and the typed prompt asked again.
func (r *Router) reanchorRejected(ctx context.Context, state *flow.State, locale string, pc PickerContext, notice, prompt string) error {
	return r.reanchorFlow(ctx, state, func() error {
		return r.showFlowMessage(ctx, state, r.flowStepText(locale, pc, notice, prompt), r.cancelOnlyMarkup(locale))
	})
}

// flowStepText joins the context header of values already chosen with the non-empty parts of a step.
func (r *Router) flowStepText(locale string, pc PickerContext, parts ...string) string {
	all := make([]string, 0, len(parts)+1)
	if header := RenderContextHeader(r.i18n, locale, pc, r.clock.Now(), r.clock.Location()); header != "" {
		all = append(all, header)
	}
	for _, p := range parts {
		if p != "" {
			all = append(all, p)
		}
	}
	return strings.Join(all, "\n\n")
}

func (r *Router) closeCallbackKeyboard(ctx context.Context, msg *Message) {
	if msg == nil {
		return
	}
	text := msg.Text
	if text == "" {
		text = " "
	}
	err := r.client.EditMessageText(ctx, msg.Chat.ID, msg.MessageID, text, &EditOptions{
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{}},
	})
	if err != nil {
		r.logger.Debug("failed to close flow keyboard", "chat_id", msg.Chat.ID, "message_id", msg.MessageID, "error", err)
	}
}

func (r *Router) handleFlowCallback(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	if cq.Message == nil {
		return nil
	}

	chatID := cq.Message.Chat.ID
	threadID := cq.Message.MessageThreadID
	msgID := cq.Message.MessageID

	// 1. The pressed message is authoritative: a callback may only act on the flow shown in it.
	state, err := r.flow.GetByMessage(ctx, chatID, threadID, msgID, nil)
	if err != nil {
		state = nil
	}

	// If not found or expired
	if state == nil {
		r.closeCallbackKeyboard(ctx, cq.Message)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.flow_expired"), true)
		return nil
	}

	// 2. Ownership enforcement: check actor vs owner
	if state.Key.UserID != member.ID {
		// Non-owner press: answer with localized notice, no side effects (no edit, no state change, no keyboard close)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "picker.ownership_notice"), true)
		return nil
	}

	// 3. Inert callback (e.g. fl:noop on calendar navigation or headers)
	if parsed.Kind == CallbackFlowNoop {
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return nil
	}

	prevStep := state.Step
	var handlerErr error
	if strings.HasPrefix(state.Step, "shopping_") {
		handlerErr = r.handleShoppingFlowCallback(ctx, member, cq, parsed, state)
	} else if strings.HasPrefix(state.Step, "chore_") {
		handlerErr = r.handleChoresFlowCallback(ctx, member, cq, parsed, state)
	} else if strings.HasPrefix(state.Step, "reminder_") {
		handlerErr = r.handleRemindersFlowCallback(ctx, member, cq, parsed, state)
	}

	if handlerErr != nil {
		return handlerErr
	}

	newState, getErr := r.flow.Get(ctx, state.Key, nil)
	if getErr != nil || newState == nil {
		// Flow was completed or cleared: close keyboard
		r.closeCallbackKeyboard(ctx, cq.Message)
	} else if newState.Step != prevStep && (newState.MessageID == 0 || newState.MessageID != cq.Message.MessageID) {
		// Step changed and the new step is not editing this message: close keyboard on previous prompt
		r.closeCallbackKeyboard(ctx, cq.Message)
	}

	return nil
}

func (r *Router) cancelOnlyMarkup(locale string) *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"}},
		},
	}
}

func (r *Router) parseTypedDateOrTime(input string) (dateOnly string, dateTime time.Time, isDateTime bool, err error) {
	trimmed := strings.TrimSpace(input)
	loc := r.clock.Location()
	now := r.clock.Now().In(loc)

	// Try YYYY-MM-DD HH:MM
	if t, pErr := time.ParseInLocation("2006-01-02 15:04", trimmed, loc); pErr == nil {
		return "", t, true, nil
	}

	// Try HH:MM for today
	if t, pErr := time.ParseInLocation("15:04", trimmed, loc); pErr == nil {
		combined := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, loc)
		return "", combined, true, nil
	}

	// Try YYYY-MM-DD
	if t, pErr := time.ParseInLocation("2006-01-02", trimmed, loc); pErr == nil {
		return t.Format("2006-01-02"), time.Time{}, false, nil
	}

	return "", time.Time{}, false, errors.New("invalid date format")
}
