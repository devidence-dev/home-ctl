# chores Specification

## Purpose

Lets the household define reusable chores and schedule individual occurrences assigned to specific members, with personal completion, postponement and history.

## Requirements

### Requirement: Reusable chore definitions
The system SHALL keep chore definitions with an emoji, a name and an optional description. A definition SHALL carry the optional recurrence rule and anchor.

#### Scenario: Create definition
- **WHEN** Alice creates chore 🧹 "Sweep the floor" with no description in the Chores topic
- **THEN** the definition is stored and offered in later `/add` flows

#### Scenario: Blank name rejected
- **WHEN** a user submits an empty name for a new chore
- **THEN** nothing is stored and the user is asked again

### Requirement: Chore scheduling flow
After a chore is selected, the system SHALL collect the assignee (from active members), the recurrence type, the interval when recurrent, an explicit first due date and time, and the recurrence anchor when recurrent, using the guided scheduling picker, and SHALL create the first occurrence only after the user confirms. The first occurrence SHALL have `scheduled_at` equal to `due_at`, and the first due date SHALL be the origin of a recurring series. When the selected chore definition already carries a recurrence rule and anchor, the system SHALL NOT ask for recurrence type, interval or anchor again.

#### Scenario: Weekly chore creation
- **WHEN** Alice selects 🧹 Sweep the floor, assigns Alice, picks Weekly, interval 1, first due tomorrow at 18:00, anchor keep the schedule, and confirms
- **THEN** one open occurrence exists for Alice with `scheduled_at` = `due_at` = tomorrow 18:00 in the household timezone

#### Scenario: Every 3 months from completion
- **WHEN** Alice creates 🧹 "Change water purifier filter" for herself, Monthly, interval 3, first due Oct 10 2026 at 10:30, anchor from completion, and confirms
- **THEN** the definition stores "every 3 months" with anchor `completion` and one open occurrence is due Oct 10 2026 10:30

#### Scenario: Definition already recurring
- **WHEN** Alice selects a chore whose definition is "every 2 weeks, scheduled"
- **THEN** the flow skips recurrence type, interval and anchor and asks for assignee, first due date and time

#### Scenario: Abandoned flow creates nothing
- **WHEN** Alice leaves the flow before confirming and the flow expires
- **THEN** no definition change and no occurrence exist

#### Scenario: Due time in the past rejected
- **WHEN** a typed or picked due time is earlier than now
- **THEN** it is rejected and the user is asked to pick a future time

### Requirement: Assignee-only resolution
Only the assignee of a chore occurrence SHALL be allowed to complete or postpone it. This SHALL be enforced by the application logic, independent of which buttons Telegram displays.

#### Scenario: Assignee completes
- **WHEN** Alice, assignee, completes "Sweep the floor"
- **THEN** the occurrence is completed with Alice and the timestamp recorded

#### Scenario: Non-assignee rejected
- **WHEN** Bob replays or forwards Alice's Done button for her occurrence
- **THEN** the system refuses, the occurrence stays open, and Bob gets a "not yours" response

#### Scenario: Non-assignee postpone rejected
- **WHEN** Bob triggers Postpone on Alice's occurrence
- **THEN** `due_at` is unchanged and no postponement record exists

### Requirement: Completion
Completing an occurrence SHALL record `completed_at` and the completing member, stop notifications for it, create the next occurrence if recurrent (see recurrence), and post an update to the Chores topic. History SHALL NOT be deleted or overwritten. Completion before `scheduled_at` or `due_at` SHALL be allowed.

#### Scenario: Normal completion
- **WHEN** Alice completes "Sweep the floor" due Tue 18:00 at Tue 18:20
- **THEN** it is completed with that timestamp, the next occurrence is created, and the topic shows "✅ 🧹 Sweep the floor — completed by Alice"

#### Scenario: Early completion
- **WHEN** Alice completes a chore due Wed 10:00 on the previous Saturday
- **THEN** completion succeeds and the next occurrence follows the definition's anchor rule

#### Scenario: Repeated Done
- **WHEN** Alice presses Done twice
- **THEN** exactly one completion and at most one successor occurrence exist and the second press is a no-op

#### Scenario: Group update fails
- **WHEN** the group message fails to send after the completion commit
- **THEN** the completion remains persisted and the failure is recorded

### Requirement: Postponement
Postponing SHALL apply only to open occurrences and SHALL update only `due_at` of the occurrence (never `scheduled_at` or the recurrence definition), record who postponed and when, reset the notification state for the new `due_at`, and post an update to the Chores topic. Options SHALL include 30 minutes, 1 hour, 3 hours, tomorrow and a typed date/time.

#### Scenario: Postpone by three hours
- **WHEN** Alice postpones a chore due 18:00 and picks 3 hours at 17:30
- **THEN** `due_at` becomes 20:30, `scheduled_at` is unchanged, and a postponement record stores the old and new values

#### Scenario: Postpone to tomorrow
- **WHEN** Alice picks "Tomorrow" on Oct 6
- **THEN** `due_at` is Oct 7 at the household default time of day, evaluated in the household timezone

#### Scenario: Typed time unparsable
- **WHEN** Alice types "banana" as a date/time
- **THEN** the system reports the accepted format and `due_at` is unchanged

#### Scenario: Postponing a completed occurrence
- **WHEN** Alice completed "Sweep the floor" and later presses Postpone on a stale button
- **THEN** the system rejects it with an "already completed" message, `due_at` is unchanged, no postponement record exists and nothing is posted to the group

#### Scenario: Authorization precedes state and time checks
- **WHEN** Bob (not the assignee) postpones Alice's completed occurrence to a past time
- **THEN** the result is "not yours", not "already completed" or a time error

#### Scenario: Repeated postponement to the same time
- **WHEN** Alice's postpone request to Sat 10:00 is delivered twice
- **THEN** one postponement record exists and the occurrence is due Sat 10:00

#### Scenario: Completion and postponement race
- **WHEN** Alice's Done and Postpone arrive at the same moment
- **THEN** exactly one transition wins: either the occurrence is completed with its original `due_at` and the postpone is rejected, or it was postponed once and then completed

### Requirement: Single due notification
A chore SHALL produce one private due notification per `due_at`. It SHALL remain open and be shown overdue until resolved, without repeated notifications.

#### Scenario: Overdue chore not re-notified
- **WHEN** a chore due Mon 18:00 is still open on Tue 18:00
- **THEN** only one notification attempt exists for that `due_at` and the chore is listed as overdue

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
