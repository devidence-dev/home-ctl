# Proposal

## Why

New chores are always stored with 🧹 and new reminders with ⏰: the creation flows never ask for an emoji, so every item in lists and notifications looks the same. Shopping already asks for an emoji; chores and reminders should let the member choose one too.

## What Changes

- After the name of a new chore, and after the title of a new reminder, the flow asks for an emoji with a few suggestion buttons, Other to type one, and Skip to keep the default (🧹 for chores, ⏰ for reminders).
- A typed emoji is accepted when it is a short emoji without letters or digits; otherwise the member is asked again.
- Selecting an existing chore from the catalog keeps its emoji and does not ask.
- The chosen emoji appears in the context header and confirmation, and is stored on the definition.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `chores`: new chore definitions get an emoji chosen in the flow.
- `reminders`: new reminder definitions get an emoji chosen in the flow.

## Impact

- `internal/telegram` (chores and reminders flow steps, picker renderer, callback kind, typed validation).
- `locales/en.yaml`, `locales/es.yaml` (prompt, buttons, suggestion lists), `docs/manual-tests.md`.
- No domain, storage or schema change: definitions already store an emoji.

## Non-goals

- Editing the emoji of existing definitions.
- Changing the Shopping emoji step.
- Emoji search or keyboards beyond the suggestion row.
