# Tasks

## 1. Flow persistence and ownership lookup

- [x] 1.1 Add migration `0002` with nullable `flow_state.message_id`, plus `SetMessageID` and `GetByMessage` on the SQLite flow store and the `flow` store interface and manager; verify with storage tests that the id round-trips, rows without an id still load, lookup by (chat, thread, message) returns the owner, expired rows are not returned, and migrations apply on a database created by `0001`.

## 2. Picker rendering and callbacks

- [x] 2.1 Add the new callback kinds and parsing in `callback.go`; verify with table tests that each valid form parses, malformed and oversized data are rejected, and every generated callback is at most 64 bytes.
- [x] 2.2 Implement pure renderers for recurrence type, interval grid (1-12 plus Other), month calendar, hour grid and 5-minute grid with Other, anchor and confirmation summary; verify with tests that October 2026 puts Oct 1 under Thursday with 31 day buttons in seven columns, past days and the previous button on the current month use `fl:noop`, hours are 6 columns by 4 rows, minutes 4 by 3, and Spanish output uses Spanish month and weekday names.
- [x] 2.3 Implement pure validators for typed interval (1-99) and minute (00-59) answers; verify with table tests including "0", "100", "abc", "07" and "75".

## 3. Single-message editing

- [x] 3.1 Add the flow-message helper that edits the stored message and falls back to sending and storing a replacement on edit errors except "message is not modified"; verify with tests using the fake client that a normal step edits the same id, a failing edit sends a replacement and updates the stored id, and a not-modified response is not treated as failure.

## 4. Ownership

- [x] 4.1 Reorder `handleFlowCallback` to look up the flow by the pressed message and enforce ownership, with the localized notice and no side effects for non-owners; verify with tests that Bob pressing Cancel on Alice's Chores flow and an item on Alice's Shopping menu changes no state, no edit and no keyboard, that Alice sees the same flows intact afterwards, and that Alice pressing after expiry still gets the expired alert and a closed keyboard.
- [x] 4.2 Route Shopping flow prompts through the helper in send mode so their message ids are recorded; verify with the existing Shopping tests plus one asserting the id is stored after `/add`.

## 5. Chores flow

- [x] 5.1 Wire the picker into the Chores flow in the order recurrence type, interval, first date, hour, minute, anchor, confirmation, skipping interval and anchor for one time and all three recurrence steps for definitions that already recur; verify with flow tests for one time, weekly every 2 weeks from Oct 14 2026 (slots Oct 28 and Nov 11), monthly from Jan 31 2027 (Feb 28, Mar 31), "every 3 months from completion", and a recurring definition.
- [x] 5.2 Verify with tests that a past time on today keeps the flow at the hour step with the date kept, that abandoning creates nothing, that Confirm creates exactly one occurrence with `scheduled_at` equal to `due_at`, and that the same message id is edited through the whole flow.

## 6. Reminders flow

- [x] 6.1 Wire the same picker into the Reminders flow keeping target selection; verify with tests for a one-off reminder for Bob (no interval or anchor asked) and a daily every-2-days reminder whose next slot is two days after the first, with one edited message throughout.

## 7. Typed alternatives

- [x] 7.1 Implement Type date, Other for interval and Other for minute as typed steps that continue by editing the flow message; verify with tests for valid answers, invalid answers staying on the step with guidance, and another user's text being ignored.

## 8. Localization

- [x] 8.1 Add all picker strings to `locales/en.yaml` and `locales/es.yaml` and to the required keys; verify with the existing localization completeness test, a test that startup fails when a picker key is missing, and an assertion that no picker text is hardcoded in Go (rendering tests in both locales).

## 9. Documentation and validation

- [x] 9.1 Update `docs/manual-tests.md` sections 5 and 6 for the new flow, ownership and single-message behavior; verify each listed step matches the implemented sequence.
- [x] 9.2 Run `scripts/verify.sh` and require it to pass on the current code.
