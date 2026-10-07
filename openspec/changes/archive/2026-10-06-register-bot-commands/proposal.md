# Proposal

## Why

Users must know and type bot commands by hand because the bot never registers its command menu with Telegram, so typing `/` suggests nothing. Registration should be owned by the application, not by a manual BotFather step.

## What Changes

- At startup, after the Telegram client exists, the application registers command menus through `setMyCommands`, best effort and idempotent (safe on every start).
- Group scope: the configured household chat only, advertising `add`, `list`, `status`, `help`.
- Private scope: all private chats, advertising `start`, `status`, `help`. `add` is not advertised (it needs Topic context). `list` is not advertised: the private `/list` today only shows chores, not a chosen or inferred domain.
- Menus are registered for language codes `""` (fallback), `en` and `es`. Descriptions come from the existing locale files under new keys; command names stay stable.
- A failure for one scope/language is logged with that context and the rest continue. Registration never blocks startup and never produces a Telegram message.
- `/list` in an unmapped Topic replies with the existing localized "not available here" notice (same as `/add`); it never guesses a domain.
- The menu is discoverability only; every command keeps its existing membership, onboarding, Topic and ownership checks.

## Capabilities

### New Capabilities
- `command-menu`: startup registration of localized, scoped Telegram command menus, failure isolation, and the rule that the menu grants no authorization.

### Modified Capabilities

None. Existing requirements are unchanged.

## Impact

- `internal/telegram`: Client interface gains `SetMyCommands`; new registrar; fakes updated.
- `internal/i18n`: required keys and a way to list loaded locales.
- `locales/en.yaml`, `locales/es.yaml`: command descriptions.
- `cmd/homectl`: one startup call. No new dependencies, no schema or config changes.

## Non-goals

- New domain commands.
- Topic-specific menus (Telegram has no Topic scope).
- Per-user menus or forcing the menu language to the app locale; Telegram picks by client language.
- Menus that change with flow state.
- BotFather `/setcommands` as a deployment requirement.
