# route-profiles Specification

## Purpose
Describes a Run8 route's operating scheme: its industries, yards, jobs, road trains, limits and car-type rules. With that description the operations logic works for AGS South Phase 1 and for other routes without code changes.

## Requirements

### Requirement: Selectable route profile
The system SHALL operate on exactly one active route profile at a time, chosen by the user from the profiles in the data directory. On first run, the system SHALL install the bundled AGS South Phase 1 profile and make it active.

#### Scenario: First run
- **WHEN** the system starts with an empty data directory
- **THEN** the AGS South Phase 1 profile is installed and active

#### Scenario: Switching profile
- **WHEN** the user selects a different installed profile
- **THEN** all car status and orders are recomputed against that profile, and in-progress orders from the previous profile are kept but hidden until it is reactivated

### Requirement: Industry configuration import
The system SHALL load industries from a Run8-format industry configuration CSV, which has a title row, a header row, and continuation rows with a blank name. For each industry it SHALL record name, incoming tag, local symbol and one entry per car type. Each car-type entry holds processing time, capacity, produces (Loads or Empties) and outgoing tags. Tags SHALL be trimmed, with internal whitespace collapsed.

#### Scenario: Continuation rows
- **WHEN** the CSV lists `CMC Steel,CMC,AB10,Gondola,...` followed by two rows with a blank name
- **THEN** industry CMC has three car-type entries: Gondola, Covered Hopper and Bulkhead_Flat

#### Scenario: Padded incoming tag
- **WHEN** the CSV incoming tag is ` BRR`
- **THEN** the industry's incoming tag is `BRR`, and a car tagged `AB10  BRR` matches it

### Requirement: Road train and job limits import
The system SHALL load train limits from a tab-separated file with the columns base train symbol, class codes, destination, minimum HP per ton, maximum length in feet, maximum tons and maximum trailing tons. Blank numeric cells mean no limit.

#### Scenario: Class codes mapped
- **WHEN** the limits row for `131` lists class codes `BRI, NOL1`
- **THEN** a car with outgoing tag `NOL1` is routed to road train `131`, which has a 8,000 ft and 8,000 ton limit

### Requirement: Default save folders
A profile SHALL define the route's default Run8 trains folder and auto-save folder.

#### Scenario: AGS defaults
- **WHEN** the AGS profile is active
- **THEN** the trains folder is `C:\Run8Studios\Run8 Train Simulator V3\Content\V3Routes\Regions\GulfCoast\Trains`, and the auto-save folder is `C:\Run8Studios\Run8 Train Simulator V3\Content\V3Routes\Regions\GulfCoast\AutoSaves`

### Requirement: Yard definitions
A profile SHALL define each yard with a name and a short code.

#### Scenario: AGS yards
- **WHEN** the AGS profile is active
- **THEN** the yards Norris, Tuscaloosa, McCalla, Bessemer and 32nd Street are defined

### Requirement: Local job definitions
A profile SHALL define local jobs. Each has a unique ID, a train symbol, a home yard, the industries it serves, and zero or more yard lines. By default it serves the CSV industries whose local symbol matches its symbol; an explicit list replaces that default. A yard line names a satellite yard and the tags it carries there, which can be local symbols, industry tags or class codes.

#### Scenario: Local with a yard line
- **WHEN** the AGS profile is active
- **THEN** local A55 is homed at Norris, serves KYK and ABS, and has a Tuscaloosa yard line carrying A37, A56 and AGR

#### Scenario: Local served from a satellite yard
- **WHEN** the AGS profile is active
- **THEN** local A39 is homed at McCalla, and local A44 has a McCalla yard line carrying A39

### Requirement: Road train definitions
A profile SHALL define road trains. Each has a symbol and an optional origin yard; without an origin yard it is a through train. Each also lists its class codes and any off-map industries it carries, and its ordered stops. A stop is either a yard with set-out tags, pick-up tags and optional pick-up car types, or an industry. A profile SHALL be able to override the limits-file values, or supply them, for any road train.

#### Scenario: Road train with a stop
- **WHEN** the AGS profile is active
- **THEN** road train 131 originates at Norris with class codes BRI and NOL1, and has a McCalla stop that sets out `BRI` and picks up `NOL` and `MER`

#### Scenario: Through train
- **WHEN** the AGS profile is active
- **THEN** road train 346 has no origin yard and a McCalla stop that picks up `ATL`

### Requirement: Train thresholds
A profile SHALL define a minimum length percentage and an optional maximum car age for each road train.

#### Scenario: Default road threshold
- **WHEN** the AGS profile is active and no override is set for 108
- **THEN** 108's minimum length percentage is 50

### Requirement: Car type and dimension rules
A profile SHALL map car model file name patterns to the industry car types. It SHALL provide a default length in feet and a default tare weight in tons for each car type. A car whose model matches no pattern SHALL have type Unknown.

#### Scenario: Model pattern match
- **WHEN** a car's model file is `R8_Gon_52ft_NS01.xml` and the profile maps `R8_Gon_` to Gondola
- **THEN** the car's type is Gondola

### Requirement: Tag aliases
A profile SHALL allow outgoing tags to be aliased to another tag, so that tags with no road train in the limits file can be routed without editing Run8 data.

#### Scenario: Alias routes an orphan tag
- **WHEN** the profile aliases `MERCP` to `MER`
- **THEN** cars tagged `MERCP` are routed to road train `339`

### Requirement: Profile validation report
On loading a profile, the system SHALL report every problem it finds while still loading the valid parts. Problems to report are:
- outgoing tags that resolve to no train or alias;
- malformed tags, such as tags containing a space;
- industries served by no local job or road train stop, and not carried off-map by a road train;
- yard lines or stops that reference an unknown yard or industry;
- duplicate tags.

#### Scenario: Malformed outgoing tag
- **WHEN** an industry lists the outgoing tag `ATL NOL3`
- **THEN** the validation report flags it, and the industry's other outgoing tags still load

#### Scenario: Orphan outgoing tag
- **WHEN** an outgoing tag `ROS` matches no class code and no alias
- **THEN** the validation report lists `ROS` together with the industries that produce it
