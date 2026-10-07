# Design

## Context

Every flow button reaches `handleFlowCallback` (`internal/telegram/flow.go`), which loads the flow state and dispatches to the Shopping, Chores or Reminders handler. Handlers either advance the flow (`flow.Set` to a new step), finish it (`flow.Clear`), or reject the press with an alert and leave the step unchanged. `EditMessageText` already supports replacing a message's markup, and the shopping list refresh already uses an empty keyboard. Non-flow keyboards use other callback kinds and do not pass through this path. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:** stop used or expired flow keyboards from being actionable, in one place, without changing flow logic.

**Non-Goals:** see proposal.md; also no new Telegram client method and no change to flow storage.

## Decisions

- **Close in `handleFlowCallback`, not per handler.** Read the step before dispatch; after the handler returns without error, close the keyboard when the flow no longer awaits that step (cleared or moved on). If the press found no state (expired or already closed), close it too. A rejected press leaves the step unchanged, so the keyboard stays. Alternative, an explicit close call in every handler: rejected, about two dozen sites that could each be forgotten.
- **Reuse `EditMessageText`** with the callback message's own text (`cq.Message.Text`) and an empty `InlineKeyboard`. Alternative, a new `editMessageReplyMarkup` client method: rejected, one more interface method and fake for no gain since the text is plain. Alternative, `deleteMessage`: rejected, it removes the conversation record and fails on old messages.
- **Best effort.** An edit failure (message too old, already edited, "message is not modified") is logged at debug level and never changes the flow outcome or the user's reply.
- **Message text must be present.** If `cq.Message.Text` is empty the edit is skipped.

Principles: SQLite stays authoritative (nothing new is stored; Telegram message state carries no business logic); idempotent (a repeated press hits the expired path, writes nothing, and closing twice is harmless); the Telegram call stays behind the `Client` interface and fails gracefully; authorization is untouched because the close runs after the existing membership and ownership checks.

## Risks / Trade-offs

- A press that legitimately keeps the same step but is accepted would leave its keyboard open -> none exist today (all flows are single-choice per step); tests cover each flow so a future one is noticed.
- Telegram answers the edit with an error for messages older than its limit or unchanged -> logged and ignored; the stale button then only yields the expired alert, as today.
- Typed-input prompts keep their Cancel button after the user types -> accepted non-goal; first press closes it.
