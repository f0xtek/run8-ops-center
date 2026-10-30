# Spec Delta

## Purpose

Interprets each car in a world save snapshot as operational status: type, load, weight, length, destination, current location and readiness. Order generation and the UI build on this status.

## ADDED Requirements

### Requirement: Car identity
A car SHALL be identified by its model file name together with its unit number, so the same car can be followed across snapshots. The displayed car number SHALL be the reporting mark derived from the model file name followed by the unit number. When no mark can be derived, only the unit number SHALL be displayed.

#### Scenario: Same car across saves
- **WHEN** a car with model `R8_Reefer_PCF_57_rbld_ARMN.xml` and unit `194887` appears in two consecutive snapshots on different sections
- **THEN** it is treated as one car that moved

### Requirement: Load state and weight
A car SHALL be loaded when its load weight is greater than zero and empty otherwise. Gross weight SHALL be load weight plus tare. Tare SHALL come from the car's Run8 definition when available, and otherwise from the profile default for the car's type.

#### Scenario: Empty car
- **WHEN** a Gondola has load weight 0 and the profile's Gondola default tare is 30 tons
- **THEN** the car is empty with a gross weight of 30 tons

### Requirement: Car length
A car's length SHALL come from its Run8 definition when available. Otherwise it SHALL come from the profile default for the car's type, or from the profile's Unknown default for a car of unknown type.

#### Scenario: Fallback length
- **WHEN** no Run8 definition is available for a Box Car and the profile default is 60 ft
- **THEN** the car's length is 60 ft

### Requirement: Destination classification
After normalising whitespace, each car's destination tag SHALL be classified as one of:
- **industry-bound:** `<local> <industry tag>` for a profile industry;
- **train-bound:** a class code, directly or via an alias, or an industry that a road train carries off-map;
- **foreign:** `<code> .`;
- **unassigned:** `None`, or a yard-internal tag such as `NOR Hump`;
- **unknown:** anything else.

#### Scenario: Industry-bound tag
- **WHEN** a car is tagged `AB45 HTI`
- **THEN** it is industry-bound for Holcim Trading Inc, served by local AB45

#### Scenario: Off-map simulated industry
- **WHEN** the profile lists industry `A28` as carried off-map by road train A28, and a car is tagged `A28 A28`
- **THEN** the car is train-bound for A28 rather than industry-bound

#### Scenario: Train-bound tag
- **WHEN** a car is tagged `CHA`
- **THEN** it is train-bound for road train 108

#### Scenario: Foreign tag
- **WHEN** a car is tagged `CPKC .`
- **THEN** it is foreign and excluded from all orders

#### Scenario: Unknown tag
- **WHEN** a car is tagged `XYZ123`
- **THEN** it is unknown and is listed in the dashboard's attention list

### Requirement: Location resolution
A car's location SHALL be the labelled location of its leading truck's section. If that section is unmapped, the trailing truck's section is used instead. A location is a yard track, an industry track, ignored track or unmapped. Each car SHALL also report the snapshot train it belongs to, and that train's symbol if it is powered.

#### Scenario: Car at industry
- **WHEN** a car's leading truck is on a section labelled as industry track `GES`
- **THEN** the car's location is industry GES

#### Scenario: Car on mainline
- **WHEN** a car's sections are labelled ignored
- **THEN** the car is reported as on line and is not counted as being at any yard or industry

### Requirement: Car service state
Each car SHALL be given exactly one service state:
- **awaiting delivery:** industry-bound and in a yard;
- **working:** at an industry and tagged for that same industry;
- **ready to pull:** at an industry and tagged for anything else;
- **awaiting train:** train-bound and in a yard;
- **en route:** in a powered train on ignored track;
- **other:** none of the above.

#### Scenario: Industry finished processing
- **WHEN** a car at industry HTI changes tag from `AB45 HTI` to `NOL1` between snapshots
- **THEN** its state changes from working to ready to pull

### Requirement: Location dwell time
For each car, the system SHALL record the in-game date at which it was first seen at its current location, and SHALL reset that date when the location changes.

#### Scenario: Dwell reset on move
- **WHEN** a car moves from yard track `Norris / Bowl 14` to industry GES
- **THEN** its dwell start becomes the in-game date of the snapshot in which it was first seen at GES
