# Proposal

## Why

Creating a chore or reminder asks for a due date first, with one preset menu and a free-text fallback, and only then offers recurrence with a fixed interval of 1. Users cannot say "every 2 weeks" or "every 3 months", cannot pick a date or time without typing, and every step adds a new message to the shared topic. Any household member can also press another member's flow buttons.

## What Changes

- Chores and Reminders share one guided scheduling picker with this order: recurrence type (one time, daily, weekly, monthly), interval (every N, only when recurrent), first due date, hour, minute, anchor (only when recurrent), confirmation.
- The first due date is always chosen explicitly and is the origin of the series.
- Date: inline month calendar (7 columns, month navigation, Today, Tomorrow, Type date; past days inert). Time: hour grid 00-23, minute grid in steps of 5, plus Other for a typed minute.
- Interval: buttons 1-12 plus Other for a typed integer.
- All button steps edit one flow message, whose id is persisted with the flow so it survives restarts and falls back to a new message if editing fails.
- A flow message is owned by the user who started it; presses by anyone else change nothing and get a short notice.
- A chore whose definition already carries a recurrence rule keeps skipping the recurrence questions.
- Every label, month, weekday and summary comes from the locale files.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `telegram-interaction`: picker sequence, calendar, time selection, single-message editing, flow ownership; refines typed input and the keyboard-closing rule.
- `chores`: scheduling flow collects recurrence type, interval, explicit first due date and time, anchor.
- `reminders`: creation flow uses the same sequence.
- `localization-config`: picker text comes from locale resources.

## Impact

- `internal/telegram`: shared picker, chores and reminders flow glue, callback kinds, ownership check.
- `internal/flow` and `internal/storage/sqlite`: flow state records the active message id; migration `0002`.
- `locales/en.yaml`, `locales/es.yaml`, `docs/manual-tests.md`.
- No domain or recurrence-semantics change, no new dependency, no config change.

## Non-goals

- New recurrence types, including weekday sets (separate change `weekday-set-recurrence`).
- Natural-language or LLM date parsing.
- Changing recurrence anchor semantics or editing recurrence of existing definitions.
- Changing Shopping flow steps or the postpone picker.
- Per-user timezones.
