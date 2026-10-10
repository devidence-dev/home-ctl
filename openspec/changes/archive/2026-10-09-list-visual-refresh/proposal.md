# Proposal

## Why

The private lists now show due dates, but every entry repeats "📅 hoy a las …" and a number, which reads cluttered and makes it hard to see at a glance what is late and what is next.

## What Changes

- Private chores and reminders lists group entries by due day in the household timezone, with traffic-light headers: 🔴 Overdue, 🟡 Today, 🟢 Tomorrow, ⚪ Upcoming. Empty groups are not shown.
- Each entry is two lines: `emoji name`, then `╰┈➤ ` followed by the time (Today and Tomorrow) or the day and time (Overdue and Upcoming), plus `· 🔁 <recurrence>` when recurring.
- Entries and buttons drop the numbers; buttons keep the same order as the body.
- The existing cap of 20 entries and the "+N more" line stay.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `chores`: private list layout.
- `reminders`: private list layout.

## Impact

- `internal/telegram/list.go` (grouping and entry rendering), `internal/i18n` (a localized day label without time).
- `locales/en.yaml`, `locales/es.yaml`, `docs/manual-tests.md`.
- No domain or storage change.

## Non-goals

- Rich text formatting (bold or italics) in lists.
- Changing the detail view, the view switch or the 20-entry cap.
- Grouping in the Shopping list.
