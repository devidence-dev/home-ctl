# Spec Delta

## Purpose

Defines who may use the household bot, how members are identified and how private-chat onboarding state is tracked and exposed, so private notifications never fail silently.

## ADDED Requirements

### Requirement: Explicitly configured members
The system SHALL accept interactions only from users listed in configuration, identified by Telegram user id. Telegram usernames SHALL be used for display and initial association only and SHALL NOT be used as identity.

#### Scenario: Configured member is authorized
- **WHEN** user id 123456789 (Alice) sends `/add` in the Chores topic
- **THEN** the system starts the chore flow for Alice

#### Scenario: Unknown user is rejected
- **WHEN** user id 555000111, not configured, sends `/add` in the Shopping topic
- **THEN** the system performs no domain action and creates no state for that user

#### Scenario: Username change does not break identity
- **WHEN** Alice (id 123456789) renames her Telegram username from `alice` to `alice_new` and sends `/list`
- **THEN** the system still recognizes her as Alice

### Requirement: Members synchronized into the database
The system SHALL persist configured members and keep them in sync with configuration at startup, matching by Telegram user id. Members removed from configuration SHALL be deactivated, not deleted, so history keeps its references.

#### Scenario: Member removed from config
- **WHEN** Bob is removed from `users.yaml` and the application restarts
- **THEN** Bob is marked inactive, cannot interact, and his past completions remain attributed to him

#### Scenario: Duplicate user id in config
- **WHEN** `users.yaml` lists id 123456789 twice
- **THEN** startup fails with an error naming the duplicate id

### Requirement: Private-chat onboarding tracking
The system SHALL record, per member, whether private-chat onboarding is complete, set when that member sends `/start` to the bot in a private chat.

#### Scenario: Member completes onboarding
- **WHEN** Bob (id 987654321) sends `/start` in a private chat with the bot
- **THEN** Bob is recorded as connected with the timestamp

#### Scenario: Non-member sends /start
- **WHEN** an unconfigured user sends `/start` privately
- **THEN** the system replies that access is not configured and records no onboarding

### Requirement: Onboarding status view
The system SHALL provide `/status` in the group that lists every member with their connection state.

#### Scenario: Mixed connection states
- **WHEN** `/status` runs and Alice and Bob are connected but Charlie is not
- **THEN** the reply lists "Alice — connected", "Bob — connected" and "Charlie — open the bot and press Start"

### Requirement: Unconnected members are surfaced, not ignored
When the system must send a private message to a member who is not connected, it SHALL NOT drop it silently. It SHALL record the failed delivery and, where a user is waiting on the result, tell them in the originating topic how to connect.

#### Scenario: /list by unconnected member
- **WHEN** Charlie, not connected, sends `/list` in the Chores topic
- **THEN** the topic receives a short notice telling Charlie to open the bot and press Start, and no list is posted in the group

#### Scenario: Due chore for unconnected assignee
- **WHEN** a chore assigned to Charlie becomes due and Charlie is not connected
- **THEN** a failed notification attempt is recorded with reason "not connected" and the occurrence stays open
