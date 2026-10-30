# Spec Delta

## MODIFIED Requirements

### Requirement: Dashboard view
The dashboard SHALL show:
- summary tiles: cars on route, at industries, ready to pull, awaiting delivery, awaiting road trains, and unmapped or unknown cars;
- In Progress and Completion Detected orders;
- Available orders grouped by kind;
- each road train's build percentage toward its threshold;
- an attention list of ingest warnings, profile validation problems, unknown-tag or unmapped cars, Run8 route data files that could not be used, and the Run8 track data notice (changed since the last import, or an import available).

#### Scenario: Attention items
- **WHEN** 12 cars are on unmapped sections and the profile reports an orphan tag
- **THEN** the attention list shows both, with links to learn mode and to the profile report

#### Scenario: Track data changed
- **WHEN** the Run8 track data fingerprint differs from the one recorded at the last import
- **THEN** the attention list says the Run8 track data changed, with a link to the import preview

### Requirement: Learn mode view
The interface SHALL provide the learn mode described by track mapping: a list of unmapped cuts with suggestions, label entry, a list of existing labels with edit and remove actions, and the import from Run8 with its preview. The preview SHALL offer confirm and cancel. Each label in the list SHALL show whether it came from an import or was set by hand.

#### Scenario: Label from the UI
- **WHEN** the user accepts a suggested label for an unmapped cut
- **THEN** the cut disappears from the unmapped list, and the dashboard counts update

#### Scenario: Import from the UI
- **WHEN** the user starts the import, reviews the preview and confirms it
- **THEN** the imported labels appear in the label list marked as imported, cuts on those sections leave the unmapped list, and the dashboard counts update
