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
Learn mode SHALL list every cut and train in the current snapshot that occupies at least one unmapped section. Each entry SHALL show its sections, car count, a sample of car numbers and its destination tags. The user SHALL be able to apply one label to all unmapped sections an entry occupies in a single action. With the track database available, that action SHALL also label the unmapped sections of the switch-bounded tracks the entry stands on, and the entry SHALL list them.

#### Scenario: Labelling a parked cut
- **WHEN** a 12-car cut on unmapped sections 5101 to 5103 is shown and the user labels it as yard track `Norris / Bowl 14`
- **THEN** sections 5101, 5102 and 5103 are all labelled `Norris / Bowl 14`, and the cut no longer appears in the unmapped list

#### Scenario: Whole track labelled
- **WHEN** the 19-car `BRI BRI` cut occupying sections 280/11211, 280/11293, 280/11303 and 280/11319 is labelled as a yard track, with the track database available
- **THEN** all 6 unmapped sections of the switch-bounded track it stands on are labelled, not only the 4 it occupies

#### Scenario: No track database
- **WHEN** the track database is unavailable and the user labels a cut
- **THEN** only the unmapped sections the cut occupies are labelled

### Requirement: Label suggestions
When every car in a cut is tagged for one industry that has no labelled sections, or whose labelled sections connect to the cut's track, learn mode SHALL suggest that industry's track. Otherwise, when the cut's track connects, directly or across one switch, to a labelled section, it SHALL suggest that industry label, or for a yard label its yard alone. Otherwise it SHALL pre-fill the yard with the nearest anchor, within 3 km. No suggestion is applied unaccepted.

#### Scenario: Suggestion from tags
- **WHEN** an unmapped cut contains 7 cars all tagged `A39 GES`
- **THEN** learn mode suggests the industry track label `GES`, and nothing is labelled until the user accepts

#### Scenario: Tagged cut away from its industry
- **WHEN** the 19-car cut tagged `BRI BRI` stands on unlabelled track 608 m from the `East Forwarding Yard` anchor of yard NOR, and BRI's imported sections don't connect to that track
- **THEN** learn mode does not suggest industry `BRI`, and pre-fills yard `NOR`

#### Scenario: Suggestion from a connected yard track
- **WHEN** an unmapped cut with mixed tags stands on a track joined by one switch to a section labelled `NOR / Receiving 3`
- **THEN** learn mode pre-fills yard `NOR` and leaves the track name for the user

#### Scenario: Suggestion from a yard anchor
- **WHEN** an unmapped cut with mixed tags has no labelled neighbour, its nearest anchor is industry `BRI` of yard MCC at 400 m, and the next nearest is 2.5 km away
- **THEN** learn mode pre-fills yard `MCC`

#### Scenario: No anchor in range
- **WHEN** an unmapped cut with mixed tags has no labelled neighbour and no anchor within 3 km
- **THEN** learn mode suggests nothing

#### Scenario: No index guessing
- **WHEN** the track database is unavailable
- **THEN** no neighbour or anchor suggestion is made, and sections whose indices differ by one are not treated as neighbours

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

### Requirement: Import from Run8
Learn mode SHALL offer an import that builds labels from Run8 route data: industry tracks from the industry configuration, hump bowl tracks from the hump controllers, and receiving tracks from named end points. The import SHALL change no label until the user confirms its preview.

#### Scenario: Import offered
- **WHEN** the AGS route data has been read and the user opens learn mode
- **THEN** an import action is offered, and starting it shows a preview without changing any label

#### Scenario: Import cancelled
- **WHEN** the user cancels an import preview
- **THEN** the track map is unchanged

### Requirement: Industry labels from Run8
For each industry in the industry configuration whose tag, after whitespace normalisation, is an industry of the active profile, the import SHALL label every section it lists as an industry track for that tag. Industries whose tag the profile doesn't know SHALL be listed in the preview as skipped.

