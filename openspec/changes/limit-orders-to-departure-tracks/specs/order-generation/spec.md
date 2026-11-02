## ADDED Requirements

### Requirement: Yard cars available to orders
Wherever order generation takes cars that are in a yard, a car SHALL count as in that yard only when it stands on one of the yard's departure tracks, if the profile marks any. In a yard that marks no departure tracks, every yard track SHALL count. This covers industry-line and yard-line set-outs, yard-line pick-ups, the cars waiting for a road train at its origin, and yard-stop pick-ups.

#### Scenario: Car still in the bowl
- **WHEN** a Box Car tagged `AB45 JPI` stands on `NOR / Bowl 14`, and Norris marks departure tracks
- **THEN** no AB45 order sets it out, and the car stays awaiting delivery

#### Scenario: Car trimmed to a forwarding track
- **WHEN** the same car is moved to `NOR / WF03`, which is a Norris departure track
- **THEN** it is a set-out on AB45's JPI line

#### Scenario: Receiving cars do not build a road train
- **WHEN** Norris holds 3,000 ft of cars for 108 on `NOR / R07` and 1,200 ft on `NOR / EF05`, against a maximum of 8,000 ft and a 50% minimum
- **THEN** 108's waiting length is 1,200 ft, no 108 order is available, and the dashboard shows 108 building at 15%

#### Scenario: Unlisted Norris track
- **WHEN** a car tagged `A37 CYC` stands on `NOR / All`, a name Norris doesn't list
- **THEN** it is not a set-out on A55's Tuscaloosa yard line

#### Scenario: Yard without departure tracks
- **WHEN** Tuscaloosa marks no departure tracks and holds A56 pulls tagged `SEL` on `TUS / TY04`
- **THEN** the A55 Tuscaloosa line picks them up

#### Scenario: Local yard
- **WHEN** cars tagged `AB10 BRR` stand on `NOR / L08`, which is a Norris departure track
- **THEN** they are set-outs on AB10's BRR line
