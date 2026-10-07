# Design

## Context

`telegram.Client` (raw `net/http`) has no `setMyCommands`. The router already enforces membership and Topic routing and already replies `general.unmapped_topic` for `/add` and `/list` in unmapped Topics. Private `/list` goes to `handlePrivateList`, which only lists chores, so it is not advertised. `i18n.Bundle` flattens nested YAML into dotted keys and keeps loaded locales private. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:** best-effort menu registration at startup using existing locale resources; isolated from command handling.

**Non-Goals:** Topic scopes, per-user scopes, dynamic menus, any change to routing or authorization.

## Decisions

- **`Client.SetMyCommands(ctx, scope, languageCode, commands)`**: one more method on the existing interface, same `call` helper. Scope is a small struct serialized as `{"type":"chat","chat_id":N}` or `{"type":"all_private_chats"}`. Alternative, a separate client interface: rejected, one Bot API client already exists. Test fakes get the method.
- **Registrar in `internal/telegram/commands.go`**: `RegisterCommands(ctx, client, translator, chatID, locales, logger)`. It loops scopes x language codes (`""`, plus each locale), builds the list from locale keys, calls `SetMyCommands`, logs failures with `scope` and `lang`, and continues. It returns nothing; startup ignores the outcome. It holds no household logic. Alternative, a struct with state: rejected, nothing to hold.
- **Locale keys** `telegram.commands.<name>.description` for add, list, status, help, start, added to `DefaultRequiredKeys` so a missing key fails startup like other keys. Alternative, hardcoded descriptions: rejected (all text is localized).
- **Language codes**: `""` uses the default locale; `en`, `es` use their own files. `Bundle` gets a small `Locales()` accessor so the list follows the loaded files; fallback menu is always registered. Alternative, hardcode `en`/`es`: rejected, drifts when a locale is added.
- **Command lists** are plain constants in the registrar (group: add, list, status, help; private: start, status, help). The names are protocol, not text.
- **Call site**: `main.go`, right after `NewHTTPClient`, with the startup context and a bounded timeout, run before the polling loop. Six sequential calls are cheap; no goroutine needed.
- **Unmapped `/list`** already works; the change only adds a test and a spec scenario.

Principles: SQLite stays authoritative (nothing persisted); registration is idempotent (`setMyCommands` replaces the menu); the Telegram call stays behind the `Client` interface and fails gracefully (logged, never silent success claimed); no business logic or authorization is added; no new dependency; the token is never logged.

## Risks / Trade-offs

- Group menu shows `/add` and `/list` in every Topic, including General -> the router replies with the unmapped-topic notice.
- Menu language follows the Telegram client, not the app locale -> accepted for Phase 1.
- Telegram may take time to refresh clients' menus -> none; retry happens on next start.
- Bot not admin or removed from the group makes the group registration fail -> logged, startup continues.
