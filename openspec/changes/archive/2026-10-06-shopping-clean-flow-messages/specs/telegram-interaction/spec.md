# Spec Delta

## ADDED Requirements

### Requirement: Flow keyboards are closed once used
When a flow button press is accepted, the system SHALL remove the inline keyboard from the message that carried it and keep the message text. When a flow button is pressed after its flow expired or was closed, the system SHALL also remove that message's keyboard and tell the user the action expired. A rejected press SHALL leave the keyboard in place. Persistent list and notification keyboards SHALL NOT be affected.

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

#### Scenario: List keyboards stay
- **WHEN** Alice taps "Leche" on the shopping list in the Shopping topic
- **THEN** the list message is refreshed with the remaining pending items still tappable
