# Spec Delta

## MODIFIED Requirements

### Requirement: Private personal list
`/list` in the Chores topic SHALL deliver to the requester's private chat the requester's open occurrences (overdue and upcoming) sorted by `due_at`. The message body SHALL list each occurrence on its own numbered entry with emoji, name, due date and time rendered in the household timezone and the member's locale, the recurrence summary when recurring, and an overdue marker when `due_at` is in the past. There SHALL be one numbered button per occurrence. `/list` sent in the private chat SHALL show the same chores view. Every private list view SHALL offer buttons to switch between the chores and the reminders view, editing the same message. Selecting one SHALL edit the same message into a detail view with Done, Postpone and Back. Back SHALL edit the message to the refreshed list. The group topic SHALL NOT show personal action buttons.

#### Scenario: Alice lists her chores
- **WHEN** Alice runs `/list` in Chores with 🚿 Clean bathroom overdue, 💧 Change water filter due Sat, 🗑 Take out trash due Mon
- **THEN** her private chat shows three buttons in that order, and the group shows none of them

#### Scenario: Entries show date and time
- **WHEN** it is Thu Oct 8 2026 21:00 and Pablito (locale `es`) has 🧹 "Limpiar bebedero de los niños" due today 23:05 every 2 weeks and 🚿 "Lavar el baño" due yesterday 18:00
- **THEN** the body shows "1. 🚿 Lavar el baño" with "⚠️ Venció: ayer a las 18:00" and "2. 🧹 Limpiar bebedero de los niños" with "📅 hoy a las 23:05 · 🔁 Cada 2 semanas", and the buttons read "1. 🚿 Lavar el baño" and "2. 🧹 Limpiar bebedero de los niños"

#### Scenario: Private /list opens the overview
- **WHEN** Alice sends `/list` in her private chat
- **THEN** she sees her chores view with buttons to switch to reminders

#### Scenario: Switch views
- **WHEN** Alice presses the reminders button on her chores view
- **THEN** the same message shows her reminders view with a button back to chores

#### Scenario: Detail view
- **WHEN** Alice taps "💧 Change water filter"
- **THEN** the same message becomes the detail with due date, recurrence summary, assignee and buttons Done, Postpone, Back

#### Scenario: No other member's chores
- **WHEN** Bob has a chore and Alice runs `/list`
- **THEN** Bob's chore does not appear

#### Scenario: Stale detail
- **WHEN** Alice taps Done in a detail view for an occurrence already completed from a notification
- **THEN** no second completion is made and the message re-renders the current list

#### Scenario: Long list
- **WHEN** a member has 35 open occurrences
- **THEN** the list shows the first 20 by `due_at` and a "+15 more" line
