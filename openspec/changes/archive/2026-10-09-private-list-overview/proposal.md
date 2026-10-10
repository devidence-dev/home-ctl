# Proposal

## Why

In the private chat, `/list` works but is not in the command menu, because it only showed chores and the menu must not advertise a command that covers one domain. The list itself shows only one button per item with emoji and name, so a member cannot see when anything is due without opening each item.

## What Changes

- Private `/list` shows a personal overview with two views, chores and reminders, switched by `🧹` and `⏰` buttons that edit the same message. It opens on chores.
- With both domains reachable, `list` is advertised in the private command menu (`start`, `list`, `status`, `help`); `add` stays out.
- Each list (private `/list`, and `/list` from the Chores or Reminders topic, which is delivered privately) shows every item in the message body: number, emoji, name, localized due date and time, recurrence, and an overdue marker. Buttons stay one per item, numbered, to open the detail view.
- Items stay sorted by `due_at`, capped at 20 with the "+N more" line.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `command-menu`: the private menu advertises `list`.
- `chores`: private list layout and the private `/list` overview with a switch to reminders.
- `reminders`: same layout and switch for the reminders view.

## Impact

- `internal/telegram/list.go` (list rendering, view switch), `internal/telegram/commands.go` (private menu), router private `/list`.
- `locales/en.yaml`, `locales/es.yaml`, `docs/manual-tests.md`.
- No domain, storage or schema change; no new dependency.

## Non-goals

- Shopping in the private overview (the shopping list lives in its topic).
- Filters, paging beyond the existing 20-item cap, or editing items from the list.
- Changing the detail view, Done or Postpone behavior.
