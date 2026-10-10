# Spec Delta

## MODIFIED Requirements

### Requirement: Private personal list
`/list` in the Reminders topic SHALL deliver the requester's own open occurrences privately with the same grouped layout (traffic-light day headers, emoji and title, "╰┈➤ " time line, recurrence), one unnumbered button per occurrence, detail, Done, Postpone and Back behavior as chores, sorted by `due_at`. The reminders view SHALL offer the switch to the chores view, editing the same message.

#### Scenario: Bob lists reminders
- **WHEN** Bob runs `/list` in Reminders
- **THEN** his private chat shows only reminders targeting Bob, and nothing personal appears in the group

#### Scenario: Reminder entries show date and time
- **WHEN** it is Thu Oct 8 2026 21:00 and Bob (locale `en`) has ⏰ "Buy medicine" due Fri Oct 9 2026 10:00, one time
- **THEN** the body shows "🟢 Tomorrow" with "⏰ Buy medicine" and "╰┈➤ 10:00", with no recurrence text

#### Scenario: Empty reminders view
- **WHEN** Bob switches to reminders and has none open
- **THEN** the message shows the empty-reminders text and still offers the switch back to chores
