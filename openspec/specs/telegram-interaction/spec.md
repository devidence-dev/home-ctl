# telegram-interaction Specification

## Purpose

Defines how Telegram acts as the interaction layer: topic-derived context, inline-keyboard flows, persisted temporary flow state, callback safety and idempotency, with the database as the only source of truth.

## Requirements

### Requirement: Topic determines domain context
The system SHALL derive the domain from the Telegram topic (`message_thread_id`) mapped through configuration. The same command SHALL behave per domain.

#### Scenario: /add per topic
- **WHEN** `/add` is sent in topic 42 (shopping), 51 (chores) and 63 (reminders)
- **THEN** the shopping, chore and reminder flows start respectively

#### Scenario: Unmapped topic
- **WHEN** `/add` is sent in a topic id not in configuration
- **THEN** no flow starts and the user gets a short notice

#### Scenario: Wrong chat
- **WHEN** a command arrives from a group whose id differs from `telegram.chat_id`
- **THEN** it is ignored

### Requirement: Inline-keyboard flows with confirmation
Multi-step flows SHALL use inline keyboards and create or modify permanent data only at a confirmation step. Cancel SHALL be available at every step.

#### Scenario: Confirm creates data
- **WHEN** a user reaches the final Confirm in the chore flow and presses it
- **THEN** permanent records are written once

#### Scenario: Cancel leaves no data
- **WHEN** a user presses Cancel at the assignee step
- **THEN** nothing permanent is written and the flow state is removed

### Requirement: Persisted flow state with TTL
Temporary conversation state SHALL be stored in the database, separate from domain data, keyed by user, chat and topic (topic empty in private chats), and SHALL expire after a configured TTL. State SHALL survive restarts.

#### Scenario: Concurrent flows
- **WHEN** Alice and Bob both run `/add` in Chores at the same time and answer differently
- **THEN** each flow keeps its own selections and neither affects the other

#### Scenario: Survives restart
- **WHEN** the application restarts while Alice is at the due-date step
- **THEN** Alice can continue the flow

#### Scenario: Expired flow
- **WHEN** Alice presses a button on a flow past its TTL
- **THEN** she is told it expired and nothing is written

#### Scenario: Different topic, same user
- **WHEN** Alice has an open flow in Chores and starts `/add` in Shopping
- **THEN** the two flows are independent

### Requirement: Typed date/time input
Where "Choose date/time", Type date, or Other is offered, the flow SHALL accept a typed reply from the same user in the same chat and topic, parsed in the household timezone, and SHALL reject unparsable or past values with guidance.

#### Scenario: Valid typed value
- **WHEN** Alice types "2026-10-10 10:00" at the due-date step
- **THEN** the due time is Oct 10 2026 10:00 household time

#### Scenario: Another user's message ignored
- **WHEN** Bob posts text in the Chores topic while Alice's flow awaits typed input
- **THEN** Alice's flow is unchanged

### Requirement: Callbacks carry identity-free references
Callback data SHALL identify only the target and action, within Telegram's size limit. The acting user SHALL always be taken from the Telegram update, never from callback data.

#### Scenario: Forwarded button
- **WHEN** Bob presses a button message forwarded from Alice's private notification
- **THEN** the actor is evaluated as Bob and authorization applies

#### Scenario: Oversized or malformed data
- **WHEN** a callback with malformed data arrives
- **THEN** it is ignored and logged without changing state

### Requirement: Idempotent callbacks and persistent state authority
Callbacks SHALL be idempotent where practical. Telegram message history SHALL NOT be used as application state. A failed Telegram send after a committed change SHALL leave the committed state authoritative.

#### Scenario: Repeated callback
- **WHEN** the same Done callback is delivered twice
- **THEN** one completion exists

#### Scenario: Telegram reconnect redelivery
- **WHEN** Telegram redelivers an already-processed update id
- **THEN** no domain action is repeated

### Requirement: Telegram carries no business logic
The Telegram adapter SHALL translate updates into application commands and render results. Authorization, recurrence and state transitions SHALL be decided outside the adapter.

#### Scenario: Equivalent paths
- **WHEN** Done is pressed from a due notification or from the private list detail
- **THEN** both reach the same completion command with the same outcome

### Requirement: Personal list navigation
Private lists SHALL be navigated by editing a single message: list, detail and back. Navigation SHALL NOT create additional messages per step.

#### Scenario: One message
- **WHEN** Alice opens a detail and presses Back
- **THEN** the same message id is edited each time

