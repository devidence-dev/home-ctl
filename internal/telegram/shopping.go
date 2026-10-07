package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/px1/home-ctl/internal/flow"
	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/shopping"
)

type shoppingNewItemPayload struct {
	Name  string `json:"name,omitempty"`
	Emoji string `json:"emoji,omitempty"`
}

func (r *Router) handleShoppingAdd(ctx context.Context, member *household.Member, msg *Message) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	items, err := r.shopping.ListItems(ctx)
	if err != nil {
		return err
	}

	flowKey := flow.Key{
		UserID:   member.ID,
		ChatID:   msg.Chat.ID,
		ThreadID: msg.MessageThreadID,
	}

	if len(items) == 0 {
		// No items in catalog, immediately ask for new item name
		_ = r.flow.Set(ctx, flowKey, "shopping_new_name", shoppingNewItemPayload{}, r.cfg.FlowTTL)
		text := r.i18n.T(locale, "shopping.prompt_new_item_name")
		markup := &InlineKeyboardMarkup{
			InlineKeyboard: [][]InlineKeyboardButton{
				{{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"}},
			},
		}
		_, err := r.client.SendMessage(ctx, msg.Chat.ID, text, &SendOptions{
			ThreadID:    msg.MessageThreadID,
			ReplyMarkup: markup,
		})
		return err
	}

	// Catalog has items: show selectable items plus Add new item and Cancel
	_ = r.flow.Set(ctx, flowKey, "shopping_select_items", nil, r.cfg.FlowTTL)
	var rows [][]InlineKeyboardButton
	var currentRow []InlineKeyboardButton
	for _, item := range items {
		btn := InlineKeyboardButton{
			Text:         fmt.Sprintf("%s %s", item.Emoji, item.Name),
			CallbackData: fmt.Sprintf("fl:sel:%d", item.ID),
		}
		currentRow = append(currentRow, btn)
		if len(currentRow) == 2 {
			rows = append(rows, currentRow)
			currentRow = nil
		}
	}
	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}

	rows = append(rows, []InlineKeyboardButton{
		{Text: r.i18n.T(locale, "buttons.add_new_item"), CallbackData: "fl:new"},
		{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
	})

	text := r.i18n.T(locale, "shopping.prompt_select")
	_, err = r.client.SendMessage(ctx, msg.Chat.ID, text, &SendOptions{
		ThreadID:    msg.MessageThreadID,
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

func (r *Router) handleShoppingList(ctx context.Context, member *household.Member, msg *Message) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	entries, err := r.shopping.ListPending(ctx)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		text := r.i18n.T(locale, "shopping.list_empty")
		_, err := r.client.SendMessage(ctx, msg.Chat.ID, text, &SendOptions{
			ThreadID: msg.MessageThreadID,
		})
		return err
	}

	text := r.i18n.T(locale, "shopping.list_title")
	var rows [][]InlineKeyboardButton
	for _, entry := range entries {
		rows = append(rows, []InlineKeyboardButton{
			{
				Text:         fmt.Sprintf("%s %s", entry.Item.Emoji, entry.Item.Name),
				CallbackData: fmt.Sprintf("sh:p:%d", entry.ID),
			},
		})
	}

	_, err = r.client.SendMessage(ctx, msg.Chat.ID, text, &SendOptions{
		ThreadID:    msg.MessageThreadID,
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

func (r *Router) handleShoppingPurchaseCallback(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	now := r.clock.Now().UTC()
	entry, err := r.shopping.Purchase(ctx, parsed.ActorID, parsed.TargetID, now)
	if err != nil {
		if errors.Is(err, shopping.ErrAlreadyPurchased) {
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "shopping.already_purchased", r.i18n.T(locale, "shopping.someone")), true)
		} else {
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.userErrorText(locale, err), true)
		}
		// Refresh list message in group topic
		return r.refreshShoppingListMessage(ctx, cq, locale)
	}

	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "shopping.purchased_notice"), false)

	// Post group topic update
	actorName := member.Name
	if actorName == "" {
		actorName = member.Username
	}
	groupMsg := r.i18n.T(r.cfg.DefaultLocale, "group.shopping_purchased", entry.Item.Emoji, entry.Item.Name, actorName)
	_, _ = r.client.SendMessage(ctx, r.cfg.ChatID, groupMsg, &SendOptions{
		ThreadID: r.cfg.ShoppingTopicID,
	})

	// Refresh list message
	return r.refreshShoppingListMessage(ctx, cq, locale)
}