#### Scenario: Multi-section industry
- **WHEN** the AGS route data is imported
- **THEN** BRL's 8 sections, including 280/548, 280/559 and 280/11776, are labelled as industry track `BRL`

#### Scenario: Padded tag in the industry file
- **WHEN** the industry file lists BRR's entry with the tag ` BRR`, which has a leading space
- **THEN** its section 280/8346 is labelled as industry track `BRR`

### Requirement: Hump bowl labels from Run8
For each hump controller the profile maps to a yard, the import SHALL label each controller track as a yard track named with the profile's prefix and the Run8 track name. A track's sections run from the switch its path ends at, along the branch the path selects, to the next switch or the end of the track.

#### Scenario: Norris bowl
- **WHEN** the AGS route data is imported
- **THEN** 56 yard tracks `NOR / Bowl 1` to `NOR / Bowl 56` are labelled, covering 336 sections, and no section is in two of them

### Requirement: Receiving track labels from Run8
For each name pattern the profile maps to a yard, the import SHALL find the pairs of AI special locations that mark the two ends of one track. It SHALL label every section on the path between the two ends as a yard track named with the profile's prefix and the track number. A pair whose path crosses a switch or exceeds the search limit SHALL be listed as skipped.

#### Scenario: Norris receiving yard
- **WHEN** the AGS route data is imported
- **THEN** 14 yard tracks `NOR / Receiving 1` to `NOR / Receiving 14` are labelled, covering 120 sections, and no section is in two of them

#### Scenario: End names differ in case
- **WHEN** one end is named `Norris RCV 3 South End` and the other `Norris RCV 3 North end`
- **THEN** both ends match and track `NOR / Receiving 3` is labelled

### Requirement: Import preview
The preview SHALL group the outcome by location and show counts for each category: new labels, changed `run8` labels, removed `run8` labels, sections left alone because they carry a manual label, and skipped items with reasons. It SHALL also list any route data file that could not be used.

#### Scenario: Manual label kept
- **WHEN** section 280/8346 is manually labelled `NOR / Track 9` and the import would label it `BRR`
- **THEN** the preview lists 280/8346 under kept manual labels, and confirming leaves it as `NOR / Track 9`

#### Scenario: Re-import after an industry moves
- **WHEN** an earlier import labelled 280/1055 as `84L` and the current industry file lists 84L on 280/1060 instead
- **THEN** the preview shows 280/1055 as removed and 280/1060 as new

### Requirement: Applying an import
Confirming a preview SHALL apply all of it at once: add the new labels, replace changed `run8` labels and remove `run8` labels the import no longer produces, never touching manual labels. When the route data changed after the preview was built, confirming SHALL be refused and the user asked to review a fresh preview.

#### Scenario: Confirmed import
- **WHEN** the user confirms an AGS import on an empty track map
- **THEN** the industry, bowl and receiving labels are saved together, and car status and available orders are recomputed

#### Scenario: Files changed after the preview
- **WHEN** `Config.ind` is replaced between building the preview and confirming it
- **THEN** nothing is applied and learn mode asks the user to review a new preview

### Requirement: Label source
Every label SHALL record its source: `run8` for labels added by an import, manual for all others. Labelling, relabelling or renaming by the user SHALL make the affected labels manual. Labels in a track map without sources, such as one from an earlier version, SHALL count as manual.

#### Scenario: Imported label edited
- **WHEN** the user relabels imported section 280/1055 from `84L` to `COP`
- **THEN** the label becomes manual, and later imports leave it alone

#### Scenario: Older track map
- **WHEN** the app loads a track map written before label sources existed
- **THEN** all its labels load as manual

### Requirement: Run8 track data change notice
After an import, the track map SHALL keep the route data fingerprint it was built from. When the route data read later has a different fingerprint, the system SHALL report that Run8 track data changed since the last import. When route data can be read and no import was ever made, it SHALL report that an import is available.

#### Scenario: Industry file swapped
- **WHEN** the user swaps in a different `Config.ind` after an import and restarts the app
- **THEN** the system reports that Run8 track data changed since the last import
