# Tasks

## 1. Command menu

- [x] 1.1 Add `list` to the private menu (`start`, `list`, `status`, `help`) and update `telegram.commands.list.description` in both locales; verify by updating the registrar tests so the private list is exactly start, list, status, help and still has no `add`.

## 2. List entries

- [x] 2.1 Add the shared entry renderer and locale keys (entry lines, overdue label, view buttons) to both locales and required keys; verify with table tests in `es` and `en` for an upcoming recurring item ("📅 hoy a las 23:05 · 🔁 Cada 2 semanas"), a one-time item without recurrence text, and an overdue item ("⚠️ Venció: ayer a las 18:00").
- [x] 2.2 Rewrite `formatChoresList` and `formatRemindersList` to put numbered entries in the body and numbered buttons with the existing detail callbacks; verify with list tests for the chores scenario (two items, order by `due_at`) and the reminders scenario, and that the "+N more" line still appears past 20 items.

## 3. View switch

- [x] 3.1 Add the chores/reminders switch row to every private list view, including empty ones, using `list:ch` and `list:re`; verify with tests that pressing the reminders button on the chores view edits the same message into the reminders view, that the empty reminders view still shows the switch, and that Back from a detail still returns to its list.
- [x] 3.2 Keep private `/list` opening the chores view and topic `/list` delivering its domain view privately; verify with router tests for private `/list` and for `/list` in the Reminders topic.

## 4. Documentation and validation

- [x] 4.1 Update `docs/manual-tests.md` (command menu and list sections) and the `help.text` locale string if it no longer matches; verify the listed steps match the implementation.
- [x] 4.2 Run `scripts/verify.sh` and require it to pass on the current code.
