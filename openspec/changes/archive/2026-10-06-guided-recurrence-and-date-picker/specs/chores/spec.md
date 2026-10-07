# Spec Delta

## MODIFIED Requirements

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
