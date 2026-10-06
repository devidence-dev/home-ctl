# Tasks

## 1. Project scaffold and validation gate

- [x] 1.1 Create `go.mod`, `cmd/homectl/main.go` (wiring only), `.gitignore` additions and `build/package/Dockerfile`; verify `docker run --rm -v "$PWD":/app -w /app golang:<version> go build ./...` succeeds. Add `modernc.org/sqlite` and `gopkg.in/yaml.v3` only (user-approved).
- [x] 1.2 Copy `/home/px1/repositories/leafctl/scripts/verify.sh` and `.golangci.yml` into the repo and adapt: cache dir `~/.cache/homectl`, coverage filter `github.com/px1/home-ctl/cmd/` matching the `go.mod` module path, Docker image pins matching `go.mod`'s Go version, remove the "no Go files, exit 0" shortcut (main.go exists after 1.1), and gitignore `coverage.out` and `coverage_prod.out`; verify `scripts/verify.sh` runs green on the scaffold and fails when a file is left unformatted.
- [x] 1.3 Add `docs/adr/0001-layout-and-dependency-direction.md` and `README.md` with the build/run/verify commands; verify the documented commands run as written.

## 2. Config, clock, timezone

- [x] 2.1 Implement `internal/config`: load `users.yaml`, `telegram.yaml`, `app.yaml`, env `TELEGRAM_BOT_TOKEN` and `APP_TIMEZONE`; reject duplicate user ids, missing topics, YAML secret keys, non-positive durations, unsupported `default_locale`. Verify with table-driven tests for each spec scenario in `localization-config` and `notifications` (missing topic).
- [x] 2.2 Implement `internal/clock` (injectable now, location from `APP_TIMEZONE`, local day/tomorrow helpers); verify tests for "tomorrow near midnight" in `America/Guayaquil` and an invalid-timezone failure.

## 3. SQLite storage foundation

- [x] 3.1 Implement `internal/storage/sqlite` open helper (WAL, busy_timeout, foreign keys) and embedded versioned migration runner; verify a test applies migrations to a temp DB twice without error and rejects a modified applied migration.
- [x] 3.2 Write migration 0001 with STRICT tables and CHECK constraints for users, onboarding, shopping items/entries, chore and reminder definitions/occurrences, postponements, notification attempts, flow_state, processed updates; verify constraint tests (invalid status, duplicate `(definition_id, scheduled_at)`).

## 4. Recurrence engine

- [x] 4.1 Implement `internal/recurrence` rule validation and `scheduled` anchor next-slot logic with series origin; verify table tests for early completion (Mon Jan 12), late completion (Feb 2) and no-backlog skipping.
- [x] 4.2 Implement `completion` anchor and month clamping; verify tests for Mar 14 → Jun 14, Jan 31 + 3 months → Apr 30, and monthly origin Jan 31 → Feb 28, Mar 31, Apr 30, May 31.
- [x] 4.3 Verify DST behavior (`America/New_York` weekly 18:00 across March change) and invalid intervals (0, negative) with tests.

## 5. Household access

- [x] 5.1 Implement `internal/household` service, member sync from config (deactivate removed members, match by user id), and SQLite store; verify tests for removal keeping history and username change.
- [x] 5.2 Implement onboarding recording on private `/start`, disconnect on blocked-bot, and the `/status` data; verify tests for connected, non-member and not-connected states.

## 6. Shopping

- [x] 6.1 Implement item catalog (unique case-insensitive name) and add-to-list service with duplicate-pending guard; verify tests for duplicate name and already-pending entry.
- [x] 6.2 Implement purchase transition with conditional update; verify tests for double tap, concurrent purchase by two members (`-race`) and non-member rejection.

## 7. Chores

- [x] 7.1 Implement chore definitions and occurrence creation (`scheduled_at` = `due_at`, past time rejected); verify tests including abandoned creation.
- [x] 7.2 Implement `Complete` with actor check, early completion, idempotency and transactional successor creation; verify tests for assignee, non-assignee rejection, double Done and retry producing one successor.
- [x] 7.3 Implement `Postpone` changing only `due_at` with record and notification reset; verify tests for 3-hour, tomorrow, non-assignee and unparsable input.
- [x] 7.4 Implement `ListMyOpen` ordering by `due_at` with 20-item cap and "+N more"; verify tests for ordering, isolation between members and the 35-item case.

## 8. Reminders

