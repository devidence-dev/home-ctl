# Spec Delta

## MODIFIED Requirements

### Requirement: Reminder definition and creation
A reminder definition SHALL have a title, optional emoji, optional description, a target member, and optional recurrence rule and anchor. `/add` in the Reminders topic SHALL collect these using the guided scheduling picker (recurrence type, interval when recurrent, explicit first due date and time, anchor when recurrent) and create the first occurrence only after confirmation, with `scheduled_at` equal to `due_at`.

#### Scenario: One-off reminder
- **WHEN** Alice creates 💊 "Buy medicine" for Bob, picks One time, Friday Oct 9 2026 10:00, and confirms
- **THEN** one open occurrence exists for Bob due Friday Oct 9 2026 10:00 in the household timezone and no interval or anchor was asked

#### Scenario: Recurring reminder
- **WHEN** Alice creates 💊 "Take vitamins" for Bob, Daily, interval 2, first due Oct 10 2026 at 08:00, anchor keep the schedule, and confirms
- **THEN** one open occurrence exists for Bob due Oct 10 2026 08:00 and the next scheduled slot is Oct 12 2026 08:00

#### Scenario: Missing title
- **WHEN** the title is empty at confirmation
- **THEN** nothing is created and the user is asked for a title

#### Scenario: Target not a member
- **WHEN** a callback names a target that is not an active member
- **THEN** the creation is rejected
