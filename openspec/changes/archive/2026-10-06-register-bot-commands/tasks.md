# Tasks

## 1. Telegram client

- [x] 1.1 Add `SetMyCommands` and the scope/command types to `Client` and `HTTPClient`; update test fakes; verify with a `client_test.go` case asserting the request body for chat and private scopes and language code, and an API error returned.

## 2. Localization

- [x] 2.1 Add `telegram.commands.{add,list,status,help,start}.description` to `locales/en.yaml` and `locales/es.yaml`, add them to `DefaultRequiredKeys`, and add `Bundle.Locales()`; verify with i18n tests (missing key fails startup, `Locales()` lists en and es) and the existing localization completeness test.

## 3. Registrar

- [x] 3.1 Implement the registrar in `internal/telegram/commands.go` (group scope for `chat_id`, private scope, language codes `""`/`en`/`es`, failures logged with scope and lang, continue on error); verify with tests: group list is add/list/status/help, private list is start/status/help, no `start` in group, no `add`/`list` in private.
- [x] 3.2 Verify with tests that `en`/`es` descriptions equal `en.yaml`/`es.yaml`, the `""` menu uses the default locale, and a fake that fails the `es` group call still receives the remaining five calls and a log entry naming scope and lang.

## 4. Startup wiring

- [x] 4.1 Call the registrar in `cmd/homectl/main.go` after the client is created, with a bounded timeout, ignoring failures; verify by running the app against an unreachable API base in a test or a manual run showing logged errors and polling still starting.

## 5. Authorization and routing

- [x] 5.1 Add router tests: non-member `/add` in the group is rejected with no flow; `/list` in an unmapped Topic returns `general.unmapped_topic`; verify they pass without any registration having run.

## 6. Validation

- [x] 6.1 Run `scripts/verify.sh` and require it to pass on the current code.
