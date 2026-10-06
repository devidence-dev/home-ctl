# reminders Specification

## Purpose

Lets members create reminders targeted at a household member, with repeat notifications until acknowledged, personal completion, postponement and history.

## Requirements

### Requirement: Reminder definition and creation
A reminder definition SHALL have a title, optional emoji, optional description, a target member, and optional recurrence rule and anchor. `/add` in the Reminders topic SHALL collect these and create the first occurrence only after confirmation, with `scheduled_at` equal to `due_at`.

#### Scenario: One-off reminder
- **WHEN** Alice creates 💊 "Buy medicine" for Bob, Friday 10:00, no recurrence, and confirms
- **THEN** one open occurrence exists for Bob due Friday 10:00 in the household timezone

#### Scenario: Missing title
- **WHEN** the title is empty at confirmation
- **THEN** nothing is created and the user is asked for a title

#### Scenario: Target not a member
- **WHEN** a callback names a target that is not an active member
- **THEN** the creation is rejected

### Requirement: Target-only resolution
Only the target member of a reminder occurrence SHALL be allowed to complete or postpone it, enforced by the application logic.

#### Scenario: Target completes
- **WHEN** Bob completes "Buy medicine"
- **THEN** it is completed by Bob with a timestamp

#### Scenario: Other member rejected
- **WHEN** Alice presses Bob's Done button
- **THEN** the system refuses and the occurrence stays open

### Requirement: Repeat notifications until resolved
While an occurrence is open, past `due_at`, and not postponed, the system SHALL send the target a private notification at `due_at` and again every configured `unresolved_reminder_interval`. Repeats SHALL stop on completion or postponement.

#### Scenario: Hourly repeats
- **WHEN** interval is 1h and Bob ignores a reminder due 10:00
- **THEN** notifications are sent at 10:00, 11:00 and 12:00 until he resolves it

#### Scenario: Postpone stops repeats
- **WHEN** Bob postpones at 11:10 by 3 hours
- **THEN** no further notification is sent before 14:10, and one is sent at 14:10

#### Scenario: Restart does not duplicate
- **WHEN** the application restarts at 11:00:30 after sending the 11:00 notification
- **THEN** no duplicate 11:00 notification is sent and the next is at 12:00

### Requirement: Completion
Completing SHALL record `completed_at` and the resolving member, stop notifications, create the next occurrence if recurrent, and post an update to the Reminders topic. Completion before `due_at` SHALL be allowed. Repeated Done SHALL be idempotent.

#### Scenario: Completed with update
- **WHEN** Bob completes "Buy medicine"
- **THEN** the Reminders topic shows "✅ 💊 Buy medicine — completed by Bob"

#### Scenario: Double Done
- **WHEN** Bob presses Done twice
- **THEN** exactly one completion and at most one successor exist

### Requirement: Postponement
Postponing SHALL apply only to open occurrences, change only `due_at`, record actor and old/new values, and post an update to the Reminders topic. Options SHALL include 15 minutes, 1 hour, 3 hours, tomorrow and a typed date/time.

#### Scenario: Postpone to tomorrow
- **WHEN** Bob picks Tomorrow on Oct 6
- **THEN** `due_at` is Oct 7 at the household default time of day and the topic shows "Bob postponed it until tomorrow at 09:00"

#### Scenario: Past typed time
- **WHEN** Bob types a time in the past
- **THEN** it is rejected and `due_at` is unchanged

#### Scenario: Postponing a completed reminder
- **WHEN** Bob completed "Buy medicine" and later presses Postpone on a stale button
- **THEN** the system rejects it with an "already completed" message, `due_at` is unchanged and nothing is posted to the group

#### Scenario: Postponement restarts the notification cycle
- **WHEN** Bob postpones a reminder notified at 10:00 by 15 minutes with a 1h repeat interval
- **THEN** he is notified again at the new due time 10:20, not at 11:00

#### Scenario: Completion and postponement race
- **WHEN** Bob's Done and Postpone arrive at the same moment
- **THEN** exactly one transition wins and the occurrence is never moved after completion

### Requirement: Private personal list
`/list` in the Reminders topic SHALL deliver the requester's own open occurrences privately with the same one-button, detail, Done, Postpone and Back behavior as chores, sorted by `due_at`.

#### Scenario: Bob lists reminders
- **WHEN** Bob runs `/list` in Reminders
- **THEN** his private chat shows only reminders targeting Bob, and nothing personal appears in the group
