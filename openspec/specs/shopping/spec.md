# shopping Specification

## Purpose

Lets household members maintain a reusable catalog of shopping items and one shared list of items currently needed, with idempotent purchase tracking.

## Requirements

### Requirement: Reusable item catalog
The system SHALL keep a catalog of shopping items, each with an emoji and a name. Items SHALL be reusable across list entries. Item names SHALL be unique case-insensitively.

#### Scenario: Create new item
- **WHEN** Alice runs `/add` in the Shopping topic, taps "Add new item", and provides emoji 🥚 and name "Eggs"
- **THEN** the item is stored in the catalog and offered in later `/add` flows

#### Scenario: Duplicate name rejected
- **WHEN** Bob tries to create an item named "eggs" while "Eggs" exists
- **THEN** no item is created and Bob is told the item already exists

#### Scenario: Cancel creation
- **WHEN** Alice cancels midway through new-item creation
- **THEN** no catalog item is created

### Requirement: Add one or more items to the list
Running `/add` in the Shopping topic SHALL show catalog items as selectable buttons and allow selecting one or more before confirming. Each confirmed selection SHALL create a pending list entry recording who added it and when.

#### Scenario: Add two items
- **WHEN** Alice selects 🥚 Eggs and 🥛 Milk and confirms
- **THEN** two pending entries exist, each added by Alice with a creation timestamp

#### Scenario: Item already pending
- **WHEN** Alice confirms 🥚 Eggs while a pending Eggs entry already exists
- **THEN** no duplicate pending entry is created and Alice is told Eggs is already on the list

### Requirement: Shared list view with tap-to-purchase
`/list` in the Shopping topic SHALL render the shared list of pending entries in that topic, one button per entry. Tapping an entry SHALL mark it purchased.

#### Scenario: Render pending entries
- **WHEN** Bob runs `/list` with pending Eggs, Milk and Chicken
- **THEN** the Shopping topic shows three buttons "🥚 Eggs", "🥛 Milk", "🍗 Chicken"

#### Scenario: Empty list
- **WHEN** `/list` runs with no pending entries
- **THEN** the topic shows an "all done" message and no buttons

### Requirement: Household-level purchase authorization
Any active household member SHALL be allowed to mark any pending entry purchased. Purchase SHALL record the purchasing member and timestamp. The entry SHALL NOT be deleted.

#### Scenario: Another member purchases
- **WHEN** Bob taps the Eggs entry added by Alice
- **THEN** the entry becomes purchased with Bob as purchaser and the current timestamp, and the list updates

#### Scenario: Unknown user taps a forwarded button
- **WHEN** a non-member triggers the purchase callback
- **THEN** the entry stays pending

### Requirement: Idempotent purchase transition
The pending-to-purchased transition SHALL occur at most once per entry, even under repeated or concurrent attempts, with no duplicate side effects.

#### Scenario: Double tap
- **WHEN** Bob taps Eggs twice within one second
- **THEN** the entry has exactly one purchased timestamp and one purchaser, and one group update is posted

#### Scenario: Concurrent purchase by two members
- **WHEN** Alice and Bob tap Eggs at the same moment
- **THEN** exactly one of them is recorded as purchaser, the other is told it was already purchased, and only one update is posted

#### Scenario: Purchased entry can be re-added
- **WHEN** Eggs was purchased and Alice adds Eggs again
- **THEN** a new pending entry is created and the purchased entry remains in history
