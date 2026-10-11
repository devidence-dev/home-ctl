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
	"github.com/px1/home-ctl/internal/reminders"
)

func (r *Router) handleListCommand(ctx context.Context, kind *listKind, member *household.Member, msg *Message) error {
	locale := r.memberLocale(member)

	connected, err := r.household.IsConnected(ctx, member.ID)
	if err != nil || !connected {
		return r.sendUnconnectedNotice(ctx, locale, msg)
	}

	return r.sendPrivateList(ctx, kind, member, member.ID)
}

// sendUnconnectedNotice tells a member in a topic how to connect. When the bot username is known,
// the "Connect with the bot" button opens the private chat and sends /start connect in one tap.
func (r *Router) sendUnconnectedNotice(ctx context.Context, locale string, msg *Message) error {
	opts := &SendOptions{ThreadID: msg.MessageThreadID}
	if r.cfg.BotUsername != "" {
		opts.ReplyMarkup = &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{
			Text: r.i18n.T(locale, "general.connect_button"),
			URL:  "https://t.me/" + r.cfg.BotUsername + "?start=connect",
		}}}}
	}
	_, err := r.client.SendMessage(ctx, msg.Chat.ID, r.i18n.T(locale, "general.unconnected_notice"), opts)
	return err
}

func (r *Router) handlePrivateList(ctx context.Context, member *household.Member, msg *Message) error {
	return r.sendPrivateList(ctx, r.choresKind(), member, msg.Chat.ID)
}

// sendPrivateList sends a new private list message with the member's open occurrences of one kind.
func (r *Router) sendPrivateList(ctx context.Context, kind *listKind, member *household.Member, targetChatID int64) error {
	occs, remaining, err := kind.list(ctx, member.ID)
	if err != nil {
		return err
	}

	text, markup := r.formatList(kind, r.memberLocale(member), occs, remaining)
	_, err = r.client.SendMessage(ctx, targetChatID, text, &SendOptions{
		ReplyMarkup: markup,
	})
	return err
}

// memberLocale is the locale a member's messages are rendered in.
func (r *Router) memberLocale(member *household.Member) string {
	if member.Locale != "" {
		return member.Locale
	}
	return r.cfg.DefaultLocale
}

// memberName is the name a member is shown by: their name, else their username.
func memberName(member *household.Member) string {
	if member.Name != "" {
		return member.Name
	}
	return member.Username
}

// listItem is one entry of a private list, resolved from its domain.
type listItem struct {
	emoji, name, recurrence, callback string
	dueAt                             time.Time
}

// listViewSwitch is the row that moves a private list between chores and reminders.
// The list title already names the current view, so neither button is marked.
func (r *Router) listViewSwitch(locale string) []InlineKeyboardButton {
	return []InlineKeyboardButton{
		{Text: r.i18n.T(locale, "lists.view_chores"), CallbackData: "list:ch"},
		{Text: r.i18n.T(locale, "lists.view_reminders"), CallbackData: "list:re"},
	}
}

// listGroup is a traffic-light day group of a private list. The values are in display order.
type listGroup int

const (
	groupOverdue listGroup = iota
	groupToday
	groupTomorrow
	groupUpcoming
)

// listGroupKeys is the header key of each listGroup.
var listGroupKeys = [...]string{"lists.group_overdue", "lists.group_today", "lists.group_tomorrow", "lists.group_upcoming"}

// listGroupOf assigns a due time to its group. An item earlier today that is already past is overdue.
func (r *Router) listGroupOf(due time.Time) listGroup {
	switch {
	case due.Before(r.clock.Now()):
		return groupOverdue
	case due.Before(r.dayAt(1, timeOfDay{})):
		return groupToday
	case due.Before(r.dayAt(2, timeOfDay{})):
		return groupTomorrow
	}
	return groupUpcoming
}

// renderPrivateList renders the title, then the entries grouped by due day under their headers, one
// unnumbered button per entry in body order, followed by the view switch. The "+N more" line closes the body.
// Items arrive sorted by due time, so each group is contiguous and empty groups never appear.
func (r *Router) renderPrivateList(locale, title string, remaining int, items []listItem) (string, *InlineKeyboardMarkup) {
	var sb strings.Builder
	sb.WriteString(title)
	rows := [][]InlineKeyboardButton{}
	prev := listGroup(-1)
	for _, item := range items {
		sb.WriteString("\n\n")
		group := r.listGroupOf(item.dueAt)
		if group != prev {
			prev = group
			sb.WriteString(r.i18n.T(locale, listGroupKeys[group]) + "\n")
		}
		sb.WriteString(r.renderListEntry(locale, group, item))
		rows = append(rows, []InlineKeyboardButton{
			{Text: fmt.Sprintf("%s %s", item.emoji, item.name), CallbackData: item.callback},
		})
	}
	if remaining > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(r.i18n.T(locale, "chores.remaining_count", remaining))
	}
	rows = append(rows, r.listViewSwitch(locale))
	return sb.String(), &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// renderListEntry renders "emoji name" and, below it, the time line: "╰┈➤ 18:00". Overdue and upcoming
