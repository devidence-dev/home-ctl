# Proposal

## Why

Manual testing of the scheduling picker showed three rough edges. The flow message is sent at `/add` and then edited in place, so a typed answer (name, Type date, Other) appears below the flow message and the conversation reads out of order. Choosing a time that already passed today is only reported in a transient alert while all 24 hours stay tappable, so the same mistake can be repeated. The steps show no context of what was already chosen, and the confirmation is a flat list of internal labels ("Ancla: Al completar", "cada 2 semana(s)").

## What Changes

- After an accepted typed answer, the flow continues in a new message sent below the user's text; the previous flow message is deleted, or loses its buttons when deletion fails. Button-driven steps keep editing one message.
- When the first due date is today, hours before the current hour and minutes not after the current time are inert. The hour step gets a Change date button back to the calendar. If a past time still arrives (typed minute, or time moving on), the message itself explains it and shows the hour step again.
- Every step after the title shows a short header with what was already chosen (title, person, date).
- The confirmation uses one emoji line per value, a plain-language next-occurrence sentence instead of "anchor", and Confirm/Cancel buttons with icons.
- Recurrence text uses correct singular and plural forms ("cada semana", "cada 2 semanas") everywhere it is shown.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `telegram-interaction`: typed answers re-anchor the flow message; past times are prevented and explained; steps carry a context header; new confirmation layout.
- `localization-config`: recurrence text has singular and plural forms.

## Impact

- `internal/telegram`: flow-message helper, picker renderers, chores and reminders glue, Telegram client gains `deleteMessage`.
- `locales/en.yaml`, `locales/es.yaml`, `docs/manual-tests.md`.
- No domain, storage or recurrence change; no new dependency.

## Non-goals

- Deleting the user's own typed messages.
- Changing the order of picker steps or recurrence semantics.
- Weekday-set recurrence (separate change).
- Restyling Shopping or notification messages beyond the shared Confirm/Cancel labels.
