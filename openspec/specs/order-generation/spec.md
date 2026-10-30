# order-generation Specification

## Purpose
Builds the list of available work from the current car status. Local job orders have industry lines and yard lines, and road train orders can include en-route stops. Every order is subject to the route's readiness, capacity, length and tonnage rules.

## Requirements

### Requirement: Regeneration on every snapshot
The system SHALL recompute all available orders whenever a new snapshot is ingested, or whenever the route profile or track map changes. Available orders for the same job and kind SHALL keep a stable identity across recomputation.

#### Scenario: New save changes available work
- **WHEN** a new snapshot shows three more cars tagged `AB45 HTI` in Norris
- **THEN** the AB45 job order's HTI line is updated in place, keeping the same order identity

### Requirement: Reserved cars excluded
Cars belonging to an order that is In Progress or Completion Detected SHALL NOT appear in any available order.

#### Scenario: Car already assigned
- **WHEN** a car is a set-out in the in-progress AB45 order
- **THEN** no other available order lists that car

### Requirement: Local job order structure
For each local job, the system SHALL produce at most one available order. The order SHALL contain one line per eligible industry served by the job, and one line per yard line that has work. Each line SHALL list set-out cars and pick-up cars, each with car number, type, load state, tag and current track.

#### Scenario: Job with two eligible industries
- **WHEN** industries STI and HTI, both served by AB45, are eligible
- **THEN** the AB45 order has an STI line and an HTI line

### Requirement: Industry readiness gate
An industry line SHALL be generated only when no car at that industry is in the working state. An industry with no cars on its tracks counts as ready.

#### Scenario: Industry still processing
- **WHEN** one car at GES is still tagged `A39 GES`
- **THEN** no GES line is generated, even if other cars at GES are ready to pull

#### Scenario: All cars retagged
- **WHEN** every car at GES is tagged with an outgoing tag
- **THEN** a GES line is generated that lists those cars as pick-ups

### Requirement: Set-out selection and capacity
The set-outs for an industry line SHALL be cars awaiting delivery to that industry that are in the job's home yard. For each car type, the number of set-outs SHALL NOT exceed that industry's capacity for the type. Cars of unknown type SHALL count against the industry's total capacity. When more cars wait than fit, the cars with the oldest dwell SHALL be chosen.

#### Scenario: Capacity limit
- **WHEN** 8 Box Cars tagged `AB45 JPI` wait in Norris and JPI's Box Car capacity is 6
- **THEN** the JPI line sets out the 6 cars with the oldest dwell, and the other 2 remain awaiting delivery

#### Scenario: Car outside the home yard
- **WHEN** a car tagged `A37 CYC` is in Norris but job A37's home yard is Tuscaloosa
- **THEN** it is not a set-out in any A37 order, and is instead a set-out on A55's Tuscaloosa yard line

### Requirement: Pick-up selection
The pick-ups for an industry line SHALL be all cars at that industry in the ready-to-pull state. The line SHALL state that pick-ups return to the job's home yard.

#### Scenario: Pick-up only line
- **WHEN** two cars at WRG are tagged `CHA` and no cars wait for WRG
- **THEN** the WRG line has two pick-ups returning to Norris and no set-outs

### Requirement: Job limit warning
Each local job order SHALL show the outbound train's total length and gross tons, counting all set-outs, against the job's limits. When a limit is exceeded, the order SHALL be flagged.

#### Scenario: Over-length job
- **WHEN** AB45's set-outs total 4,300 ft and AB45's maximum length is 4,000 ft
- **THEN** the AB45 order is flagged as over length

### Requirement: Yard line set-outs
A local job's yard line drops a block at a satellite yard. Its set-outs SHALL be cars in the job's home yard whose local symbol, industry tag or class code is in the yard line's carried tags. Yard lines have no capacity limit.

#### Scenario: A55 block for Tuscaloosa
- **WHEN** Norris holds cars tagged `A37 CYC`, `A56 TBP` and `AGR AGR`, and A55's Tuscaloosa yard line carries A37, A56 and AGR
- **THEN** the A55 order has a Tuscaloosa line that sets out those cars, alongside its KYK and ABS industry lines

#### Scenario: AB45 block for Bessemer
- **WHEN** Norris holds cars tagged `AB45 BRL`, and AB45's Bessemer yard line carries BRL and WWL
- **THEN** the AB45 order has a Bessemer line that sets out those cars

### Requirement: Yard line pick-ups
A yard line's pick-ups SHALL be the train-bound or industry-bound cars in the satellite yard that do not belong there. A car belongs there when any of the following holds:
- its tags are carried by the yard line;
- it is industry-bound for a job homed at that yard;
- a road train stop at that yard picks it up.

