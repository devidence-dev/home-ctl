# Tasks

## 1. Close flow keyboards

- [x] 1.1 In `handleFlowCallback` close the callback message's keyboard (edit with its own text and an empty keyboard, errors logged at debug) when the flow was cleared or advanced after the handler, and when the press found no flow state; verify with a test that a Shopping item pick, a Cancel and an expired press each leave the menu message edited with an empty keyboard.
- [x] 1.2 Verify with a test that a rejected press (item already pending) neither edits the message nor clears the flow, and that the shopping list purchase refresh still keeps its remaining buttons.

## 2. Other flows

- [x] 2.1 Add Chores tests: an assignee pick and Cancel close their step messages; the Confirm press closes the summary and still creates the chore once.
- [x] 2.2 Add a Reminders test: Confirm closes the summary and still creates the reminder once; a repeated press after closing hits the expired path and writes nothing.
- [x] 2.3 Verify with a test that an edit failure from the client is ignored and the flow still completes.

## 3. Validation

- [x] 3.1 Run `scripts/verify.sh` and require it to pass.
