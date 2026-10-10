# Design

## Context

`internal/telegram/list.go` builds the private chores and reminders lists (`formatChoresList`, `formatRemindersList`): a title line and one `emoji name` button per occurrence (`ch:d:<id>`, `re:d:<id>`); empty lists return an empty keyboard. Private `/list` (`handlePrivateList`) sends only the chores list. `list:ch` and `list:re` callbacks already re-render a list by editing the message (used by Back). Services return at most 20 occurrences ordered by `due_at` plus a remaining count. `RegisterCommands` (`internal/telegram/commands.go`) advertises `start`, `status`, `help` privately. `i18n.FormatDateTime` renders localized dates with today and tomorrow. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:** advertise `list` privately; one overview with a chores/reminders switch; entries that show when each item is due.

**Non-Goals:** see proposal.md; no change to detail, Done or Postpone, nor to the 20-item cap.

## Decisions

- **Switch through the existing callbacks.** Every private list view (empty or not) gets a last row `[🧹 Tareas] [⏰ Recordatorios]` mapped to `list:ch` and `list:re`; the current view's button is marked (for example "• 🧹 Tareas") and still re-renders. The existing `list:*` handlers already edit the message, so no new callback kind. Alternative, a new `list:all` combined view: rejected, two short views read better on a phone and reuse Back.
- **Body entries rendered by one helper per domain** sharing `renderListEntry(n, emoji, name, dueAt, recurrence, now)`: line 1 `n. emoji name`, line 2 either `📅 <date> · 🔁 <recurrence>` or `⚠️ <overdue label>: <date>`. Dates use `FormatDateTime` in the household location and member locale; recurrence uses the existing pluralized summary.
- **Numbered buttons** `n. emoji name` keep the detail callbacks; the number links each button to its body entry when Telegram truncates long names.
- **Private `/list`** keeps calling the chores view, now with the switch row. Topic `/list` in Chores and Reminders keeps delivering its own view privately, also with the switch row.
- **Menu**: `privateCommandNames` becomes `start, list, status, help`. The private `list` uses its own key `telegram.commands.private_list.description` ("Ver tus tareas y recordatorios" / "Show your chores and reminders"), because the group `list` description also covers shopping and stays unchanged.
- **Locale keys** for the entry lines, a gender-neutral overdue label ("⚠️ Venció: …", shared by chores and reminders), view buttons, `datetime.yesterday` and updated list titles in `en.yaml` and `es.yaml`, added to required keys.

Principles: SQLite stays authoritative (lists are read-only renders of service results; nothing is stored); idempotent (switching and re-rendering have no side effects); Telegram calls stay behind `Client`; authorization unchanged (services return only the requester's occurrences); no new dependency.

## Risks / Trade-offs

- 20 entries with two lines each stay well under Telegram's 4096-character message limit (about 20 × 120 characters).
- The menu language still follows the Telegram client, not the member locale -> unchanged, accepted earlier.
- A list message shows due times as of when it was rendered -> switching views or Back re-renders with the current time.
