# Tasks

## 1. Day label

- [x] 1.1 Add `i18n.FormatDay` and its `datetime.day_*` keys in both locales; verify with table tests for yesterday, today, tomorrow, a later day this year and a day next year, in English and Spanish.

## 2. Grouped list

- [x] 2.1 Rewrite `renderPrivateList`/`renderListEntry` to group into Overdue, Today, Tomorrow and Upcoming with the arrow detail line and no numbering; replace the old `lists.*` keys; verify with tests for the Pablito scenario ("🔴 Vencidas" / "╰┈➤ ayer · 18:00", "🟡 Hoy" / "╰┈➤ 23:05 · 🔁 Cada 2 semanas", no empty headers), the upcoming "╰┈➤ sáb 17 oct · 10:00 · 🔁 Cada 3 meses" entry, the English reminders "🟢 Tomorrow" / "╰┈➤ 10:00" entry, an item earlier today listed under Overdue, and buttons in body order without numbers.
- [x] 2.2 Verify with the existing list tests that the view switch, detail navigation, Back and the "+N more" line still work.

## 3. Documentation and validation

- [x] 3.1 Update `docs/manual-tests.md` list section; verify the steps match the implementation.
- [x] 3.2 Run `scripts/verify.sh` and require it to pass on the current code.
