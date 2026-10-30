# track-mapping Specification

## Purpose
Links Run8's numeric track sections to named yard tracks and industry tracks, using a learn mode the user works through in the app. Car locations can then be resolved from world saves, which carry no track names.

## Requirements

### Requirement: Location labels
A track section, identified by route prefix and section index, SHALL be labelled as one of:
- **yard track:** a yard plus a track name;
- **industry track:** one or more industry tags;
- **ignored:** mainline or other track that is not a location.

A section with no label SHALL be treated as unmapped.

#### Scenario: Industry track labelled
- **WHEN** the user labels section 280/4535 as an industry track for `GES`
- **THEN** cars whose location resolves to 280/4535 are reported at industry GES

#### Scenario: Unmapped section
- **WHEN** a car sits on a section with no label
- **THEN** the car's location is reported as unmapped, with its section index

### Requirement: Learn from occupied track
Learn mode SHALL list every cut and train in the current snapshot that occupies at least one unmapped section. Each entry SHALL show its sections, car count, a sample of car numbers and its destination tags. The user SHALL be able to apply one label to all unmapped sections an entry occupies in a single action.

#### Scenario: Labelling a parked cut
- **WHEN** a 12-car cut on unmapped sections 5101 to 5103 is shown and the user labels it as yard track `Norris / Bowl 14`
- **THEN** sections 5101, 5102 and 5103 are all labelled `Norris / Bowl 14`, and the cut no longer appears in the unmapped list

### Requirement: Label suggestions
When every car in a cut is tagged for the same industry, learn mode SHALL suggest that industry's track as the label. When the cut's sections neighbour sections that already have a label, learn mode SHALL suggest that neighbouring label. The user SHALL accept or override each suggestion; no label is applied automatically.

#### Scenario: Suggestion from tags
- **WHEN** an unmapped cut contains 7 cars all tagged `A39 GES`
- **THEN** learn mode suggests the industry track label `GES`, and nothing is labelled until the user accepts

### Requirement: Label editing
The user SHALL be able to view all labelled sections grouped by location, rename a location, relabel a section, and remove a label.

#### Scenario: Correcting a mistake
- **WHEN** the user changes section 5101 from `Norris / Bowl 14` to `Norris / Bowl 15`
- **THEN** car status and available orders are recomputed with the new label

### Requirement: Track map persistence
Labels SHALL be stored in the active route profile, survive restarts, and be shareable by copying the profile.

#### Scenario: Restart keeps labels
- **WHEN** the app is restarted after labelling sections
- **THEN** all labels are present and applied to the next snapshot

### Requirement: Mapping coverage indicator
The system SHALL show how many cars in the current snapshot sit on unmapped sections. Cars on unmapped sections SHALL NOT be included in generated orders.

#### Scenario: Partial coverage
- **WHEN** 40 of 458 cars are on unmapped sections
- **THEN** the dashboard shows 40 unmapped cars with a link to learn mode
