# Proposal

## Why

A member is "connected" only after sending `/start` privately. Telegram sends `/start` automatically just once, the first time a user opens a bot, so a member who opened the bot before being configured, or who later uses `/list` or `/help` privately, can stay unconnected without knowing it. Their due notifications are then skipped ("notification skipped: user not connected"), as happened with a chore due at 23:05. The `/status` reply and the connect notice are also plain text with no way to connect in one tap.

## What Changes

- Any message or button press from a configured member in a private chat with the bot records the member as connected, not only `/start`. Commands in the group do not, because they do not prove the bot can write to the member privately.
- When an unconnected member uses a command in a topic, the notice telling them to connect carries a "🔗 Connect with the bot" button: a deep link that opens the private chat and sends `/start` in one tap.
- `/status` gets a clearer layout: a header with an emoji, then one line per member with ✅ connected or ⏳ pending and how to connect.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `household-access`: onboarding is recorded on any private interaction; the unconnected notice offers a connect link; `/status` layout.

## Impact

- `internal/telegram` (router private messages and callbacks, connect notice, status), `internal/telegram/client.go` (`getMe` for the bot username used by the link), `cmd/homectl` (read the bot username at startup).
- `locales/en.yaml`, `locales/es.yaml`, `docs/manual-tests.md`.
- No schema change; onboarding already has its table.

## Non-goals

- Re-sending notifications that were skipped while the member was not connected.
- Marking members connected from group activity.
- Detecting a member who blocks the bot later beyond the existing disconnect handling.