// items put the day in front ("╰┈➤ ayer · 18:00"); recurring items append the recurrence summary.
func (r *Router) renderListEntry(locale string, group listGroup, item listItem) string {
	loc := r.clock.Location()
	when := item.dueAt.In(loc).Format("15:04")
	if group == groupOverdue || group == groupUpcoming {
		when = r.i18n.FormatDay(locale, item.dueAt, r.clock.Now(), loc) + " · " + when
	}
	if item.recurrence != "" {
		when = r.i18n.T(locale, "lists.entry_recurrence", when, item.recurrence)
	}
	return fmt.Sprintf("%s %s\n%s", item.emoji, item.name, r.i18n.T(locale, "lists.entry_detail", when))
}

// listKind is everything the chores list and the reminders list differ in. Every list action is written
// once against it; choresKind and remindersKind are the only places the two kinds are spelled out.
type listKind struct {
	prefix      string         // callback prefix of the entry buttons: "ch" or "re"
	emoji       string         // emoji of an entry whose definition is gone
	keys        listKeys       // i18n keys the kind renders with
	cb          listCallbacks  // callback kinds the kind's buttons parse back into
	topic       int64          // Telegram topic that receives completion and postponement notices
	typedStep   string         // flow step that waits for a typed postponement time
	short       postponePreset // the kind's own first postpone preset; the other presets are shared
	errNotOwner error          // domain error for a member who may not act on the entry
	errResolved error          // domain error for an occurrence that is already resolved

	list     func(ctx context.Context, memberID int64) ([]listOcc, int, error)
	get      func(ctx context.Context, occID int64) (listOcc, bool)
	complete func(ctx context.Context, actorID, occID int64, now time.Time) (listOcc, error)
	postpone func(ctx context.Context, actorID, occID int64, due time.Time) (listOcc, error)
}

// listKeys are the i18n keys of one listKind.
type listKeys struct {
	title, empty, detail string
	fallback             string // name of an entry whose definition is gone
	notOwner             string // answer to a member who may not act on the entry
	completed            string // group topic message for a completion
	postponed            string // group topic message for a postponement
	postponedNotice      string // callback answer after a postponement
}

// listCallbacks are the callback kinds of one listKind.
type listCallbacks struct {
	list, detail, complete, postpone CallbackKind
}

// postponePreset is a relative postponement choice: its button key, its callback payload and its delay.
type postponePreset struct {
	key, value string
	delay      time.Duration
}

// listOcc is one occurrence of either kind, reduced to what the lists and their actions show.
type listOcc struct {
	id          int64
	status      string
	dueAt       time.Time
	defined     bool // false when the definition is gone, so the kind's fallback emoji and name apply
	emoji, name string
	interval    *int
	unit        *string
}

// choreOcc reduces a chore occurrence to a listOcc. A nil occurrence yields the zero value.
func choreOcc(occ *chores.Occurrence) listOcc {
	if occ == nil {
		return listOcc{}
	}
	v := listOcc{id: occ.ID, status: occ.Status, dueAt: occ.DueAt}
	if d := occ.Definition; d != nil {
		v.defined, v.emoji, v.name, v.interval, v.unit = true, d.Emoji, d.Name, d.RecurrenceInterval, d.RecurrenceUnit
	}
	return v
}

// reminderOcc reduces a reminder occurrence to a listOcc. A nil occurrence yields the zero value.
func reminderOcc(occ *reminders.Occurrence) listOcc {
	if occ == nil {
		return listOcc{}
	}
	v := listOcc{id: occ.ID, status: occ.Status, dueAt: occ.DueAt}
	if d := occ.Definition; d != nil {
		v.defined, v.emoji, v.name, v.interval, v.unit = true, d.Emoji, d.Title, d.RecurrenceInterval, d.RecurrenceUnit
	}
	return v
}

