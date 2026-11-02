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

## MODIFIED Requirements

### Requirement: Import preview
The preview SHALL group the outcome by location and show counts for each category: new labels, changed `run8` labels, removed `run8` labels, sections left alone because they carry a manual label, and skipped items with reasons. It SHALL also list any route data file that could not be used. Each location SHALL state its counts as track sections, show its measured track length when route data is loaded and the cars standing on those sections in the current save, and list section IDs only on request.

#### Scenario: Manual label kept
- **WHEN** section 280/8346 is manually labelled `NOR / Track 9` and the import would label it `BRR`
- **THEN** the preview lists 280/8346 under kept manual labels, and confirming leaves it as `NOR / Track 9`

#### Scenario: Re-import after an industry moves
- **WHEN** an earlier import labelled 280/1055 as `84L` and the current industry file lists 84L on 280/1060 instead
- **THEN** the preview shows 280/1055 as removed and 280/1060 as new

#### Scenario: Empty bowl track
- **WHEN** the import would label the 4 sections of `NOR / Bowl 1` and no car stands on them in the current save
- **THEN** the preview shows `NOR / Bowl 1` with 4 track sections, its measured length and no cars there now, and lists the 4 section IDs only when the user opens them

#### Scenario: Cars on an imported track
- **WHEN** cars stand on sections the import would label `NOR / Receiving East Thoroughfare`
- **THEN** the preview shows how many cars are there now, with a sample of their car numbers

### Requirement: Receiving track labels from Run8
For each name pattern the profile maps to a yard, the import SHALL find the pairs of AI special locations that mark the two ends of one track. It SHALL label every section on the path between the two ends as a yard track named by the profile's name for that track number, or else by its prefix and the number. A pair whose path crosses a switch or exceeds the search limit SHALL be listed as skipped.

#### Scenario: Norris receiving yard
- **WHEN** the AGS route data is imported
- **THEN** 14 yard tracks are labelled, `NOR / Receiving East Thoroughfare` for `Norris RCV 1`, `NOR / R12` to `NOR / R01` for `Norris RCV 2` to `13`, and `NOR / Receiving West Thoroughfare` for `Norris RCV 14`, covering 120 sections, and no section is in two of them

#### Scenario: End names differ in case
- **WHEN** one end is named `Norris RCV 3 South End` and the other `Norris RCV 3 North end`
- **THEN** both ends match and track `NOR / R11` is labelled

#### Scenario: Number without a name
- **WHEN** a profile's receiving rule has prefix `Receiving` and no name for track 15
- **THEN** track 15 is labelled `Receiving 15`
