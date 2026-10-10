package telegram

import (
	"errors"

	"github.com/px1/home-ctl/internal/chores"
	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/reminders"
	"github.com/px1/home-ctl/internal/shopping"
)

// isPastDue reports whether err means a requested due time lies in the past.
func isPastDue(err error) bool {
	return errors.Is(err, chores.ErrPastDueTime) || errors.Is(err, reminders.ErrPastDueTime)
}

// userErrorText maps err to a localized, safe message. Unexpected errors are logged
// for developers and replaced by a generic message so internals never reach users.
func (r *Router) userErrorText(locale string, err error) string {
	switch {
	case isPastDue(err):
		return r.i18n.T(locale, "general.past_due_time")
	case errors.Is(err, household.ErrNotMember), errors.Is(err, chores.ErrNotMember),
		errors.Is(err, reminders.ErrNotMember), errors.Is(err, shopping.ErrNotMember):
		return r.i18n.T(locale, "general.not_member")
	case errors.Is(err, chores.ErrBlankName), errors.Is(err, reminders.ErrBlankTitle),
		errors.Is(err, shopping.ErrBlankName):
		return r.i18n.T(locale, "general.blank_name")
	case errors.Is(err, chores.ErrOccurrenceResolved), errors.Is(err, reminders.ErrOccurrenceResolved):
		return r.i18n.T(locale, "general.already_resolved")
	case errors.Is(err, chores.ErrOccurrenceNotFound), errors.Is(err, chores.ErrDefinitionNotFound),
		errors.Is(err, reminders.ErrOccurrenceNotFound), errors.Is(err, reminders.ErrDefinitionNotFound),
		errors.Is(err, shopping.ErrEntryNotFound):
		return r.i18n.T(locale, "general.not_found")
	}
	r.logger.Error("unexpected error handling telegram action", "err", err)
	return r.i18n.T(locale, "general.internal_error")
}

// recurrenceSummary renders "every N unit" (plus the anchor when given) from locale resources.
func (r *Router) recurrenceSummary(locale string, interval int, unit string, anchor *string) string {
	text := RecurrenceText(r.i18n, locale, interval, unit)
	if anchor != nil {
		text += " (" + r.i18n.T(locale, "buttons.anchor_"+*anchor) + ")"
	}
	return text
}
