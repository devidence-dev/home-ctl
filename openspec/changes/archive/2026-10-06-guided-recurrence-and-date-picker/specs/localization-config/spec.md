# Spec Delta

## ADDED Requirements

### Requirement: Scheduling picker text localized
Every visible text of the scheduling picker SHALL come from locale resources: prompts, recurrence labels, interval and time labels, month names, weekday abbreviations, Today, Tomorrow, Type date, Other, anchor labels, validation messages, ownership notice and the confirmation summary. Months and weekdays SHALL reuse the locale's date resources so calendar and rendered dates agree. A missing picker key SHALL fail startup like any other required key.

#### Scenario: Spanish calendar
- **WHEN** Charlie (locale `es`) opens the calendar for October 2026
- **THEN** the title reads "octubre 2026", the weekday row is lun mar mié jue vie sáb dom and the shortcuts read Hoy and Mañana

#### Scenario: English summary
- **WHEN** Alice (locale `en`) reaches the confirmation of a monthly chore every 3 months from completion
- **THEN** the summary labels and the recurrence text are English

#### Scenario: Missing picker key
- **WHEN** a required picker key is absent from `en.yaml`
- **THEN** startup fails naming the key
