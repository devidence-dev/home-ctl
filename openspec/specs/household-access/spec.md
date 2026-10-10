# household-access Specification

## Purpose

Defines who may use the household bot, how members are identified and how private-chat onboarding state is tracked and exposed, so private notifications never fail silently.

## Requirements

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
The system SHALL record, per member, whether private-chat onboarding is complete, set when that member sends any message, including `/start` and other commands, or presses any button in a private chat with the bot. Activity in the group SHALL NOT set it. Recording SHALL be idempotent and keep the first connection timestamp.

#### Scenario: Member completes onboarding
- **WHEN** Bob (id 987654321) sends `/start` in a private chat with the bot
- **THEN** Bob is recorded as connected with the timestamp

#### Scenario: Non-member sends /start
- **WHEN** an unconfigured user sends `/start` privately
- **THEN** the system replies that access is not configured and records no onboarding

#### Scenario: Any private message connects
- **WHEN** Pablito (id 3572757), not connected, sends `/list` in his private chat with the bot
- **THEN** Pablito is recorded as connected and receives his list

#### Scenario: Private button press connects
- **WHEN** Bob, not connected, presses a button on a message in his private chat with the bot
- **THEN** Bob is recorded as connected

#### Scenario: Group command does not connect
- **WHEN** Bob, not connected, sends `/status` in the group
- **THEN** Bob stays not connected

#### Scenario: Repeated private messages
- **WHEN** Pablito, connected since Oct 9 2026 23:10, sends more private messages
- **THEN** his connection timestamp stays Oct 9 2026 23:10

### Requirement: Onboarding status view
The system SHALL provide `/status` that lists every member with their connection state under a header, one line per member marked ✅ when connected and ⏳ when pending, with the pending line saying how to connect.

#### Scenario: Mixed connection states
- **WHEN** `/status` runs and Alice and Bob are connected but Charlie is not
- **THEN** the reply starts with "🏠 Household members" and lists "✅ Alice — connected", "✅ Bob — connected" and "⏳ Charlie — pending: open the bot and tap Start"

### Requirement: Unconnected members are surfaced, not ignored
When the system must send a private message to a member who is not connected, it SHALL NOT drop it silently. It SHALL record the failed delivery and, where a user is waiting on the result, tell them in the originating topic how to connect. That notice SHALL include a button that opens the private chat with the bot and starts it in one tap, when the bot's username is known.

#### Scenario: /list by unconnected member
- **WHEN** Charlie, not connected, sends `/list` in the Chores topic
- **THEN** the topic receives a short notice telling Charlie to open the bot and press Start, with a "🔗 Connect with the bot" button linking to the bot's private chat with a start parameter, and no list is posted in the group

#### Scenario: Due chore for unconnected assignee
- **WHEN** a chore assigned to Charlie becomes due and Charlie is not connected
- **THEN** a failed notification attempt is recorded with reason "not connected" and the occurrence stays open

#### Scenario: Bot username unknown
- **WHEN** the bot username could not be read at startup and Charlie, not connected, sends `/list` in Chores
- **THEN** the notice is sent without the button
