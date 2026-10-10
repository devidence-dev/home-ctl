# Tasks

## 1. Onboarding on private activity

- [x] 1.1 Add `EnsureConnected` to the household service and store (write only when not connected, keep the first `connected_at`); verify with service and storage tests for a first connection, a repeated call keeping the timestamp, and a non-member being rejected.
- [x] 1.2 Call it from the router for every private message and private callback of an authorized member; verify with router tests that `/list` and a button press in private connect Pablito, `/status` in the group does not connect Bob, and `/start` still replies with help.

## 2. Connect link

- [x] 2.1 Add `GetMe` to `Client`, `HTTPClient` and the fakes, and an optional `URL` field on `InlineKeyboardButton`; verify with client tests for the `getMe` result and the serialized `url` button.
- [x] 2.2 Read the username at startup (best effort) into the router config and add the "🔗 Connect with the bot" URL button to the unconnected notice; verify with tests for the notice with a username (`https://t.me/<username>?start=connect`) and without one (no button), and that `/start connect` connects the member.

## 3. Status layout

- [x] 3.1 Update `status.*` keys in both locales and the status rendering; verify with a test for the mixed-state reply in English and Spanish.

## 4. Documentation and validation

- [x] 4.1 Update `docs/manual-tests.md` (onboarding and `/status` steps); verify the steps match the implementation.
- [x] 4.2 Run `scripts/verify.sh` and require it to pass on the current code.