#### Scenario: Pulls returned from Tuscaloosa
- **WHEN** Tuscaloosa holds cars pulled by A56 tagged `SEL` and `NOL3`, plus cars tagged `A37 SIS`
- **THEN** the A55 Tuscaloosa line picks up the `SEL` and `NOL3` cars and leaves the `A37 SIS` cars

#### Scenario: Autorack empties left for road trains
- **WHEN** McCalla holds A39 pulls tagged `CHA` and empty autoracks tagged `NOL`, and road train 131's McCalla stop picks up `NOL`
- **THEN** the A44 McCalla line picks up the `CHA` cars only

### Requirement: Road train orders
For each road train with an origin yard, the system SHALL gather the cars waiting for it in that yard. These are cars train-bound for it plus cars matching its stops' set-outs. When their total length reaches the train's minimum percentage of its maximum length (default 50%), the system SHALL produce one available order. Cars SHALL be taken oldest dwell first, stopping before the next car would exceed the maximum length or tons.

#### Scenario: Below threshold
- **WHEN** cars for 108 total 3,000 ft against a maximum of 8,000 ft and a 50% minimum
- **THEN** no 108 order is available, and the dashboard shows 108 building at 38%

#### Scenario: Threshold reached and capped
- **WHEN** cars for 131 total 9,000 ft against a maximum of 8,000 ft
- **THEN** a 131 order is available listing cars up to 8,000 ft, and the remaining cars stay awaiting train

#### Scenario: Stop set-outs count toward the threshold
- **WHEN** Norris holds 2,500 ft of cars tagged `NOL1` and 2,000 ft tagged `BRI BRI`, and 131 has a McCalla stop that sets out `BRI`
- **THEN** 131's waiting length is 4,500 ft, which reaches 56% and makes an order available

### Requirement: Road train yard stops
A road train's order SHALL include its profile-defined en-route stops, in order. At a yard stop, the set-outs SHALL be the order's cars matching the stop's set-out tags. The pick-ups SHALL be cars in that yard matching the stop's pick-up tags, and the stop's car types when it lists any. Pick-ups SHALL be added oldest dwell first, only while the train after the stop stays within its maximum length and tons.

#### Scenario: 131 works McCalla
- **WHEN** a 131 order includes cars tagged `BRI BRI`, and McCalla holds empty autoracks tagged `NOL` and `MER`
- **THEN** the order's McCalla stop sets out the `BRI BRI` cars and picks up the `NOL` and `MER` racks

#### Scenario: Same tag, different trains by car type
- **WHEN** McCalla holds an empty Autorack Car and a Well Car, both tagged `ATL`, and 346's stop picks up `ATL` Autorack Cars while 28J's stop picks up `ATL` Well Cars
- **THEN** the 346 order picks up only the autorack, and the 28J order only the well car

#### Scenario: Pick-ups limited by length
- **WHEN** 131 leaves McCalla with 600 ft of spare length and 900 ft of `NOL` racks wait there
- **THEN** the stop picks up only the oldest racks that fit within 600 ft

### Requirement: Road train industry stops
At an industry stop, the set-outs SHALL be the order's cars that are industry-bound for that industry. The same readiness gate and per-type capacity rules as local industry lines apply. The pick-ups SHALL be the cars at that industry that are ready to pull.

#### Scenario: A28 works PHS
- **WHEN** an A28 order includes cars tagged `A28 PHS`, and two cars at PHS are tagged `ATL`
- **THEN** the PHS stop sets out the `A28 PHS` cars and picks up the two `ATL` cars

### Requirement: Through-train orders
A road train with no origin yard SHALL be a through train: it enters and leaves the map, and only works its stops. Its order SHALL list only stop pick-ups, and SHALL be available once their total length reaches the train's minimum percentage of its maximum length.

#### Scenario: 346 collects ATL racks
- **WHEN** 346 is a through train with a McCalla stop that picks up `ATL`, and three empty racks tagged `ATL` sit in McCalla
- **THEN** a 346 order is available listing those three racks as pick-ups

### Requirement: Road train age trigger
When a road train has a maximum car age configured, the system SHALL also make its order available once its oldest waiting car's dwell exceeds that age, regardless of the length threshold.

#### Scenario: Old cars force a train
- **WHEN** 341 has a maximum age of 2 in-game days and a car for 341 has waited 3 days at 1,200 ft total
- **THEN** a 341 order is available

### Requirement: Road train power requirement
Each road train order SHALL show its destination, total length, gross tons, and the minimum horsepower required, which is gross tons multiplied by the train's minimum HP per ton.

#### Scenario: Horsepower shown
- **WHEN** a 131 order totals 6,200 gross tons and 131's minimum HP/T is 1.0
- **THEN** the order shows a minimum of 6,200 HP
