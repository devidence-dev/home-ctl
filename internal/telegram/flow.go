package telegram

import (
	"context"
	"strings"

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

func (r *Router) closeCallbackKeyboard(ctx context.Context, msg *Message) {
	if msg == nil || msg.Text == "" {
		return
	}
	err := r.client.EditMessageText(ctx, msg.Chat.ID, msg.MessageID, msg.Text, &EditOptions{
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

	flowKey := flow.Key{
		UserID:   member.ID,
		ChatID:   cq.Message.Chat.ID,
		ThreadID: cq.Message.MessageThreadID,
	}

	state, err := r.flow.Get(ctx, flowKey, nil)
	if err != nil || state == nil {
		r.closeCallbackKeyboard(ctx, cq.Message)
		if parsed.Kind == CallbackFlowCancel {
			_ = r.flow.Clear(ctx, flowKey)
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.cancelled"), false)
			return nil
		}
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.flow_expired"), true)
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

	newState, getErr := r.flow.Get(ctx, flowKey, nil)
	if getErr != nil || newState == nil || newState.Step != prevStep {
		r.closeCallbackKeyboard(ctx, cq.Message)
	}

	return nil
}