### Requirement: Flow keyboards are closed once used
When a flow button press completes or cancels the flow, the system SHALL remove the inline keyboard from the message that carried it and keep the message text. When a press moves a one-message flow to its next step, the system SHALL replace the keyboard with the next step's keyboard, and a press that only navigates within a step, such as changing the displayed calendar month, SHALL keep the step and keyboard. When a flow button is pressed by its owner after its flow expired or was closed, the system SHALL also remove that message's keyboard and tell the user the action expired. A rejected press SHALL leave the keyboard in place. Persistent list and notification keyboards SHALL NOT be affected.

#### Scenario: Item picked in the Shopping menu
- **WHEN** Alice taps "Leche" on the Shopping `/add` menu
- **THEN** the menu message keeps its text "Selecciona artículos para agregar a la lista de compras:" with no buttons, and Alice gets the confirmation message

#### Scenario: Cancel closes the menu
- **WHEN** Alice taps Cancel on the Chores assignee step
- **THEN** that message loses its buttons and the flow state is removed

#### Scenario: Confirm closes the summary
- **WHEN** Bob taps Confirm on the reminder summary
- **THEN** the summary message loses its buttons and the reminder is created once

#### Scenario: Press after expiry
- **WHEN** Alice taps "Leche" on a menu whose flow expired 30 minutes ago
- **THEN** she sees the expired alert, nothing is written, and the menu message loses its buttons

#### Scenario: Rejected press keeps the menu
- **WHEN** Alice taps "Pan" and "Pan" is already on the shopping list
- **THEN** she sees the already-pending alert and the menu keeps its buttons so she can pick another item

#### Scenario: Step change replaces the keyboard
- **WHEN** Alice picks hour 10 in the Chores flow
- **THEN** the same message now shows the minute buttons

#### Scenario: Calendar navigation keeps the step
- **WHEN** Alice presses next month on the calendar
- **THEN** the message still shows a calendar and the flow is still at the date step

#### Scenario: List keyboards stay
- **WHEN** Alice taps "Leche" on the shopping list in the Shopping topic
- **THEN** the list message is refreshed with the remaining pending items still tappable

