# Spec Delta

## ADDED Requirements

### Requirement: Singular and plural recurrence text
Recurrence descriptions SHALL use a singular form for an interval of 1 and a plural form otherwise, both from locale resources, wherever recurrence is shown (confirmation, lists, details). Placeholders such as "(s)" or "(es)" SHALL NOT appear.

#### Scenario: Spanish weekly
- **WHEN** a Spanish user sees a chore every 1 week and another every 2 weeks
- **THEN** they read "Cada semana" and "Cada 2 semanas"

#### Scenario: English monthly
- **WHEN** an English user sees a reminder every 1 month and another every 3 months
- **THEN** they read "Every month" and "Every 3 months"
