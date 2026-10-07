# command-menu Specification

## Purpose

Lets users discover the bot's commands from Telegram's native command menu, registered automatically by the application at startup, localized and scoped to where each command is meaningful.

## Requirements

### Requirement: Group command menu
At startup the system SHALL register a command menu scoped only to the configured household group, containing `add`, `list`, `status` and `help` and no other command. It SHALL NOT register a menu for all groups.

#### Scenario: Household group menu
- **WHEN** the application starts with `telegram.chat_id` -1003916572365
- **THEN** the menu for chat -1003916572365 lists add, list, status, help in that order

#### Scenario: Start not advertised in group
- **WHEN** the group menu is registered
- **THEN** it does not contain `start`

#### Scenario: Other groups unaffected
- **WHEN** the bot is added to a group with id -1009999999999
- **THEN** no command menu was registered for that group

### Requirement: Private command menu
At startup the system SHALL register a menu for all private chats containing `start`, `status` and `help`. It SHALL NOT advertise `add`, and SHALL NOT advertise `list` while the private `/list` shows only one domain.

#### Scenario: Private menu
- **WHEN** the application starts
- **THEN** the private-chat menu lists start, status, help

#### Scenario: Add not advertised privately
- **WHEN** the private menu is registered
- **THEN** it does not contain `add` or `list`

### Requirement: Localized command descriptions
Command descriptions SHALL come from locale resources, command names SHALL be identical in every language, and menus SHALL be registered for language codes `""`, `en` and `es`. The `""` menu SHALL use the default locale's descriptions.

#### Scenario: English menu
- **WHEN** the `en` menu is registered
- **THEN** each description equals the matching text in `en.yaml`

#### Scenario: Spanish menu
- **WHEN** the `es` menu is registered
- **THEN** each description equals the matching text in `es.yaml`

#### Scenario: Fallback menu
- **WHEN** `default_locale: es`
- **THEN** a menu with language code "" is registered for both scopes using `es.yaml` descriptions

#### Scenario: Menu language independent of member locale
- **WHEN** Bob has locale `es` but his Telegram client language is English
- **THEN** he may see English descriptions and the bot's replies to him stay in Spanish

### Requirement: Best-effort registration
Registration SHALL be idempotent, run on every start, and never prevent startup or affect other functionality. A failure for one scope and language SHALL be logged with that scope and language, SHALL NOT stop the remaining registrations, and SHALL NOT send any Telegram message.

#### Scenario: One registration fails
- **WHEN** registering the `es` group menu fails with a Telegram error and the others succeed
- **THEN** the failure is logged naming scope group and language es, and the other five menus are still registered

#### Scenario: All registrations fail
- **WHEN** Telegram is unreachable at startup
- **THEN** every failure is logged, the application starts and polling begins

#### Scenario: Restart
- **WHEN** the application restarts with unchanged locales
- **THEN** the same menus are registered again with the same content

### Requirement: Menu grants no authorization
A command's presence in a menu SHALL NOT authorize it. Every received command SHALL pass the existing household membership, onboarding, Topic and ownership checks.

#### Scenario: Non-member uses advertised command
- **WHEN** user 555000111, not in `users.yaml`, sends `/add` in the group where it is advertised
- **THEN** the command is rejected as for any non-member and no flow starts

#### Scenario: List in an unmapped Topic
- **WHEN** Alice sends `/list` in a Topic id not in configuration
- **THEN** she gets the localized "not available in this topic" notice and no domain list is shown
