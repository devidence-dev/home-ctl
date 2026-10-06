# Proposal

## Why

A household needs one shared place to track shopping, chores and reminders without a new app. Telegram is already open on everyone's phone. The home-ctl repository is empty, so this change establishes the whole Phase 1 product: a self-hosted, multi-user Telegram bot backed by SQLite as the source of truth.

## What Changes

- New Go single-binary bot (`cmd/homectl`) for one household mapped to one Telegram forum group with Topics (Shopping, Chores, Reminders), plus private chats with each member.
- Explicitly configured household members (Telegram user id is the identity) with private-chat onboarding tracking and a `/status` view.
- Shopping: reusable item catalog, shared list entries, tap-to-purchase, idempotent.
- Chores and Reminders: definition/occurrence model, assignment, due dates, recurrence, one-off and recurring, completion (including early), postponement, notification attempts, full history.
- Personal task control: only the assignee/target may complete or postpone; `/list` is private and user-scoped.
- Shared recurrence engine with `scheduled` and `completion` anchors, no-backlog policy, calendar-correct month arithmetic.
- Household timezone from `APP_TIMEZONE` (validated at startup); UTC persistence.
- Localization: all user-facing strings from locale files, per-user locale.
- YAML configuration for users, Telegram topics and notifications; secrets from environment only.

## Capabilities

### New Capabilities
- `household-access`: configured members, authorization, private-chat onboarding, `/status`.
- `shopping`: item catalog, shared list entries, purchase transition.
- `chores`: chore catalog, occurrences, assignee-only complete/postpone, private `/list`.
- `reminders`: reminder definitions and occurrences, target-user-only complete/postpone, repeat notifications.
- `recurrence`: rules, anchors, no-backlog policy, month clamping, timezone semantics.
- `notifications`: notification attempts, delivery failures, group status updates.
- `telegram-interaction`: topic-based context, inline-keyboard flows, persisted flow state with TTL, idempotent callbacks.
- `localization-config`: locale resources, YAML config split, env-only secrets, `APP_TIMEZONE`.

### Modified Capabilities
None (no existing specs).

## Non-goals

- Calendar/iCal/CalDAV/WebDAV, web UI, mobile app, email or other push channels.
- Multiple households or multiple groups; per-user timezones; roles and complex permissions.
- Inventory, price or expense tracking, external task-system integrations.
- Any LLM or natural-language interpretation. A proactive planning advisor (Phase 3) is only anticipated by keeping `ListUpcoming(from, to)` read-only queries available.
- Calendar-style date pickers (typed date/time input is enough).

## Impact

- New code: `cmd/homectl`, `internal/{household,shopping,chores,reminders,recurrence,notification,scheduler,flow,i18n,telegram,storage/sqlite,config,clock}`, `config/`, `locales/`, `build/package`, `deployments`, `docs/adr`, `scripts/verify.sh`.
- Dependencies (user-approved): `modernc.org/sqlite`, `gopkg.in/yaml.v3`. Telegram via raw Bot API unless it grows past a small wrapper.
- Runtime: needs `TELEGRAM_BOT_TOKEN` and `APP_TIMEZONE` env vars and a mounted SQLite volume.
