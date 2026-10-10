# Spec Delta

## MODIFIED Requirements

### Requirement: Reusable chore definitions
The system SHALL keep chore definitions with an emoji, a name and an optional description. A definition SHALL carry the optional recurrence rule and anchor. When a new chore is created in the flow, the system SHALL ask for its emoji right after the name, offering suggestion buttons, Other to type an emoji, and Skip to keep 🧹. A typed answer SHALL be accepted only when it is non-empty, has no letters or digits and is at most 16 characters; otherwise the member SHALL be asked again. Selecting an existing chore SHALL keep its emoji without asking.

#### Scenario: Create definition
- **WHEN** Alice creates chore 🧹 "Sweep the floor" with no description in the Chores topic
- **THEN** the definition is stored and offered in later `/add` flows

#### Scenario: Blank name rejected
- **WHEN** a user submits an empty name for a new chore
- **THEN** nothing is stored and the user is asked again

#### Scenario: Pick a suggested emoji
- **WHEN** Alice creates chore "Limpiar bebedero de los niños" and presses the 🚰 suggestion
- **THEN** the definition is stored with 🚰 and the confirmation shows "🚰 Limpiar bebedero de los niños"

#### Scenario: Type an emoji
- **WHEN** Alice presses Other and sends "🪴"
- **THEN** the chore is stored with 🪴

#### Scenario: Invalid typed emoji
- **WHEN** Alice presses Other and sends "plants"
- **THEN** she is asked again for an emoji and nothing is stored

#### Scenario: Skip keeps the default
- **WHEN** Alice presses Skip at the emoji step
- **THEN** the chore is stored with 🧹

#### Scenario: Existing chore keeps its emoji
- **WHEN** Alice selects the existing chore 🚿 "Lavar el baño" in `/add`
- **THEN** no emoji is asked and the occurrence uses 🚿