func (r *Router) refreshShoppingListMessage(ctx context.Context, cq *CallbackQuery, locale string) error {
	if cq.Message == nil {
		return nil
	}

	entries, err := r.shopping.ListPending(ctx)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, r.i18n.T(locale, "shopping.list_empty"), &EditOptions{
			ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{}},
		})
	}

	var rows [][]InlineKeyboardButton
	for _, entry := range entries {
		rows = append(rows, []InlineKeyboardButton{
			{
				Text:         fmt.Sprintf("%s %s", entry.Item.Emoji, entry.Item.Name),
				CallbackData: fmt.Sprintf("sh:p:%d", entry.ID),
			},
		})
	}

	return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, r.i18n.T(locale, "shopping.list_title"), &EditOptions{
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

func (r *Router) handleShoppingFlowText(ctx context.Context, member *household.Member, msg *Message, state *flow.State) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	flowKey := state.Key
	text := strings.TrimSpace(msg.Text)

	switch state.Step {
	case "shopping_new_name":
		if text == "" {
			return nil
		}
		payload := shoppingNewItemPayload{Name: text}
		_ = r.flow.Set(ctx, flowKey, "shopping_new_emoji", payload, r.cfg.FlowTTL)
		prompt := r.i18n.T(locale, "shopping.prompt_new_item_emoji")
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

	case "shopping_new_emoji":
		var payload shoppingNewItemPayload
		_ = json.Unmarshal(state.Payload, &payload)
		emoji := text
		if emoji == "" {
			emoji = "🛒"
		}
		item, err := r.shopping.CreateItem(ctx, member.ID, payload.Name, emoji)
		_ = r.flow.Clear(ctx, flowKey)
		if err != nil {
			if errors.Is(err, shopping.ErrItemAlreadyExists) {
				_, _ = r.client.SendMessage(ctx, msg.Chat.ID, r.i18n.T(locale, "shopping.item_already_exists", payload.Name), &SendOptions{
					ThreadID: msg.MessageThreadID,
				})
				return nil
			}
			_, _ = r.client.SendMessage(ctx, msg.Chat.ID, r.userErrorText(locale, err), &SendOptions{
				ThreadID: msg.MessageThreadID,
			})
			return nil
		}

		createdMsg := r.i18n.T(locale, "shopping.item_created", item.Emoji, item.Name)
		_, err = r.client.SendMessage(ctx, msg.Chat.ID, createdMsg, &SendOptions{
			ThreadID: msg.MessageThreadID,
		})
		return err
	}

	return nil
}

func (r *Router) handleShoppingFlowCallback(ctx context.Context, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback, state *flow.State) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	flowKey := state.Key

	switch parsed.Kind {
	case CallbackFlowCancel:
		_ = r.flow.Clear(ctx, flowKey)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.cancelled"), false)
		return nil

	case CallbackFlowNewItem:
		_ = r.flow.Set(ctx, flowKey, "shopping_new_name", shoppingNewItemPayload{}, r.cfg.FlowTTL)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		prompt := r.i18n.T(locale, "shopping.prompt_new_item_name")
		markup := &InlineKeyboardMarkup{
			InlineKeyboard: [][]InlineKeyboardButton{
				{{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"}},
			},
		}
		_, err := r.client.SendMessage(ctx, cq.Message.Chat.ID, prompt, &SendOptions{
			ThreadID:    cq.Message.MessageThreadID,
			ReplyMarkup: markup,
		})
		return err

	case CallbackFlowSelect:
		// User selected item to add to pending list
		entries, err := r.shopping.AddToList(ctx, member.ID, []int64{parsed.TargetID})
		if err != nil {
			if errors.Is(err, shopping.ErrEntryAlreadyPending) {
				_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "shopping.item_already_pending", r.i18n.T(locale, "shopping.item_fallback")), true)
				return nil
			}
			_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.userErrorText(locale, err), true)
			return nil
		}

		_ = r.flow.Clear(ctx, flowKey)
		added := r.i18n.T(locale, "shopping.items_added", entries[0].Item.Emoji, entries[0].Item.Name)
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, added, false)
		_, err = r.client.SendMessage(ctx, cq.Message.Chat.ID, added, &SendOptions{
			ThreadID: cq.Message.MessageThreadID,
		})
		return err
	}

	return nil
}