// choresKind binds the chores list to this router's chores service and topic.
func (r *Router) choresKind() *listKind {
	return &listKind{
		prefix: "ch",
		emoji:  "🧹",
		keys: listKeys{
			title: "chores.list_title", empty: "chores.list_empty", detail: "chores.detail_view",
			fallback: i18nKeyChoresFallbackName, notOwner: i18nKeyChoresNotAssignee,
			completed: "group.chore_completed", postponed: "group.chore_postponed", postponedNotice: "chores.postponed_notice",
		},
		cb: listCallbacks{
			list: CallbackListChores, detail: CallbackChoreDetail, complete: CallbackChoreComplete, postpone: CallbackChorePostpone,
		},
		topic:       r.cfg.ChoresTopicID,
		typedStep:   "chore_postpone_typed",
		short:       postponePreset{key: "buttons.postpone_30m", value: "30m", delay: 30 * time.Minute},
		errNotOwner: chores.ErrNotAssignee,
		errResolved: chores.ErrOccurrenceResolved,
		list: func(ctx context.Context, memberID int64) ([]listOcc, int, error) {
			res, err := r.chores.ListMyOpen(ctx, memberID)
			occs := make([]listOcc, 0, len(res.Occurrences))
			for i := range res.Occurrences {
				occs = append(occs, choreOcc(&res.Occurrences[i]))
			}
			return occs, res.RemainingCount, err
		},
		get: func(ctx context.Context, occID int64) (listOcc, bool) {
			occ, err := r.chores.GetOccurrence(ctx, occID)
			if err != nil || occ == nil || occ.Status != "open" {
				return listOcc{}, false
			}
			return choreOcc(occ), true
		},
		complete: func(ctx context.Context, actorID, occID int64, now time.Time) (listOcc, error) {
			occ, _, err := r.chores.Complete(ctx, actorID, occID, now)
			return choreOcc(occ), err
		},
		postpone: func(ctx context.Context, actorID, occID int64, due time.Time) (listOcc, error) {
			occ, err := r.chores.Postpone(ctx, actorID, occID, due)
			return choreOcc(occ), err
		},
	}
}

// remindersKind binds the reminders list to this router's reminders service and topic.
func (r *Router) remindersKind() *listKind {
	return &listKind{
		prefix: "re",
		emoji:  "⏰",
		keys: listKeys{
			title: "reminders.list_title", empty: "reminders.list_empty", detail: "reminders.detail_view",
			fallback: i18nKeyRemindersFallbackTitle, notOwner: i18nKeyRemindersNotTarget,
			completed: "group.reminder_completed", postponed: "group.reminder_postponed", postponedNotice: "reminders.postponed_notice",
		},
		cb: listCallbacks{
			list: CallbackListReminders, detail: CallbackReminderDetail, complete: CallbackReminderComplete, postpone: CallbackReminderPostpone,
		},
		topic:       r.cfg.RemindersTopicID,
		typedStep:   "reminder_postpone_typed",
		short:       postponePreset{key: "buttons.postpone_15m", value: "15m", delay: 15 * time.Minute},
		errNotOwner: reminders.ErrNotTarget,
		errResolved: reminders.ErrOccurrenceResolved,
		list: func(ctx context.Context, memberID int64) ([]listOcc, int, error) {
			res, err := r.reminders.ListMyOpen(ctx, memberID)
			occs := make([]listOcc, 0, len(res.Occurrences))
			for i := range res.Occurrences {
				occs = append(occs, reminderOcc(&res.Occurrences[i]))
			}
			return occs, res.RemainingCount, err
		},
		get: func(ctx context.Context, occID int64) (listOcc, bool) {
			occ, err := r.reminders.GetOccurrence(ctx, occID)
			if err != nil || occ == nil || occ.Status != "open" {
				return listOcc{}, false
			}
			return reminderOcc(occ), true
		},
		complete: func(ctx context.Context, actorID, occID int64, now time.Time) (listOcc, error) {
			occ, _, err := r.reminders.Complete(ctx, actorID, occID, now)
			return reminderOcc(occ), err
		},
		postpone: func(ctx context.Context, actorID, occID int64, due time.Time) (listOcc, error) {
			occ, err := r.reminders.Postpone(ctx, actorID, occID, due)
			return reminderOcc(occ), err
		},
	}
}

// entryLabel returns the emoji and name of an occurrence, with the kind's fallbacks when its definition is gone.
func (r *Router) entryLabel(kind *listKind, locale string, occ listOcc) (string, string) {
	if !occ.defined {
		return kind.emoji, r.i18n.T(locale, kind.keys.fallback)
	}
	return occ.emoji, occ.name
}

