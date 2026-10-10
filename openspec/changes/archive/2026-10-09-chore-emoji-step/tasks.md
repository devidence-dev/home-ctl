# Tasks

## 1. Picker pieces

- [x] 1.1 Add `fl:em:<i>`, `fl:em:o` and `fl:em:skip` to `callback.go`; verify with parse tests, including an out-of-range index rejected by the handler.
- [x] 1.2 Add `RenderEmojiStep` and `ValidateTypedEmoji` with the locale suggestion lists; verify with renderer tests (two rows of four, Other, Skip, Cancel) and table tests for "🪴", "🇪🇨", "👍🏽", "plants", "7", "".

## 2. Flows

- [x] 2.1 Insert the emoji step after the chore name and after the reminder title, skipping it for existing chores; verify with flow tests for a suggested emoji (🚰), a typed emoji (🪴), an invalid typed answer, Skip (🧹 and ⏰), and an existing chore keeping 🚿.
- [x] 2.2 Verify with a test that the chosen emoji appears in the context header and confirmation and is stored on the definition.

## 3. Documentation and validation

- [x] 3.1 Add the emoji keys to both locales and required keys, and update `docs/manual-tests.md`; verify with the localization completeness test.
- [x] 3.2 Run `scripts/verify.sh` and require it to pass on the current code.
