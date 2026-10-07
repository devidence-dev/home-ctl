# Design

## Context

Today the Chores and Reminders flows (`chores.go`, `reminders.go`) send a new message per step, ask for the due date first (presets plus a typed fallback), then recurrence with a fixed interval of 1 and an anchor. `flow_state` is keyed by (user, chat, thread) and stores step, JSON payload and expiry; it does not know which message the flow is shown in. `handleFlowCallback` looks up the flow of the pressing user, so another member's press finds no flow and, since the keyboard-closing change, is treated as expired and closes the owner's keyboard. Callback data is parsed in `callback.go` within the 64-byte limit. Recurrence rules already support "every N day/week/month" with origin-based month clamping, so no domain change is needed. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:** one shared picker for Chores and Reminders; single-message editing that survives restarts; flow ownership by actor; localized, compact, testable UI.

**Non-Goals:** new recurrence types or weekday sets, changes to recurrence or anchor semantics, changes to Shopping steps or the postpone picker, natural-language dates. See proposal.md.

## Decisions

- **Shared picker in `internal/telegram/schedpicker.go`.** Pure functions render each step (recurrence type, interval, calendar, hours, minutes, anchor, summary) into text plus an inline keyboard from locale strings, and pure functions validate typed answers. The Chores and Reminders glue maps picker output to their existing services; the services stay separate. Alternative, a widget per domain: rejected, it duplicates the calendar and time logic.
- **Compact callbacks, state in the payload.** New kinds under `fl:`: `fl:rt:<daily|weekly|monthly|once>`, `fl:iv:<n>` and `fl:iv:o` (Other), `fl:cal:<yyyy-mm>` navigation, `fl:d:<yyyy-mm-dd>` day, `fl:d:today`, `fl:d:tomorrow`, `fl:d:type`, `fl:h:<hh>`, `fl:mi:<mm>` and `fl:mi:o`, `fl:noop`. The accumulated choices (type, interval, date, hour) live in the flow payload, never in callback data. Longest value is well under 64 bytes.
- **Persist the active message id.** Migration `0002` adds nullable `message_id INTEGER` to `flow_state`. The flow store gains `SetMessageID` and `GetByMessage(chat, thread, message)`, returning the owner and state. Migration `0001` is not edited. Rows without a message id behave as before (key-based lookup).
- **One flow-message helper.** `showFlowMessage(state, text, markup)` edits the stored message; on any edit error other than "message is not modified" it sends a replacement, stores the new id and continues; edit failures are logged at debug and never change flow or domain state. Shopping prompts keep sending new messages but go through the same helper in send mode so their id is recorded for ownership.
- **Ownership check first in `handleFlowCallback`.** Look up the flow by the pressed message. Found and owner equals the actor: proceed. Found and owner differs: answer with the localized notice and return without any edit, state change or keyboard close. Not found: the existing expired path (alert, close keyboard). This also repairs the keyboard-closing change, whose no-state branch closed other users' menus. The actor always comes from the update, never from callback data.
- **Keyboard rule in a one-message flow.** Closing becomes: terminal transitions (confirm, cancel, expired) remove the keyboard; step transitions replace it through the helper; in-step navigation (calendar month) edits only the grid. The existing before/after step comparison stays for Shopping and is replaced by explicit per-handler outcomes for the picker.
- **Time and dates.** Date plus hour plus minute are combined in the household location from the existing clock and `APP_TIMEZONE`; the result must be later than now or the flow returns to the hour step with the date kept. Calendar weekday order is Monday first, using the locale's weekday names reordered; month names reuse `datetime.months`. Past days and the previous-month button on the current month use the inert `fl:noop` callback, answered with an empty toast.
- **Limits.** Interval 1 to 99 (typed), minutes by 5 plus typed 00-59, calendar months unbounded forward. These are assumptions recorded here.
- **Existing recurring definitions** keep skipping recurrence type, interval and anchor, as today.
- **Typed input.** Type date, Other (interval) and Other (minute) set a typed step; the existing `handleFlowText` dispatch handles it, validates with the existing parser and rules, then edits the active flow message.

Principles: SQLite stays authoritative (flow state and the message id are persisted; nothing is read back from Telegram message text); idempotent (a repeated press repeats a pure transition and persists nothing until Confirm; confirm still creates once); Telegram calls stay behind `Client` and fail gracefully; recurrence and authorization stay in the domain and services (the picker only builds a rule and a due time and the services validate them); no new dependency; the owner is the Telegram actor.

## Risks / Trade-offs

- Flows in progress during deploy lose message-id tracking -> they last at most the 30 minute TTL; their next press follows the key-based path or expires.
- Telegram may reject an edit of an old message -> replacement message sent, flow continues.
- 24 hour buttons plus navigation stay far below the 100-button limit; 7-column calendar rows stay below the 8-button row limit.
- A stale message of an earlier flow can be pressed by its owner while a newer flow exists in the same topic -> lookup by message id finds no flow for that message and it takes the expired path, so it cannot drive the newer flow.
- Several steps means more taps than presets -> Today and Tomorrow shortcuts and typed alternatives stay.

## Migration Plan

Deploy the binary; the migration runs at startup and is additive. Rollback to the previous binary is safe because the new column is nullable and ignored by older code.
