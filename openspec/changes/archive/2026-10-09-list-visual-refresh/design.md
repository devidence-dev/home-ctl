# Design

## Context

`renderPrivateList` and `renderListEntry` in `internal/telegram/list.go` render numbered entries ("1. emoji name" then "📅 date · 🔁 recurrence" or "⚠️ Venció: date") and numbered buttons, from items already sorted by `due_at` (at most 20). `i18n.FormatDateTime` renders "today at 18:00", "tomorrow at …", "yesterday at …" or "sat 17 Oct, 10:00"; there is no day-only label. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:** a grouped, scannable list with the approved traffic-light layout.

**Non-Goals:** see proposal.md.

## Decisions

- **Group in one pass.** Items stay sorted by `due_at`; each is assigned a bucket by comparing its household-local day with today: overdue when `due_at` is before now (even earlier today), today, tomorrow, upcoming. Buckets are emitted in fixed order and skipped when empty. Because the input is sorted, the button order equals the body order.
- **Day label without time.** Add `i18n.FormatDay(locale, t, now, loc)` returning "ayer"/"yesterday", "hoy"/"today", "mañana"/"tomorrow" or the localized "sáb 17 oct" (with the year when it differs), reusing the weekday and month resources. Overdue and Upcoming lines use `FormatDay + " · " + HH:MM`; Today and Tomorrow use only `HH:MM`.
- **Locale keys**: `lists.group_overdue` ("🔴 Vencidas"), `lists.group_today` ("🟡 Hoy"), `lists.group_tomorrow` ("🟢 Mañana"), `lists.group_upcoming` ("⚪ Próximas"), `lists.entry_detail` ("╰┈➤ %s"), `lists.entry_recurrence` ("%s · 🔁 %s"), and `datetime.day_*` keys for the day label. The old `lists.due`, `lists.due_recurring` and `lists.overdue` keys are removed from locales and required keys.
- **No numbering**, no leading spaces: the arrow carries the indentation, so clients that trim spaces render the same.

Principles: read-only rendering of service results (SQLite stays authoritative, nothing stored); no side effects; no new dependency.

## Risks / Trade-offs

- An item due earlier today but already past shows under Overdue with "hoy · 09:00" -> correct and clearer than a past time under Today.
- Twenty entries plus four headers stay far below the 4096-character message limit.
