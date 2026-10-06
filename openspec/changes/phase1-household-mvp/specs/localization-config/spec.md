# Spec Delta

## Purpose

Defines how the application is configured and localized: split YAML config, secrets and timezone from the environment, startup validation, and externalized per-user user-facing text.

## ADDED Requirements

### Requirement: Configuration separated by responsibility
Configuration SHALL be split into `users.yaml`, `telegram.yaml` and `app.yaml`. The Telegram group id and topic ids SHALL come from configuration and SHALL NOT be hardcoded.

#### Scenario: Valid configuration
- **WHEN** all three files are present and valid
- **THEN** the application starts

#### Scenario: Invalid YAML
- **WHEN** `telegram.yaml` has a syntax error
- **THEN** startup fails with an error naming the file

### Requirement: Secrets only from environment
The Telegram bot token SHALL come from an environment variable and SHALL NOT be read from YAML or written to logs.

#### Scenario: Missing token
- **WHEN** the token variable is unset
- **THEN** startup fails without printing any secret

#### Scenario: Token in YAML ignored
- **WHEN** a YAML file contains a `token` key
- **THEN** startup fails with an error that secrets must come from the environment

### Requirement: Household timezone from environment
The system SHALL read the household timezone from `APP_TIMEZONE` and validate it as an IANA timezone at startup. A missing or invalid value SHALL abort startup. The timezone SHALL NOT be configurable in YAML.

#### Scenario: Valid timezone
- **WHEN** `APP_TIMEZONE=America/Guayaquil`
- **THEN** the application starts and uses it for all calendar semantics

#### Scenario: Invalid timezone
- **WHEN** `APP_TIMEZONE=Mars/Olympus`
- **THEN** startup fails with an error naming the value

#### Scenario: Missing timezone
- **WHEN** `APP_TIMEZONE` is unset
- **THEN** startup fails

### Requirement: Configurable notification and flow settings
The default locale, repeat interval for unresolved reminders, flow-state TTL and default time of day for relative dates SHALL come from `app.yaml` with documented defaults and positive-value validation.

#### Scenario: Interval from config
- **WHEN** `notifications.unresolved_reminder_interval: 1h`
- **THEN** unresolved reminders repeat hourly

#### Scenario: Non-positive interval
- **WHEN** the interval is `0s`
- **THEN** startup fails

### Requirement: All user-facing text localized
Every user-facing string (replies, button labels, errors, confirmations, notifications, help and status text) SHALL come from locale resources (`locales/en.yaml`, `locales/es.yaml`), not from code. Each member SHALL have a locale, defaulting to English.

#### Scenario: Spanish member
- **WHEN** a notification is sent to Bob (locale `es`)
- **THEN** it is rendered from `es.yaml`, with button labels in Spanish

#### Scenario: Group message language
- **WHEN** a status update is posted to a topic
- **THEN** it uses the application default locale from `app.yaml` `default_locale` (`en` when unset)

#### Scenario: Member without locale
- **WHEN** a member in `users.yaml` has no `locale` and `default_locale: es`
- **THEN** that member's messages use Spanish

#### Scenario: Unsupported default locale
- **WHEN** `default_locale: fr` and no `locales/fr.yaml` exists
- **THEN** startup fails naming the locale

#### Scenario: Missing key falls back
- **WHEN** a key is absent in `es.yaml` but present in `en.yaml`
- **THEN** the English text is used and the missing key is logged

#### Scenario: Locale file completeness at startup
- **WHEN** a key referenced as required is missing from `en.yaml`
- **THEN** startup fails naming the key

### Requirement: Developer-facing text in English
Source code, configuration keys, database identifiers, internal names, logs and documentation SHALL be in English.

#### Scenario: Spanish user, English logs
- **WHEN** an error occurs while serving Bob (locale `es`)
- **THEN** the log entry is in English and contains no secrets
