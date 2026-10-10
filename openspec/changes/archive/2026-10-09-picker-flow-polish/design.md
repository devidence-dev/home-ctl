# Design

## Context

`showFlowMessage` (`internal/telegram/flow.go`) edits the stored flow message or, in send mode, sends a new one and stores its id; Shopping already uses send mode. Typed steps (`chore_new_name`, `reminder_title`, `*_typed_date`, `*_typed_interval`, `*_typed_minute`) currently edit the stored message, which sits above the user's text. `RenderHourStep` and `RenderMinuteStep` render fixed grids; the past-time check lives in the chores and reminders minute handlers and only answers with an alert on the button path. `RenderConfirmationStep` prints "label: value" lines; recurrence text comes from `general.recurrence_every_{day,week,month}` with "(s)" placeholders, used by the confirmation and by `recurrenceSummary` for lists. The `Client` interface has no delete method. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:** keep the active flow message last after typed input; prevent and explain past times; give each step context; a readable confirmation; correct plurals.

**Non-Goals:** see proposal.md; no change to step order, flow storage or ownership rules.

## Decisions

- **Re-anchor on typed input in the helper.** Add a `reanchor` mode to `showFlowMessage`: send the step as a new message, store its id, then delete the previous message id; if delete fails, edit it to its text with an empty keyboard. Every typed-step handler (accepted or rejected answer) uses this mode; button steps keep edit mode. Shopping typed prompts use it too, which also removes their stale Cancel buttons. Alternative, deleting the user's message instead: rejected, users expect their text to stay and bots cannot always delete user messages.
- **`Client.DeleteMessage(ctx, chatID, messageID)`** via `deleteMessage`; fakes updated. Failures are logged at debug and never change flow state.
- **Ownership stays message-based.** The new message id replaces the stored one in the same `SetMessageID` call, so callbacks from the deleted or closed message take the expired path as today.
- **Time-aware renderers.** `RenderHourStep` and `RenderMinuteStep` take the chosen date, the chosen hour and `now` in the household location. For today: hours `< now.Hour()` and, in the current hour, minutes `<= now.Minute()` render as "·" with `fl:noop`. The server-side check stays for typed minutes and races; on failure the hour step is shown with a localized "⚠️ time has passed" line in the message text. Alternative, only the alert: rejected, it disappears and leaves no trace.
- **Change date button** on the hour step uses `fl:d:back`; the handler sets the step to the date step and re-renders the calendar on the chosen month, keeping recurrence and interval.
- **Context header** is one pure function `RenderContextHeader(payload)` prepended to every step after the title; values come from the flow payload, dates via the existing localized formatter.
- **Confirmation layout** from locale templates: `picker.summary_line_*` keys hold emoji and label ("👤 %s", "👤 Para: %s", "📅 %s", "🔁 %s", "⏭️ ..."), so emojis and wording stay in locale files. Default title emoji stays per domain (🧹 chores, ⏰ reminders).
- **Confirm and Cancel labels** become "✅ Confirmar" and "❌ Cancelar" in `buttons.confirm` and `buttons.cancel`, shared by all keyboards for consistency.
- **Plurals** with `_one` and `_other` keys (`general.recurrence_every_week_one: "Cada semana"`, `general.recurrence_every_week_other: "Cada %d semanas"`), selected in one helper used by the confirmation and `recurrenceSummary`. The old keys are removed from the required list and locale files. Alternative, a pluralization library: rejected, two forms cover en and es.

Principles: SQLite stays authoritative (only the stored message id changes, in the existing column); idempotent (re-anchoring happens once per accepted text; a repeated callback on a closed message takes the expired path); Telegram calls stay behind `Client` and their failures never alter flow or domain state; validation of past times stays in the flow and services, the inert buttons are presentation only; no new dependency.

## Risks / Trade-offs

- Bots can delete their own group messages only for 48 hours -> flows expire after 30 minutes, and the fallback removes buttons.
- A minute can pass between rendering and pressing -> the server-side check still rejects it and explains it in the message.
- Changing shared Confirm/Cancel labels touches every keyboard -> tests assert by callback data, not labels; text assertions are updated.