// entryRecurrence returns the recurrence summary of an occurrence; ok is false for a one-time definition.
func (r *Router) entryRecurrence(locale string, occ listOcc) (summary string, ok bool) {
	if occ.interval == nil || occ.unit == nil {
		return "", false
	}
	return r.recurrenceSummary(locale, *occ.interval, *occ.unit, nil), true
}

// formatList renders the body and keyboard of a private list of one kind.
func (r *Router) formatList(kind *listKind, locale string, occs []listOcc, remaining int) (string, *InlineKeyboardMarkup) {
	if len(occs) == 0 {
		return r.i18n.T(locale, kind.keys.empty), &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{r.listViewSwitch(locale)}}
	}

	items := make([]listItem, 0, len(occs))
	for _, occ := range occs {
		emoji, name := r.entryLabel(kind, locale, occ)
		item := listItem{
			emoji:    emoji,
			name:     name,
			callback: fmt.Sprintf("%s:d:%d", kind.prefix, occ.id),
			dueAt:    occ.dueAt,
		}
		item.recurrence, _ = r.entryRecurrence(locale, occ)
		items = append(items, item)
	}
	return r.renderPrivateList(locale, r.i18n.T(locale, kind.keys.title), remaining, items)
}

// handleListCallback routes the callbacks of one list kind: its list, detail, completion and postponement.
func (r *Router) handleListCallback(ctx context.Context, kind *listKind, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback) error {
	locale := r.memberLocale(member)

	switch parsed.Kind {
	case kind.cb.list:
		return r.showList(ctx, kind, member, cq, locale)
	case kind.cb.detail:
		return r.showListDetail(ctx, kind, member, cq, parsed.TargetID, locale)
	case kind.cb.complete:
		return r.completeListItem(ctx, kind, member, cq, parsed, locale)
	case kind.cb.postpone:
		return r.postponeListItem(ctx, kind, member, cq, parsed, locale)
	}

	return nil
}

// answerAndEdit acknowledges a callback and edits its message to text and markup.
func (r *Router) answerAndEdit(ctx context.Context, cq *CallbackQuery, text string, markup *InlineKeyboardMarkup) error {
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{
		ReplyMarkup: markup,
	})
}

// showList edits the message back to the member's open occurrences of one kind.
func (r *Router) showList(ctx context.Context, kind *listKind, member *household.Member, cq *CallbackQuery, locale string) error {
	occs, remaining, err := kind.list(ctx, member.ID)
	if err != nil {
		return err
	}
	text, markup := r.formatList(kind, locale, occs, remaining)
	return r.answerAndEdit(ctx, cq, text, markup)
}

// showListDetail edits the message to one open occurrence; a stale occurrence re-renders the list instead.
func (r *Router) showListDetail(ctx context.Context, kind *listKind, member *household.Member, cq *CallbackQuery, occID int64, locale string) error {
	occ, ok := kind.get(ctx, occID)
	if !ok {
		// Stale detail: re-render current list
		occs, remaining, _ := kind.list(ctx, member.ID)
		text, markup := r.formatList(kind, locale, occs, remaining)
		return r.answerAndEdit(ctx, cq, text, markup)
	}

	emoji, name := r.entryLabel(kind, locale, occ)
	recSummary, ok := r.entryRecurrence(locale, occ)
	if !ok {
		recSummary = r.i18n.T(locale, "general.recurrence_one_off")
	}

	dueStr := r.formatDateTime(locale, occ.dueAt)
	detailText := r.i18n.T(locale, kind.keys.detail, emoji, name, dueStr, memberName(member), recSummary)
	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: r.i18n.T(locale, "buttons.done"), CallbackData: fmt.Sprintf("%s:c:%d", kind.prefix, occ.id)},
				{Text: r.i18n.T(locale, "buttons.postpone"), CallbackData: fmt.Sprintf("%s:p:%d", kind.prefix, occ.id)},
			},
			{
				{Text: r.i18n.T(locale, i18nKeyButtonBack), CallbackData: "list:" + kind.prefix},
			},
		},
	}

	return r.answerAndEdit(ctx, cq, detailText, markup)
}