- [x] 8.1 Implement reminder definitions/occurrences, target-only `Complete`/`Postpone`, successor idempotency; verify tests mirroring section 7 scenarios.
- [x] 8.2 Implement repeat-notification policy using the configured interval; verify tests for 10:00/11:00/12:00 repeats, postpone stopping repeats and restart not duplicating.
- [x] 8.3 Implement `ListMyOpen` for reminders; verify test showing only the target's reminders.

## 9. Notifications and scheduler

- [x] 9.1 Implement attempt recording and the `Notifier` interface with a fake; verify attempts stay separate from occurrences and failures are recorded without resolving.
- [x] 9.2 Implement the scheduler loop with transactional claim; verify tests for missed-while-down (one notification), restart no duplicate, unconnected user failure and retry on next scan.
- [x] 9.3 Implement group status update posting after commit with failure recording; verify test that a failed post leaves committed state intact.

## 10. Flow state and i18n

- [x] 10.1 Implement `internal/flow` persisted state keyed `(user, chat, thread)` with TTL and purge; verify tests for concurrent users, restart survival, expiry and independent topics.
- [x] 10.2 Implement `internal/i18n` with `locales/en.yaml` and `locales/es.yaml`, fallback to English with logged missing keys, startup check for required keys; verify tests for Spanish rendering, fallback and missing required key.

## 11. Telegram adapter

- [x] 11.1 Implement `internal/telegram` client over `net/http` (long polling, send, edit, answer callback) behind an interface, with processed-update tracking; verify tests against an `httptest` server and redelivered update ids.
- [x] 11.2 Implement topic routing, chat/topic validation, authorization of members and compact callback data parsing; verify tests for unmapped topic, wrong chat, malformed callback and forwarded-button actor.
- [x] 11.3 Implement shopping flows and `/list` in the Shopping topic (add, new item, tap to purchase); verify tests with a fake client for the shopping success scenario.
- [x] 11.4 Implement chore and reminder `/add` flows including assignee, due date, typed date/time, recurrence and anchor, confirm and cancel; verify tests for the chore and reminder success scenarios and flow cancel.
- [x] 11.5 Implement private `/list` (one message list, detail, Done, Postpone, Back) and the notification buttons sharing the same commands; verify tests for same-message editing, stale detail and unconnected-member notice.
- [x] 11.6 Implement `/start`, `/status` and help text; verify tests for onboarding states output.

## 12. Wiring, deployment and integration

- [x] 12.1 Wire everything in `cmd/homectl/main.go` and add `deployments/docker-compose.yml` with env, config and data volumes; verify the container starts with fake config and fails fast on invalid `APP_TIMEZONE`.
- [x] 12.2 Add an end-to-end test with real temp SQLite and fake Telegram for the three success-criteria workflows (shopping, chore weekly with Done, reminder with Postpone) including a restart in the middle; verify it passes.
- [x] 12.3 Run `scripts/verify.sh` and require it to pass on the current code (gofmt, vet, golangci-lint with zero issues, `-race` tests, >= 80% coverage in aggregate and per production package, excluding `cmd/` and packages without statements).

## 13. Post-verification hardening

- [x] 13.1 Make postponement reject resolved occurrences (authorize, then open check, then time check) with an atomic conditional update and a no-op for repeated identical postponements; verified by domain tests, sqlite tests (completed, duplicate, 25-round concurrent Done vs Postpone) and a stale Telegram callback test.
- [x] 13.2 Scope the notification cycle to the current `due_at` so postponement notifies at the new due time; verified by the sqlite test that old-`due_at` attempts are ignored.
- [x] 13.3 Localize all remaining user-facing strings, never expose raw error text, and add en/es key and format-verb parity checks; verified by `TestShippedLocalesAreInSync` and the Telegram localization tests.
- [x] 13.4 Use `default_time_of_day` through one resolver for "Tomorrow" presets and postponement; verified by Telegram tests with a configured 07:30 and an invalid-value fallback.
- [x] 13.5 Add the locale-aware date/time formatter and use it at every render site; verified by i18n formatter tests (en, es, relative days, UTC input in `America/Guayaquil`, year, malformed names) and a Telegram test with a Spanish member.
- [x] 13.6 Add read-only `ListUpcoming(from, to)` on chore and reminder stores and services (Phase 3 hook, no Phase 3 behavior); verified by sqlite and service tests.
- [x] 13.7 Enforce >= 80% coverage per production package in `scripts/verify.sh`; verified by running it against a fake low-coverage package and the real tree.
- [x] 13.8 Offer the typed "Choose date/time" option in the chore and reminder postpone menus (persisted flow, same `Postpone` command, same authorization and resolved checks); verified by Telegram tests for success, unparsable and past input, cancel, completion while typing, and a non-owner.
