# Tasks

## 1. Telegram client

- [x] 1.1 Add `DeleteMessage` to `Client` and `HTTPClient` and to the test fakes; verify with a `client_test.go` case for the request body and an API error.

## 2. Re-anchoring after typed input

- [x] 2.1 Add the re-anchor mode to `showFlowMessage` (send new, store id, delete previous, fall back to removing its buttons); verify with tests that the new id is stored, the old message is deleted, and a failing delete leaves the old message edited with an empty keyboard.
- [x] 2.2 Use re-anchor mode in every typed step of Chores, Reminders and Shopping, for accepted and rejected answers; verify with flow tests that after typing the name the assignee step is a new message after the user's text, later button steps edit that new message, and a callback on the old message takes the expired path.

## 3. Past times

- [x] 3.1 Make `RenderHourStep` and `RenderMinuteStep` time-aware and add Change date (`fl:d:back`); verify with renderer tests at 14:20 on Oct 5 2026 (hours 00-13 and minutes 00-20 inert for today, nothing inert for tomorrow) and a callback test that Change date returns to the calendar keeping recurrence and interval.
- [x] 3.2 Show the localized "time has passed" line in the flow message when the server-side check rejects a time; verify with a test for a typed minute in the past (Chores and Reminders) that the message shows the warning and the hour step with today kept, and that nothing is created.

## 4. Context and confirmation

- [x] 4.1 Add `RenderContextHeader` and prepend it to every step after the title; verify with a test that the hour step for "Clean kitchen", Alice, Oct 14 2026 starts with that header in English and Spanish.
- [x] 4.2 Rewrite `RenderConfirmationStep` with emoji lines from locale keys, the next-occurrence sentence only for recurring items, and "✅/❌" button labels; verify with tests for the Spanish "Limpiar bebedero de los niños" summary and the English one-time reminder summary (no next-occurrence line).

## 5. Plurals

- [x] 5.1 Replace `general.recurrence_every_*` with `_one`/`_other` keys in both locales and the required keys, and route the confirmation and `recurrenceSummary` through one helper; verify with table tests for 1 and N in day, week and month in both locales, and with the localization completeness test.

## 6. Documentation and validation

- [x] 6.1 Update `docs/manual-tests.md` sections 5 and 6 with the re-anchoring, inert past times, Change date, context header and new confirmation; verify each step matches the implemented behavior.
- [x] 6.2 Run `scripts/verify.sh` and require it to pass on the current code.