func (r *Router) completeListItem(ctx context.Context, kind *listKind, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback, locale string) error {
	occ, err := kind.complete(ctx, parsed.ActorID, parsed.TargetID, r.clock.Now().UTC())
	if err != nil {
		_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.listErrorText(kind, locale, err), true)
		return nil
	}

	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, "general.done"), false)

	// Post group topic update
	emoji, name := r.entryLabel(kind, r.cfg.DefaultLocale, occ)
	groupMsg := r.i18n.T(r.cfg.DefaultLocale, kind.keys.completed, emoji, name, memberName(member))
	_, _ = r.client.SendMessage(ctx, r.cfg.ChatID, groupMsg, &SendOptions{
		ThreadID: kind.topic,
	})

	// If this is in a private chat message, re-render the list view
	return r.refreshList(ctx, kind, member, cq, locale)
}

func (r *Router) postponeListItem(ctx context.Context, kind *listKind, member *household.Member, cq *CallbackQuery, parsed *ParsedCallback, locale string) error {
	if parsed.Payload == "" {
		return r.showPostponePresets(ctx, kind, cq, parsed.TargetID, locale)
	}

	// Preset picked
	if parsed.Payload == "custom" {
		return r.startTypedPostpone(ctx, member, cq, kind.typedStep, parsed.TargetID, locale)
	}

	newDue := r.postponeTime(kind, parsed.Payload)
	occ, err := kind.postpone(ctx, parsed.ActorID, parsed.TargetID, newDue)
	if err != nil {
		return r.answerPostponeError(ctx, kind, member, cq, locale, err)
	}

	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.i18n.T(locale, kind.keys.postponedNotice), false)

	r.postPostponed(ctx, kind, member, occ, newDue)

	// Return to list view
	return r.refreshList(ctx, kind, member, cq, locale)
}

// showPostponePresets edits the message to the postponement presets for one occurrence.
func (r *Router) showPostponePresets(ctx context.Context, kind *listKind, cq *CallbackQuery, occID int64, locale string) error {
	rows := [][]InlineKeyboardButton{
		{
			{Text: r.i18n.T(locale, kind.short.key), CallbackData: fmt.Sprintf("%s:p:%d:%s", kind.prefix, occID, kind.short.value)},
			{Text: r.i18n.T(locale, "buttons.postpone_1h"), CallbackData: fmt.Sprintf("%s:p:%d:1h", kind.prefix, occID)},
			{Text: r.i18n.T(locale, "buttons.postpone_3h"), CallbackData: fmt.Sprintf("%s:p:%d:3h", kind.prefix, occID)},
		},
		{
			{Text: r.i18n.T(locale, "buttons.postpone_tomorrow"), CallbackData: fmt.Sprintf("%s:p:%d:tom", kind.prefix, occID)},
			{Text: r.i18n.T(locale, i18nKeyButtonBack), CallbackData: fmt.Sprintf("%s:d:%d", kind.prefix, occID)},
		},
		{
			{Text: r.i18n.T(locale, "buttons.postpone_custom"), CallbackData: fmt.Sprintf("%s:p:%d:custom", kind.prefix, occID)},
		},
	}
	return r.answerAndEdit(ctx, cq, r.i18n.T(locale, "general.postpone_prompt"), &InlineKeyboardMarkup{InlineKeyboard: rows})
}

// postponeTime returns the due time for a postponement preset of one kind.
func (r *Router) postponeTime(kind *listKind, preset string) time.Time {
	now := r.clock.Now().In(r.clock.Location())
	switch preset {
	case kind.short.value:
		return now.Add(kind.short.delay)
	case "3h":
		return now.Add(3 * time.Hour)
	case "tom":
		return r.dayAt(1, r.defaultTime)
	default:
		return now.Add(1 * time.Hour)
	}
}

// listErrorText is the answer to a failed list action: the ownership wording, or the user error text.
func (r *Router) listErrorText(kind *listKind, locale string, err error) string {
	if errors.Is(err, kind.errNotOwner) {
		return r.i18n.T(locale, kind.keys.notOwner)
	}
	return r.userErrorText(locale, err)
}

// answerPostponeError tells the member why an occurrence could not be postponed.
func (r *Router) answerPostponeError(ctx context.Context, kind *listKind, member *household.Member, cq *CallbackQuery, locale string, err error) error {
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, r.listErrorText(kind, locale, err), true)
	if errors.Is(err, kind.errResolved) {
		// Stale button: tell the user and show the current list instead of the dead detail.
		return r.refreshList(ctx, kind, member, cq, locale)
	}
	return nil
}

