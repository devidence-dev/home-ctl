# notifications Specification

## Purpose

Defines notification attempts as records distinct from occurrences, their delivery tracking, restart safety, and the group status updates posted after state changes.

## Requirements

### Requirement: Attempts are separate from occurrences
Each private notification SHALL be recorded as an attempt linked to an occurrence, with target member, send time, outcome and Telegram message id when sent. Attempts SHALL NOT create or alter occurrences.

#### Scenario: Three reminder repeats
- **WHEN** a reminder occurrence is notified three times
- **THEN** one occurrence and three attempt records exist

### Requirement: Due scanning and idempotent delivery
The system SHALL notify when `due_at` is reached for open occurrences. The same due moment SHALL NOT be notified twice, including across restarts and retries.

#### Scenario: Missed while down
- **WHEN** the application is down from 17:50 to 18:10 and a chore was due 18:00
- **THEN** on startup exactly one notification is sent for it

#### Scenario: Restart mid-run
- **WHEN** the application restarts right after sending a notification
- **THEN** no duplicate attempt is created for the same due moment

### Requirement: Delivery failure recording
A failed private send SHALL be recorded with its reason. It SHALL NOT mark the occurrence resolved and SHALL NOT corrupt domain state. Failed deliveries SHALL be retried at the next scan.

#### Scenario: Telegram outage
- **WHEN** Telegram returns an error sending Alice's due notification
- **THEN** the attempt is recorded failed, the occurrence stays open, and the next scan retries

#### Scenario: User blocked the bot
- **WHEN** Telegram reports the bot was blocked by Bob
- **THEN** Bob's onboarding is marked disconnected and `/status` shows him as not connected

### Requirement: Actionable notification
A due notification SHALL include Done and Postpone buttons that remain actionable until the occurrence is resolved, and use the same commands as the private list.

#### Scenario: Press Done on notification
- **WHEN** Alice presses Done on a chore notification
- **THEN** the occurrence is completed once and the notification message no longer offers actions

### Requirement: Group status updates after commit
After completion, postponement or purchase, the system SHALL post a status update to the matching topic. The update SHALL be sent only after the state change is committed. A failed update SHALL be recorded and SHALL NOT roll back the change.

#### Scenario: Postpone update
- **WHEN** Alice postpones 🚿 Clean bathroom until Saturday
- **THEN** the Chores topic shows "⏰ 🚿 Clean bathroom — Alice postponed it until Saturday"

#### Scenario: Update fails
- **WHEN** the topic post fails after a purchase commits
- **THEN** the entry stays purchased and the failure is logged and recorded

### Requirement: Topic routing from configuration
Status updates SHALL be posted to the topic ids configured for shopping, chores and reminders. If a topic id is not configured for a domain, startup SHALL fail.

#### Scenario: Missing topic mapping
- **WHEN** `telegram.topics.reminders` is absent
- **THEN** startup fails naming the missing key
