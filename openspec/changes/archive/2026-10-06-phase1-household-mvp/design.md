# Design

## Context

Greenfield Go repository. Layout follows golang-standards/project-layout, using only needed directories (no `pkg/`, `api/`, `web/`, no empty placeholders). Motivation and scope: see `proposal.md`; behavior: see `specs/`. Local Go is unavailable, so build, test and lint run in ephemeral Docker containers.

## Goals / Non-Goals

**Goals:**
- Domain logic independent of Telegram and SQLite, testable with fakes.
- Idempotent, restart-safe state transitions.
- Leave room for a future read-only planning advisor without Phase 1 depending on it.

**Non-Goals:**
- Multi-household support (kept possible by not hardcoding a singleton, but not implemented).
- Generic task engine merging chores and reminders.
- Any LLM dependency.

## Decisions

### Layout and dependency direction

```
cmd/homectl/            main only: config, wiring, startup
internal/
  household/            members, onboarding state
  shopping/  chores/  reminders/     domain + service per capability
  recurrence/           pure rule/anchor/next-slot logic
  notification/         attempt model, Notifier interface
  scheduler/            due scan loop
  flow/                 persisted conversation state
  i18n/                 locale bundles and lookup
  telegram/             adapter: updates -> commands, keyboards, sender
  storage/sqlite/       Store implementations, embedded migrations
  config/  clock/
config/  locales/  build/package/  deployments/  docs/adr/  scripts/verify.sh
```

```
telegram --> chores/reminders/shopping/household --> recurrence
    |                 ^                  |
    v                 | (interfaces      v
  flow, i18n          |  defined here)  notification
                 storage/sqlite implements them
```

Each domain package defines the small interfaces it consumes (`Store`, `Notifier`, `Clock`); `storage/sqlite` and `telegram` implement or call them. No domain package imports Telegram or SQLite (SOLID: dependency inversion, interface segregation). Alternative considered: one generic `task` package for chores and reminders. Rejected: the two diverge (catalog and assignee versus free-form title and repeat-until-ack), and merging couples them. They share `recurrence` and `notification` instead.

### Definition / occurrence / attempt model

`*_definitions` hold the reusable record plus recurrence rule and anchor. `*_occurrences` hold `scheduled_at`, `due_at`, `completed_at`, `completed_by`, `status`. `notification_attempts` link to an occurrence. `postponements` and completions are history rows. History is never hard-deleted. Timestamps are UTC integers or RFC3339 strings, STRICT tables, CHECK for enums, foreign keys on, WAL plus `busy_timeout`, parameterized SQL only, versioned embedded migrations (an applied migration is never edited), multi-step changes in one transaction.

### Idempotency

- Completion and purchase use a conditional `UPDATE ... WHERE status = 'open'` (or `'pending'`) and check rows affected. A repeat becomes a no-op that returns the current state.
- Successor creation happens in the same transaction as completion, protected by a unique key `(definition_id, scheduled_at)` so retries cannot create two successors.
- Notification dedupe: unique `(occurrence_id, due_at, attempt_kind/seq)`, claimed before send. Reminder repeats use `last_notified_at + interval`.
- Telegram update ids are tracked so a redelivered update does not repeat an action.
- Postponement applies only to open occurrences. The service checks authorization, then that the occurrence is open (`ErrOccurrenceResolved`), then the requested time; the store repeats the open check atomically with `UPDATE ... WHERE id = ? AND status = 'open'`, so a concurrent Done and Postpone leave exactly one winner. Postponing to the current `due_at` is a no-op with no extra history row. The notification cycle is keyed to the current `due_at`, so a postponement always notifies at the new due time.

### Authorization in the domain

`Complete(occurrenceID, actorUserID)` and `Postpone(...)` load the occurrence and return `ErrNotAssignee` unless the actor is the assignee or target. The Telegram layer passes `update.from.id` mapped to a member; callback data carries only occurrence id and action (for example `o:123:d`, under the 64-byte limit). Shopping checks only active membership.

### Recurrence engine

