package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/px1/home-ctl/internal/household"
)

func (r *Router) handleStart(ctx context.Context, member *household.Member, msg *Message) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	text := r.i18n.T(locale, "help.text")
	_, err := r.client.SendMessage(ctx, msg.Chat.ID, text, nil)
	return err
}

func (r *Router) handleHelp(ctx context.Context, member *household.Member, msg *Message) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	text := r.i18n.T(locale, "help.text")
	opts := &SendOptions{}
	if msg.MessageThreadID != 0 {
		opts.ThreadID = msg.MessageThreadID
	}
	_, err := r.client.SendMessage(ctx, msg.Chat.ID, text, opts)
	return err
}

func (r *Router) handleStatus(ctx context.Context, member *household.Member, msg *Message) error {
	locale := member.Locale
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}

	statuses, err := r.household.GetStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get member statuses: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(r.i18n.T(locale, "status.header"))
	sb.WriteString("\n")

	for _, s := range statuses {
		name := s.Member.Name
		if name == "" {
			name = s.Member.Username
		}
		if s.Connected {
			sb.WriteString(r.i18n.T(locale, "status.connected", name))
		} else {
			sb.WriteString(r.i18n.T(locale, "status.not_connected", name))
		}
		sb.WriteString("\n")
	}

	opts := &SendOptions{}
	if msg.MessageThreadID != 0 {
		opts.ThreadID = msg.MessageThreadID
	}
	_, err = r.client.SendMessage(ctx, msg.Chat.ID, strings.TrimRight(sb.String(), "\n"), opts)
	return err
}
