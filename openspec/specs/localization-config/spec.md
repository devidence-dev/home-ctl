# localization-config Specification

## Purpose

Defines how the application is configured and localized: split YAML config, secrets and timezone from the environment, startup validation, and externalized per-user user-facing text.

## Requirements

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

#### Scenario: Date-only choices use the default time of day
- **WHEN** `default_time_of_day: "07:30"` and a user picks "Tomorrow" in a due-date flow or in postponement
- **THEN** the resulting time is tomorrow 07:30 in the household timezone, and the button label shows 07:30

#### Scenario: Missing default time of day
- **WHEN** `default_time_of_day` is unset
- **THEN** it defaults to 09:00

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

### Requirement: Localized date and time presentation
Dates and times shown to users SHALL be rendered in the household timezone using weekday names, month names and layouts from the recipient's locale resources. Today and tomorrow SHALL render as relative days. Persisted timestamps and domain logic SHALL remain locale-independent.

#### Scenario: Spanish member sees Spanish dates
- **WHEN** Charlie (locale `es`) opens a chore due Saturday Oct 10 2026 10:00 in America/Guayaquil
- **THEN** the detail shows "sáb 10 oct, 10:00" and no English weekday or month

#### Scenario: Relative days
- **WHEN** a reminder is due 09:00 the day after today
- **THEN** English users see "tomorrow at 09:00" and Spanish users see "mañana a las 09:00"

#### Scenario: Day boundary follows the household timezone
- **WHEN** it is 23:30 on Oct 5 in America/Guayaquil (04:30 UTC Oct 6) and a chore is due 18:00 that day
- **THEN** it renders as "today at 18:00", not tomorrow

#### Scenario: Group updates use the default locale
- **WHEN** Charlie (`es`) postpones a chore
- **THEN** the Chores topic message is rendered in `default_locale`

### Requirement: Scheduling picker text localized
Every visible text of the scheduling picker SHALL come from locale resources: prompts, recurrence labels, interval and time labels, month names, weekday abbreviations, Today, Tomorrow, Type date, Other, anchor labels, validation messages, ownership notice and the confirmation summary. Months and weekdays SHALL reuse the locale's date resources so calendar and rendered dates agree. A missing picker key SHALL fail startup like any other required key.

#### Scenario: Spanish calendar
- **WHEN** Charlie (locale `es`) opens the calendar for October 2026
- **THEN** the title reads "octubre 2026", the weekday row is lun mar mié jue vie sáb dom and the shortcuts read Hoy and Mañana

#### Scenario: English summary
- **WHEN** Alice (locale `en`) reaches the confirmation of a monthly chore every 3 months from completion
- **THEN** the summary labels and the recurrence text are English

#### Scenario: Missing picker key
- **WHEN** a required picker key is absent from `en.yaml`
- **THEN** startup fails naming the key

### Requirement: Singular and plural recurrence text
Recurrence descriptions SHALL use a singular form for an interval of 1 and a plural form otherwise, both from locale resources, wherever recurrence is shown (confirmation, lists, details). Placeholders such as "(s)" or "(es)" SHALL NOT appear.

#### Scenario: Spanish weekly
- **WHEN** a Spanish user sees a chore every 1 week and another every 2 weeks
- **THEN** they read "Cada semana" and "Cada 2 semanas"

#### Scenario: English monthly
- **WHEN** an English user sees a reminder every 1 month and another every 3 months
- **THEN** they read "Every month" and "Every 3 months"