Pure functions: `Next(rule, anchor, series origin, scheduled_at, completed_at, loc) -> time`. For `scheduled`, slot k is computed from the series origin (`origin + k*N` units, month clamped each time) and the first slot later than both `scheduled_at` and `completed_at` is chosen; this avoids clamp drift (Jan 31 → Feb 28 → Mar 31). For `completion`, the next slot is `completed_at + N` calendar units at the definition's local time of day. Arithmetic uses `time.Date` in the household location, so DST is correct. The series origin is stored on the definition and is reset when the rule or anchor is edited.

### Time

`APP_TIMEZONE` is loaded and validated once at startup into a `*time.Location` supplied by the `clock` package. Domain code receives the clock and location by injection. Relative dates ("today", "tomorrow", postpone presets) resolve in that location through a single resolver in the Telegram adapter (`dayAt`/`resolveDuePreset`). Date-only choices ("Tomorrow", postponement "Tomorrow") use the configured `default_time_of_day` (`app.yaml`, default 09:00; parsed by `clock.ParseTimeOfDay`); the evening presets carry their own explicit 18:00 and say so in their labels.

### Date and time presentation

`i18n.Bundle.FormatDateTime(locale, t, now, loc)` is the only place user-facing dates are rendered. Weekday names, month names and layouts live in the locale files (`datetime.*`), so supporting a locale is data only; today and tomorrow render as relative days. Stored timestamps stay UTC and locale-free.

### Locale resolution

`app.yaml` `default_locale` (default `en`) is validated at startup against the available `locales/*.yaml`. Resolution order for a message: recipient member's `locale`, then `default_locale`, then `en`. Group topic posts use `default_locale`.

### Flow state

A `flow_state` table keyed `(user_id, chat_id, thread_id)` (thread 0 in private chats) stores a step name and a small JSON payload plus `expires_at`. It is separate from domain tables, written step by step, and read-then-deleted at confirmation. Expired rows are ignored and purged by the scheduler loop.

### Telegram client

Raw Bot API over `net/http` with long polling, behind a small `telegram.Client` interface (send, edit, answer callback, get updates). If the wrapper grows beyond a few hundred lines of maintained code, switch to a small maintained library; the interface isolates that choice. Failures never corrupt domain state: DB commit first, Telegram send after, failure recorded.

### Scheduler

A single loop ticks (for example every 15 seconds), finds open occurrences with `due_at <= now`, claims notification attempts transactionally, then sends. After a restart it reads persisted state, so missed due times produce one notification and no duplicates.

### Future planning advisor

Occurrence stores expose a read-only `ListUpcoming(from, to)`. A later `planning.Advisor` interface, with a deterministic default and an optional LLM-backed implementation, can consume that output. It would have no write access to domain commands; any state change must go through a user-confirmed command. Nothing of this is built in Phase 1.

### Compliance with project principles

- SQLite is the source of truth; Telegram is a projection and input channel.
- Idempotent processing covers retries, restarts and duplicate callbacks.
- External integrations (Telegram) sit behind interfaces and failures are recorded, not swallowed.
- Secrets come only from env and are never logged. Standard library first; approved dependencies are `modernc.org/sqlite` and `gopkg.in/yaml.v3`.

## Risks / Trade-offs

- [Raw Bot API grows large] → keep behind `Client` interface; swap to a maintained library if it exceeds a small size.
- [Persisted flow state adds writes] → tiny rows, household scale, TTL purge.
- [Typed date/time parsing ambiguity] → one documented format plus preset buttons; reject unparsable input with guidance.
- [Series origin edge cases when editing rules] → reset origin on edit, cover in table-driven recurrence tests.
- [Single tick scheduler delays delivery by up to one tick] → acceptable at household scale.
- [Private-chat requirement] → onboarding state exposed through `/status`; undeliverable notifications are recorded and retried.

## Open Questions

- Exact typed date/time format (default proposal: `YYYY-MM-DD HH:MM`, plus `HH:MM` for today).
- Default flow TTL (proposal: 30 minutes).
