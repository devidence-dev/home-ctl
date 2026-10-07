# Proposal

## Why

After a flow button is used (item picked, Cancel, Confirm, a step answered), its inline keyboard stays on the chat message and stays clickable. Pressing it later only yields an "expired" alert, so the chat shows actions that no longer work. Observed in the Shopping `/add` menu; Chores and Reminders flows share the same callback path and the same flaw.

## What Changes

- When a flow button press is accepted (the flow advances, completes or is cancelled), the keyboard on the message that carried the button is removed; the message text stays as history.
- When a flow button is pressed after the flow expired or was already closed, the keyboard of that message is also removed, in addition to the existing "expired" alert.
- A rejected press (for example an item already pending) leaves the keyboard in place so the user can choose again.
- Persistent list views are unchanged: the shopping list, personal lists and notification buttons keep their keyboards because they are meant to be pressed repeatedly.
- Applies to every inline-keyboard flow (Shopping, Chores, Reminders) because they share one callback path. No copy changes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `telegram-interaction`: adds a requirement that flow keyboards stop being actionable once used or expired.

## Impact

- `internal/telegram/flow.go`: one place closes the keyboard around flow callback handling.
- Tests in `internal/telegram` and the e2e test. No Telegram client interface change, no config, schema or dependency change.

## Non-goals

- Deleting messages.
- Closing the Cancel button of a typed-input prompt (name, emoji, date) when the user types the answer; that needs the prompt message id stored in flow state.
- Closing the keyboard of an older `/add` menu when the same user starts a new `/add` before pressing anything; it is closed on its first press.
- Changing list, notification or postpone keyboards.
