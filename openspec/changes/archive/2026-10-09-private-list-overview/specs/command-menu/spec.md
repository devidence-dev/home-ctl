# Spec Delta

## MODIFIED Requirements

### Requirement: Private command menu
At startup the system SHALL register a menu for all private chats containing `start`, `list`, `status` and `help`, in that order. It SHALL NOT advertise `add`. `list` is advertised because the private `/list` reaches both chores and reminders.

#### Scenario: Private menu
- **WHEN** the application starts
- **THEN** the private-chat menu lists start, list, status, help

#### Scenario: Add not advertised privately
- **WHEN** the private menu is registered
- **THEN** it does not contain `add`
