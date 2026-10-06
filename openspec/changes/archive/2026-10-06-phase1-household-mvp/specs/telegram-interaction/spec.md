# Spec Delta

## Purpose

Defines how Telegram acts as the interaction layer: topic-derived context, inline-keyboard flows, persisted temporary flow state, callback safety and idempotency, with the database as the only source of truth.

## ADDED Requirements

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
Where "Choose date/time" is offered, the flow SHALL accept a typed reply from the same user in the same chat and topic, parsed in the household timezone, and SHALL reject unparsable or past values with guidance.

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