// refreshList edits a private list message back to the member's current list of one kind.
func (r *Router) refreshList(ctx context.Context, kind *listKind, member *household.Member, cq *CallbackQuery, locale string) error {
	if cq.Message == nil || cq.Message.Chat.Type != "private" {
		return nil
	}
	occs, remaining, _ := kind.list(ctx, member.ID)
	text, markup := r.formatList(kind, locale, occs, remaining)
	return r.client.EditMessageText(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text, &EditOptions{ReplyMarkup: markup})
}

// postPostponed announces a postponement in the kind's topic, in the default locale.
// It runs after the change is committed; a failed post never rolls anything back.
func (r *Router) postPostponed(ctx context.Context, kind *listKind, member *household.Member, occ listOcc, newDue time.Time) {
	emoji, name := r.entryLabel(kind, r.cfg.DefaultLocale, occ)
	dueStr := r.formatDateTime(r.cfg.DefaultLocale, newDue)
	groupMsg := r.i18n.T(r.cfg.DefaultLocale, kind.keys.postponed, emoji, name, memberName(member), dueStr)
	_, _ = r.client.SendMessage(ctx, r.cfg.ChatID, groupMsg, &SendOptions{ThreadID: kind.topic})
}

// postponePayload is the flow payload while waiting for a typed postponement time.
type postponePayload struct {
	OccID int64 `json:"occ_id"`
}

// startTypedPostpone asks the user to type a new date/time for an occurrence. The target is kept
// in persisted flow state keyed by (user, chat, topic) like every other multi-step interaction.
func (r *Router) startTypedPostpone(ctx context.Context, member *household.Member, cq *CallbackQuery, step string, occID int64, locale string) error {
	_ = r.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	if cq.Message == nil {
		return nil
	}
	key := flow.Key{UserID: member.ID, ChatID: cq.Message.Chat.ID, ThreadID: cq.Message.MessageThreadID}
	if err := r.flow.Set(ctx, key, step, postponePayload{OccID: occID}, r.cfg.FlowTTL); err != nil {
		r.logger.Error("failed to store postpone flow state", "err", err)
		_, sendErr := r.client.SendMessage(ctx, cq.Message.Chat.ID, r.i18n.T(locale, "general.internal_error"), &SendOptions{ThreadID: cq.Message.MessageThreadID})
		return sendErr
	}
	markup := &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{
		{Text: r.i18n.T(locale, i18nKeyButtonCancel), CallbackData: "fl:c"},
	}}}
	_, err := r.client.SendMessage(ctx, cq.Message.Chat.ID, r.i18n.T(locale, "general.postpone_typed_prompt"), &SendOptions{
		ThreadID:    cq.Message.MessageThreadID,
		ReplyMarkup: markup,
	})
	return err
}

// handleTypedPostponeText applies a typed postponement through the same domain commands as the presets.
func (r *Router) handleTypedPostponeText(ctx context.Context, member *household.Member, msg *Message, state *flow.State, locale string) error {
	var payload postponePayload
	if err := json.Unmarshal(state.Payload, &payload); err != nil {
		_ = r.flow.Clear(ctx, state.Key)
		return nil
	}
	reply := func(text string) error {
		_, err := r.client.SendMessage(ctx, msg.Chat.ID, text, &SendOptions{ThreadID: msg.MessageThreadID})
		return err
	}

	newDue, err := parseDateTime(strings.TrimSpace(msg.Text), r.clock)
	if err != nil {
		guidance := r.i18n.T(locale, "general.invalid_date_format")
		if isPastDue(err) {
			guidance = r.i18n.T(locale, "general.past_due_time") + " " + guidance
		}
		return reply(guidance) // keep the flow: the user can type again or cancel
	}

	_ = r.flow.Clear(ctx, state.Key)
	kind := r.remindersKind()
	if strings.HasPrefix(state.Step, "chore_") {
		kind = r.choresKind()
	}
	occ, err := kind.postpone(ctx, member.ID, payload.OccID, newDue)
	if err != nil {
		return reply(r.postponeErrorText(locale, err))
	}
	r.postPostponed(ctx, kind, member, occ, newDue)
	return reply(r.i18n.T(locale, "general.postponed_until", r.formatDateTime(locale, newDue)))
}

// postponeErrorText is userErrorText plus the ownership errors that carry their own wording.
func (r *Router) postponeErrorText(locale string, err error) string {
	switch {
	case errors.Is(err, chores.ErrNotAssignee):
		return r.i18n.T(locale, i18nKeyChoresNotAssignee)
	case errors.Is(err, reminders.ErrNotTarget):
		return r.i18n.T(locale, i18nKeyRemindersNotTarget)
	}
	return r.userErrorText(locale, err)
}
