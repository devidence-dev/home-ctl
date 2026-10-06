# Spec Delta

## Purpose

Defines how recurring chores and reminders produce their next occurrence: supported rules, the two anchor modes, the no-backlog policy and timezone-correct calendar arithmetic.

## ADDED Requirements

### Requirement: Supported recurrence rules
A recurrence rule SHALL be an interval `every N` with a unit of day, week or month, where N is a positive integer. Daily, every N days, weekly, every N weeks and monthly SHALL all be expressible. Recurrence SHALL be defined in the domain, independent of Telegram.

#### Scenario: Valid rules
- **WHEN** rules "every 1 day", "every 3 days", "every 1 week", "every 2 weeks", "every 1 month" are created
- **THEN** all are accepted

#### Scenario: Invalid interval
- **WHEN** a rule with N = 0 or a negative N is submitted
- **THEN** it is rejected and no definition changes

### Requirement: Anchor selected on the definition
Each recurring definition SHALL have an anchor, `scheduled` or `completion`, chosen when recurrence is configured and editable later. The system SHALL NOT ask for the anchor on each completion. Editing the anchor or rule SHALL affect only occurrences generated afterward.

#### Scenario: Edit anchor later
- **WHEN** a definition changes from `scheduled` to `completion` while one occurrence is open
- **THEN** the open occurrence is unchanged and the next one is computed with the new anchor

### Requirement: Occurrence time fields
Every occurrence SHALL carry `scheduled_at` (nominal slot, never changed by postponement), `due_at` (currently actionable time, changed by postponement) and `completed_at` (set once on completion).

#### Scenario: Postponement leaves the slot intact
- **WHEN** an occurrence scheduled Mon 18:00 is postponed to Tue 09:00
- **THEN** `scheduled_at` stays Mon 18:00 and `due_at` becomes Tue 09:00

### Requirement: Scheduled anchor
For the `scheduled` anchor, the next occurrence SHALL be the first slot of the original schedule that is later than both the completed occurrence's `scheduled_at` and its `completed_at`. Early completion SHALL NOT shift the schedule.

#### Scenario: Early completion keeps the schedule
- **WHEN** "Take out trash" repeats every Monday (scheduled Mon Jan 5) and is completed Saturday Jan 3
- **THEN** the next occurrence is Monday Jan 12

#### Scenario: Late completion skips missed slots
- **WHEN** weekly Monday, scheduled Jan 5, is completed Jan 28
- **THEN** the next occurrence is Monday Feb 2 and no occurrences exist for Jan 12, 19 or 26

### Requirement: Completion anchor
For the `completion` anchor, the next `scheduled_at` SHALL be `completed_at` plus the interval, using calendar arithmetic in the household timezone, at the definition's time of day.

#### Scenario: Early completion shifts the schedule
- **WHEN** "Change water purifier filter" repeats every 3 months by completion, was scheduled Wednesday Mar 18, and is completed Saturday Mar 14
- **THEN** the next occurrence is Sunday Jun 14 at the definition's time of day

#### Scenario: Late completion
- **WHEN** the same chore, scheduled Mar 18, is completed Apr 2
- **THEN** the next occurrence is Jul 2

### Requirement: No backlog
A recurring definition SHALL have at most one unresolved occurrence at any time. The system SHALL NOT create multiple overdue occurrences for missed slots. The next occurrence SHALL be created only when the current one is completed.

#### Scenario: Long absence
- **WHEN** a weekly chore is ignored for six weeks
- **THEN** exactly one open (overdue) occurrence exists

#### Scenario: Retry creates one successor
- **WHEN** completion is retried after a crash between commit and response
- **THEN** exactly one successor exists, enforced even under concurrent attempts

### Requirement: Calendar arithmetic and end-of-month clamping
Month intervals SHALL use calendar months, not fixed-day durations. When the target month lacks the origin's day, the system SHALL clamp to the month's last day, and later slots SHALL return to the origin day when the month allows it.

#### Scenario: Months are not 90 days
- **WHEN** an "every 3 months" completion-anchored rule is completed Jan 31 (non-leap year)
- **THEN** the next date is Apr 30, not May 1

#### Scenario: No drift after clamping
- **WHEN** a monthly `scheduled` rule has origin Jan 31
- **THEN** its slots are Feb 28, Mar 31, Apr 30, May 31

### Requirement: Household timezone semantics
All calendar semantics (today, tomorrow, weekdays, month boundaries, due-date calculation, postponement options, recurrence and formatting) SHALL be evaluated in the household timezone from `APP_TIMEZONE`. Absolute timestamps SHALL be stored in UTC. The time of day of a recurring slot SHALL remain the definition's local time across daylight-saving changes.

#### Scenario: Tomorrow near midnight
- **WHEN** at 23:30 America/Guayaquil on Oct 6 (04:30 UTC Oct 7) a user picks "Tomorrow"
- **THEN** the resolved date is Oct 7 local, not Oct 8

#### Scenario: Weekly across DST
- **WHEN** `APP_TIMEZONE=America/New_York` and a weekly Monday 18:00 slot crosses the March DST change
- **THEN** the next slot is still Monday 18:00 local time
