package telegram

import (
	"context"
	"fmt"
	"time"

	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/i18n"
)

// Notifier implements notification.Notifier using Telegram Bot API.
type Notifier struct {
	client        Client
	household     *household.Service
	i18n          *i18n.Bundle
	chatID        int64
	defaultLocale string
}

// NewNotifier creates a new Telegram Notifier.
func NewNotifier(client Client, household *household.Service, i18n *i18n.Bundle, chatID int64, defaultLocale string) *Notifier {
	if defaultLocale == "" {
		defaultLocale = "en"
	}
	return &Notifier{
		client:        client,
		household:     household,
		i18n:          i18n,
		chatID:        chatID,
		defaultLocale: defaultLocale,
	}
}

// SendPrivateNotification sends a due notification with Done and Postpone buttons to a member.
func (n *Notifier) SendPrivateNotification(ctx context.Context, targetUserID int64, entityType string, occID int64, title string, _ time.Time) (int64, error) {
	locale := n.defaultLocale
	if n.household != nil {
		member, err := n.household.Authorize(ctx, targetUserID)
		if err == nil && member != nil && member.Locale != "" {
			locale = member.Locale
		}
	}

	var text string
	var doneData, postponeData string

	if entityType == "chore" {
		text = n.i18n.T(locale, "notifications.chore_due", title)
		doneData = fmt.Sprintf("ch:c:%d", occID)
		postponeData = fmt.Sprintf("ch:p:%d", occID)
	} else {
		text = n.i18n.T(locale, "notifications.reminder_due", title)
		doneData = fmt.Sprintf("re:c:%d", occID)
		postponeData = fmt.Sprintf("re:p:%d", occID)
	}

	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: n.i18n.T(locale, "buttons.done"), CallbackData: doneData},
				{Text: n.i18n.T(locale, "buttons.postpone"), CallbackData: postponeData},
			},
		},
	}

	return n.client.SendMessage(ctx, targetUserID, text, &SendOptions{
		ReplyMarkup: markup,
	})
}

// SendGroupTopicUpdate sends a message to the group chat within a specific topic thread.
func (n *Notifier) SendGroupTopicUpdate(ctx context.Context, topicID int64, text string) (int64, error) {
	return n.client.SendMessage(ctx, n.chatID, text, &SendOptions{
		ThreadID: topicID,
	})
}
