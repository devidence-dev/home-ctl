# Design

## Context

In `internal/telegram/chores.go` the `chore_new_name` text step sets `payload.Emoji = "🧹"` and moves to the assignee step; `reminders.go` sets `⏰` after `reminder_title`. Both typed steps already re-anchor the flow message. Selecting an existing chore copies the definition emoji into the payload. Picker renderers live in `schedpicker.go`; callbacks are compact `fl:` kinds parsed in `callback.go`. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:** let members choose an emoji for new chores and reminders with minimal taps.

**Non-Goals:** see proposal.md.

## Decisions

- **New step between name/title and assignee/target**: `chore_emoji` and `reminder_emoji`, rendered by a shared `RenderEmojiStep(suggestions)`: two rows of four suggestion buttons, then Other, Skip and Cancel. It is shown re-anchored because it follows a typed answer; later button steps edit it.
- **Suggestions from locale files** (`picker.emoji_suggestions_chores`: "🧹,🧽,🚰,🗑️,🧺,🍽️,🚿,🌱"; `picker.emoji_suggestions_reminders`: "⏰,💊,📞,💡,💳,🎂,🏥,🛒") so they can change without code. Callback `fl:em:<index>` carries only the index; the handler resolves it against the same locale list. `fl:em:o` opens the typed step, `fl:em:skip` keeps the default. Alternative, the emoji in callback data: rejected, multi-codepoint emojis are long and callbacks should carry identifiers.
- **Typed validation** `ValidateTypedEmoji`: trim; non-empty; at most 16 runes; no `unicode.IsLetter` or `unicode.IsDigit` runes. Plain text is rejected; flags, skin tones and ZWJ sequences pass.
- **Defaults stay** 🧹 and ⏰ for Skip and for the existing fallback paths.

Principles: SQLite stays authoritative (the emoji is stored on the definition at Confirm, as today); idempotent (the step only updates the flow payload); Telegram stays behind `Client`; ownership and authorization unchanged; no new dependency.

## Risks / Trade-offs

- One more step in the flow -> one tap with a suggestion or Skip.
- A locale list with fewer than eight entries -> the renderer shows what exists; the index is validated against the list length.
