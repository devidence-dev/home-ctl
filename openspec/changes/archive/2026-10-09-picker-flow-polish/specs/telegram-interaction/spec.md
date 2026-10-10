# Spec Delta

## MODIFIED Requirements

### Requirement: Guided scheduling picker
The Chores and Reminders creation flows SHALL use one shared scheduling picker that collects, in this order: recurrence type (one time, daily, weekly, monthly), interval (only for recurring items, "every N" of the type's unit), first due date, hour, minute, recurrence anchor (only for recurring items), and a final confirmation. The first due date SHALL always be chosen explicitly, including for recurring items, and SHALL be the scheduled origin of the series. Interval choices SHALL be 1 to 12 plus Other for a typed integer from 1 to 99. The anchor choices SHALL be labeled by their meaning (keep the schedule, from completion). Every step after the title SHALL start with a short header of the values already chosen (title with its emoji, assignee or target, date once chosen). The confirmation SHALL show, one line each with a leading emoji, the title with its emoji, the assignee or the target, the date and time, the recurrence, and for recurring items a plain-language sentence for the anchor (the next one keeps the schedule, or the next one is calculated on completion), and SHALL offer Confirm and Cancel, before anything is persisted. Internal labels such as "anchor" SHALL NOT appear in the confirmation.

#### Scenario: Every 2 weeks from a Wednesday
- **WHEN** Alice picks Weekly, interval 2, first due Wed Oct 14 2026 at 10:30, anchor keep the schedule, and confirms
- **THEN** the first occurrence is Oct 14 2026 10:30 and the scheduled slots that follow are Oct 28 and Nov 11 at 10:30

#### Scenario: Every month from the 31st
- **WHEN** Bob picks Monthly, interval 1, first due Jan 31 2027 at 09:00 and anchor keep the schedule
- **THEN** the scheduled slots are Jan 31, Feb 28, Mar 31 and Apr 30 at 09:00

#### Scenario: One-time item skips recurrence steps
- **WHEN** Alice picks One time
- **THEN** the picker asks no interval and no anchor and goes from recurrence type to the first due date

#### Scenario: Interval typed with Other
- **WHEN** Alice presses Other at the interval step and types "15"
- **THEN** the interval is 15 and the flow continues at the date step

#### Scenario: Invalid typed interval
- **WHEN** Alice types "0" or "100" or "abc" at the interval step
- **THEN** she is told to send a whole number from 1 to 99 and the flow stays at the interval step

#### Scenario: Confirmation summary
- **WHEN** the flow reaches confirmation for 🧹 "Limpiar bebedero de los niños", assignee Pablito, today 23:05, every 2 weeks, anchor from completion, for a Spanish user
- **THEN** one message shows the lines "🧹 Limpiar bebedero de los niños", "👤 Pablito", "📅 hoy a las 23:05", "🔁 Cada 2 semanas" and "⏭️ La siguiente se calcula al completar", offers "✅ Confirmar" and "❌ Cancelar", and nothing is stored yet

#### Scenario: One-time confirmation has no anchor line
- **WHEN** Alice reaches confirmation of a one-time reminder ⏰ "Buy medicine" for Bob on Fri Oct 9 2026 10:00
- **THEN** the summary shows the title, "👤 For: Bob", the date and "🔁 Once", and no next-occurrence line

#### Scenario: Context header
- **WHEN** Alice has chosen "Clean kitchen", assignee Alice and Oct 14 2026 and the hour step is shown
- **THEN** the message starts with a header naming 🧹 Clean kitchen, Alice and Oct 14 before the hour prompt

### Requirement: Time selection
After the first due date the picker SHALL ask for the hour with buttons 00 to 23 and then for the minute with buttons 00, 05, 10 and so on up to 55, plus Other for a typed minute from 00 to 59. All values SHALL be interpreted in the household timezone. When the chosen date is today in the household timezone, hour buttons before the current hour and minute buttons not later than the current time SHALL be inert. The hour step SHALL offer Change date, which returns to the calendar keeping the other choices. If the chosen date and time are still not later than now, the system SHALL NOT create an occurrence, SHALL explain in the flow message that the time has passed, and SHALL show the hour step again with the date kept.

#### Scenario: Pick 10:30
- **WHEN** Alice has picked Oct 10 2026, presses hour 10 and then minute 30
- **THEN** the due time is Oct 10 2026 10:30 in the household timezone

#### Scenario: Minute typed with Other
- **WHEN** Alice presses Other at the minute step and types "07"
- **THEN** the minute is 07

#### Scenario: Past hours inert today
- **WHEN** it is 14:20 on Oct 5 2026 and Alice picks Today
- **THEN** hours 00 to 13 are inert, pressing hour 09 changes nothing, and hours 14 to 23 are selectable

#### Scenario: Past minutes inert in the current hour
- **WHEN** it is 14:20 on Oct 5 2026 and Alice picks Today and hour 14
- **THEN** minutes 00 to 20 are inert and 25 to 55 are selectable

#### Scenario: Time earlier today
- **WHEN** Alice picks Today, hour 14 and types minute "10" with Other at 14:20
- **THEN** no occurrence is created, the flow message explains that the time has passed and shows the hour step with today kept

#### Scenario: Change date
- **WHEN** it is 23:58 and Alice, at the hour step for today, presses Change date
- **THEN** the calendar is shown again and her recurrence and interval choices are kept

#### Scenario: Invalid typed minute
- **WHEN** Alice types "75" at the minute step
- **THEN** she is told to send a minute from 00 to 59 and the flow stays at the minute step

### Requirement: Single-message flow editing
All button-driven steps of a creation flow SHALL edit one bot message instead of sending a new message per step. The id of the active flow message SHALL be stored with the persisted flow state so the flow continues across restarts. When a typed answer is accepted, the flow SHALL continue in a new flow message sent after the user's message, SHALL store its id, and SHALL delete the previous flow message, or remove its buttons when deletion fails, so the active flow message is always the latest one and only one flow message has buttons. A rejected typed answer SHALL be handled the same way so the guidance appears below the user's message. When the active flow message cannot be edited, the system SHALL send a replacement flow message, store its id and continue; the flow state and domain data SHALL NOT be affected by the failure. An edit that changes nothing SHALL NOT count as a failure.

#### Scenario: One message through the flow
- **WHEN** Alice runs `/add` in Chores and goes from recurrence type to confirmation using buttons
- **THEN** the same message id is edited at every step and the topic gains no intermediate bot messages

#### Scenario: Continue after restart
- **WHEN** the application restarts while Alice is at the minute step and she then presses a minute button
- **THEN** the flow continues by editing the same message

#### Scenario: Flow message deleted
- **WHEN** Alice's active flow message was deleted and she types an accepted answer
- **THEN** a replacement flow message at the next step is sent, its id is stored, and the flow continues

#### Scenario: Typed answer
- **WHEN** Alice runs `/add` in Chores and types "Limpiar bebedero de los niños"
- **THEN** the assignee step is sent as a new message after her text, the previous flow message is deleted, and later button steps edit the new message

#### Scenario: Old flow message cannot be deleted
- **WHEN** deleting the previous flow message fails after a typed answer
- **THEN** that message loses its buttons and the flow continues in the new message
