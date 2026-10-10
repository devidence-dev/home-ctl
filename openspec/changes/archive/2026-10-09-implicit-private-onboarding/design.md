# Design

## Context

`Router.handleMessage` authorizes the sender and only `/start` calls `household.HandlePrivateStart`, which upserts `user_onboarding` with `connected = 1`. Callbacks go through `handleCallbackQuery` without touching onboarding. `household.IsConnected` gates private delivery; the scheduler records `not connected` attempts. The connect notice (`general.unconnected_notice`) is plain text in `list.go`. `handleStatus` prints "name — state" lines. The Telegram client has no `getMe`. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:** connect members on any private interaction; one-tap connect from the group; readable `/status`.

**Non-Goals:** see proposal.md.

## Decisions

- **Record on private activity in the router.** After authorization, for a private message or a callback whose message chat is private, call a household method `EnsureConnected(ctx, userID, now)` that reads the state and writes only when not connected, keeping the first `connected_at`. `/start` keeps its reply. Alternative, writing on every message: rejected, needless writes on each update.
- **Group activity never connects.** The bot cannot write privately to a user who never opened the chat, so only private chats count.
- **Deep link button.** At startup call `getMe` (new `Client.GetMe`, best effort, same failure isolation as command registration) and keep the username in the router config. The notice gets an inline URL button `https://t.me/<username>?start=connect`; Telegram opens the private chat and sends `/start connect`, which the existing `/start` prefix match handles. Without a username the notice is sent as today. Alternative, a bot username in config: rejected, one more value to keep in sync with BotFather.
- **URL buttons in the client types.** `InlineKeyboardButton` gains an optional `URL` field serialized as `url`, mutually exclusive with callback data.
- **Status layout from locale keys** `status.header` ("🏠 Miembros del hogar"), `status.connected` ("✅ %s — conectado"), `status.not_connected` ("⏳ %s — pendiente: abre el bot y toca Iniciar").

Principles: SQLite stays authoritative (onboarding is the existing table; writes are idempotent and keep the first timestamp); Telegram calls stay behind `Client` and fail gracefully (`getMe` failure only removes the button); authorization is unchanged (only configured members are recorded, after `Authorize`); no new dependency.

## Risks / Trade-offs

- A member who blocked the bot and later writes to it privately is connected again -> correct, a private message proves the chat is open again.
- `getMe` failing at startup leaves notices without the button until the next restart -> logged; the text still explains how to connect.
