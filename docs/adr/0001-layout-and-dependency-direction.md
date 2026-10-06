# ADR 0001: Layout and Dependency Direction

## Status

Accepted

## Context

`home-ctl` is a self-hosted Go Telegram bot for household management covering Shopping, Chores, and Reminders. It must support multiple members, private chats, and a forum group with Topics. SQLite is the authoritative source of truth, while Telegram serves exclusively as the interaction layer.

Local development environment uses Docker-first ephemeral containers for build, test, and verification. Local Go is not required on the host system.

We need a clear architectural layout and strict dependency directions that adhere to idiomatic Go, SOLID principles (dependency inversion and interface segregation), and maintain high testability without coupling business logic to Telegram or SQLite.

## Decision

We adopt a package layout aligned with standard Go project structure (`golang-standards/project-layout`), containing only necessary directories:

```
cmd/homectl/            # Main entrypoint: configuration loading, dependency wiring, graceful startup
internal/
  config/               # Multi-file YAML config loading & validation, env secret and timezone loading
  clock/                # Injectable clock and timezone/calendar helpers
  household/            # Members synchronization, onboarding state, authorization
  shopping/             # Item catalog, shared list entries, tap-to-purchase
  chores/               # Chore definitions, occurrences, assignee-only complete/postpone
  reminders/            # Reminder definitions, occurrences, target-only complete/postpone
  recurrence/           # Pure recurrence engine: rules, anchors, next-slot calculation
  notification/         # Attempt model, delivery state, Notifier interface
  scheduler/            # Due scan loop, transactional claim, notification dispatch
  flow/                 # Ephemeral multi-step conversation state with TTL in SQLite
  i18n/                 # YAML-based per-user locale lookup and fallback
  telegram/             # Telegram adapter: long polling, update dispatch, keyboards, formatting
  storage/sqlite/       # Pure-Go SQLite connection management, embedded migrations, Store implementations
config/                 # Default YAML configuration files (users.yaml, telegram.yaml, app.yaml)
locales/                # Localization bundles (en.yaml, es.yaml)
build/package/          # Multi-stage Dockerfile
deployments/            # Docker Compose setup
docs/adr/               # Architecture Decision Records
scripts/                # verify.sh validation gate
```

### Dependency Direction

Dependencies point strictly inward toward domain models and interfaces:

```
telegram ---> chores / reminders / shopping / household ---> recurrence
   |                     ^                   |
   v                     |                   v
flow, i18n         (interfaces)         notification
                         |
                   storage/sqlite
```

1. **Domain Isolation**: Domain packages (`shopping`, `chores`, `reminders`, `household`, `recurrence`, `notification`) never import `telegram` or `storage/sqlite`.
2. **Interface Inversion**: Each domain package defines consumer-specific interfaces (such as `Store`, `Notifier`, `Clock`).
3. **Infrastructure Adapters**: `storage/sqlite` implements the storage interfaces using `modernc.org/sqlite`. `telegram` implements interaction handling and notification delivery.
4. **Distinct Domains for Chores and Reminders**: Instead of a generic tasks abstraction, Chores and Reminders remain separate packages. They share `recurrence` and `notification` primitives, keeping the distinct behaviors (catalog + assignee vs free-form title + repeated unacknowledged alerts) clean and decoupled.

## Consequences

### Positive
- Domain packages are easily tested with lightweight in-memory fakes without databases or network calls.
- SQLite and Telegram implementations can evolve or be swapped independently without touching domain rules.
- Circular dependencies are prevented by structural design.
- Clear separation between persistent authoritative domain data, ephemeral flow state, and external communication attempts.

### Negative / Trade-offs
- Requires interface definitions in consumer packages and glue wiring in `cmd/homectl`.
- Distinct chore and reminder packages result in some similar struct shapes, but avoid premature and fragile generic abstractions.
