# Spec Delta

## ADDED Requirements

### Requirement: Track name hints
When the user labels a learn-mode entry and a yard is chosen, learn mode SHALL offer that yard's listed tracks that no labelled section uses yet. It SHALL sort them by how close each listed length is to the measured length of the switch-bounded tracks the entry stands on, and show both lengths. Without the track database, or for an entry with no listed length, it SHALL keep the profile's order and show no measured length. No hint is applied until the user picks it.

#### Scenario: Ranked by length
- **WHEN** a cut stands on a 717 ft unlabelled track and the user chooses yard TUS, with no TUS track labelled yet
- **THEN** the hints start with TY08 (710 ft), then Pass (752 ft), and each hint shows the measured 717 ft

#### Scenario: Used names left out
- **WHEN** some sections are already labelled `TUS / TY08`
- **THEN** TY08 is not offered for TUS

#### Scenario: Siding or main-line entry
- **WHEN** the user picks a listed entry whose kind is siding or main line
- **THEN** the label form switches to ignored, and applying it labels the entry's sections as ignored

#### Scenario: Free text still allowed
- **WHEN** the user types a track name that is not in the yard's list
- **THEN** the label is applied with that name, as before