### Requirement: Guided scheduling picker
The Chores and Reminders creation flows SHALL use one shared scheduling picker that collects, in this order: recurrence type (one time, daily, weekly, monthly), interval (only for recurring items, "every N" of the type's unit), first due date, hour, minute, recurrence anchor (only for recurring items), and a final confirmation. The first due date SHALL always be chosen explicitly, including for recurring items, and SHALL be the scheduled origin of the series. Interval choices SHALL be 1 to 12 plus Other for a typed integer from 1 to 99. The anchor choices SHALL be labeled by their meaning (keep the schedule, from completion). The confirmation SHALL show title, assignee or target, date and time, recurrence and anchor before anything is persisted.

#### Scenario: Every 2 weeks from a Wednesday
- **WHEN** Alice picks Weekly, interval 2, first due Wed Oct 14 2026 at 10:30, anchor keep the schedule, and confirms
- **THEN** the first occurrence is Oct 14 2026 10:30 and the scheduled slots that follow are Oct 28 and Nov 11 at 10:30

#### Scenario: Every month from the 31st
- **WHEN** Bob picks Monthly, interval 1, first due Jan 31 2027 at 09:00 and anchor keep the schedule
- **THEN** the scheduled slots are Jan 31, Feb 28, Mar 31 and Apr 30 at 09:00

#### Scenario: One-time item skips recurrence steps
- **WHEN** Alice picks One time
- **THEN** the picker asks no interval and no anchor and goes from recurrence type to the first due date

#### Scenario: Interval typed with Other
- **WHEN** Alice presses Other at the interval step and types "15"
- **THEN** the interval is 15 and the flow continues at the date step

#### Scenario: Invalid typed interval
- **WHEN** Alice types "0" or "100" or "abc" at the interval step
- **THEN** she is told to send a whole number from 1 to 99 and the flow stays at the interval step

#### Scenario: Confirmation summary
- **WHEN** the flow reaches confirmation for "Change water purifier filter", assignee Alice, Oct 10 2026 10:30, every 3 months, anchor from completion
- **THEN** one message shows those five values and offers Confirm and Cancel, and nothing is stored yet

### Requirement: Calendar date selection
The first due date step SHALL show an inline month calendar with seven day columns, localized month and weekday labels, previous and next month navigation, and the buttons Today, Tomorrow and Type date. Days before today SHALL be inert: pressing one SHALL NOT change the flow. Month navigation SHALL change only the displayed month and SHALL NOT advance the flow step. Previous-month navigation SHALL be inert while the current month is displayed. Type date SHALL accept a typed date or date and time using the existing deterministic parser.

#### Scenario: Month layout
- **WHEN** the calendar shows October 2026 in the household timezone
- **THEN** Oct 1 appears under Thursday, the month has 31 day buttons in seven columns, and the title reads "October 2026" for an English user and "octubre 2026" for a Spanish user

#### Scenario: Past day is inert
- **WHEN** today is Oct 5 2026 and Alice presses Oct 3
- **THEN** the flow, the displayed month and the message stay unchanged

#### Scenario: Navigate to next month
- **WHEN** Alice presses the next-month button while October 2026 is displayed
- **THEN** the same message shows November 2026 and the flow is still at the date step

#### Scenario: Today and Tomorrow
- **WHEN** today is Oct 5 2026 and Alice presses Tomorrow
- **THEN** the first due date is Oct 6 2026 and the flow moves to the hour step

#### Scenario: Typed date
- **WHEN** Alice presses Type date and types "2026-10-10"
- **THEN** the first due date is Oct 10 2026 and the flow moves to the hour step

#### Scenario: Unparsable typed date
- **WHEN** Alice types "next friday" at Type date
- **THEN** she receives the accepted-format guidance and the flow stays at the date step

### Requirement: Time selection
After the first due date the picker SHALL ask for the hour with buttons 00 to 23 and then for the minute with buttons 00, 05, 10 and so on up to 55, plus Other for a typed minute from 00 to 59. All values SHALL be interpreted in the household timezone. If the chosen date and time are not later than now, the system SHALL NOT create an occurrence, SHALL tell the user the time has passed, and SHALL keep the flow active at the hour step.

#### Scenario: Pick 10:30
- **WHEN** Alice has picked Oct 10 2026, presses hour 10 and then minute 30
- **THEN** the due time is Oct 10 2026 10:30 in the household timezone

#### Scenario: Minute typed with Other
- **WHEN** Alice presses Other at the minute step and types "07"
- **THEN** the minute is 07

#### Scenario: Time earlier today
- **WHEN** it is 14:00 on Oct 5 2026 and Alice picks Today, hour 09, minute 00
- **THEN** no occurrence is created, she is told the time has passed, and the flow is at the hour step with her earlier date kept

#### Scenario: Invalid typed minute
- **WHEN** Alice types "75" at the minute step
- **THEN** she is told to send a minute from 00 to 59 and the flow stays at the minute step

### Requirement: Single-message flow editing
All button-driven steps of a creation flow SHALL edit one bot message instead of sending a new message per step. The id of the active flow message SHALL be stored with the persisted flow state so the flow continues across restarts. When a typed answer is accepted, the flow SHALL continue by editing the active flow message. When the active flow message cannot be edited, the system SHALL send a replacement flow message, store its id and continue; the flow state and domain data SHALL NOT be affected by the failure. An edit that changes nothing SHALL NOT count as a failure.

#### Scenario: One message through the flow
- **WHEN** Alice runs `/add` in Chores and goes from recurrence type to confirmation using buttons
- **THEN** the same message id is edited at every step and the topic gains no intermediate bot messages

#### Scenario: Continue after restart
- **WHEN** the application restarts while Alice is at the minute step and she then presses a minute button
- **THEN** the flow continues by editing the same message

#### Scenario: Flow message deleted
- **WHEN** Alice's active flow message was deleted and she types an accepted answer
- **THEN** a replacement flow message at the next step is sent, its id is stored, and the flow continues

#### Scenario: Typed answer
- **WHEN** Alice presses Type date, sends "2026-10-10" and the date is accepted
- **THEN** the flow message is edited to the hour step

### Requirement: Flow ownership
A creation flow message SHALL be owned by the member who started it. A press on its buttons by any other user SHALL NOT advance, cancel, edit or close the flow, SHALL NOT change the flow state, and SHALL answer with a short localized notice that the interaction belongs to another user. Ownership SHALL be derived from the Telegram actor and the stored flow, never from callback data.

#### Scenario: Another member presses a button
- **WHEN** Bob presses Cancel on the message of the Chores flow Alice started
- **THEN** Alice's flow and keyboard are unchanged and Bob sees the notice that the interaction belongs to another user

#### Scenario: Another member presses an item in the Shopping menu
- **WHEN** Bob presses "Leche" on the Shopping `/add` menu that Alice opened
- **THEN** nothing is added, the menu keeps its buttons and Bob sees the notice

#### Scenario: Owner presses after expiry
- **WHEN** Alice presses a button of her flow after it expired
- **THEN** she sees the expired alert, nothing is written and the message loses its buttons
