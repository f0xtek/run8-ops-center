## MODIFIED Requirements

### Requirement: Yard track lists
A yard SHALL be able to list its tracks. Each entry has a name, an optional length in feet, a kind (yard track, siding or main line) and an optional departure flag that only yard tracks may carry. Names SHALL be unique within a yard after whitespace normalisation. An entry with an empty or duplicate name, a negative length or an unknown kind SHALL be reported in the validation report and dropped, while the rest of the yard still loads.

#### Scenario: AGS Tuscaloosa list
- **WHEN** the AGS profile is active
- **THEN** yard TUS lists TY01 at 1,379 ft, TY02 at 877 ft, TY04 at 334 ft, TY05 at 376 ft, TY06 at 543 ft, TY07 at 500 ft, TY08 at 710 ft, TM01 at 1,081 ft, TS01 at 1,044 ft, TAGS at 1,500 ft, Pass at 752 ft and Chevron at 167 ft

#### Scenario: Receiving yard left out
- **WHEN** the AGS profile is active
- **THEN** yard NOR lists no `R01` to `R12` and no receiving thoroughfare, because the Run8 import names those tracks

#### Scenario: Duplicate name in a yard
- **WHEN** yard NOR lists `Shop 1` twice
- **THEN** the validation report flags the second entry, and the first one still loads

#### Scenario: AGS Norris departure tracks
- **WHEN** the AGS profile is active
- **THEN** yard NOR marks 30 departure tracks: EF01 to EF07, Southern Outbound, WF01 to WF07, CAB 01, CAB 02 and L02 to L14, and no other yard marks any

#### Scenario: Departure flag on a siding
- **WHEN** a yard lists a siding with the departure flag set
- **THEN** the validation report flags the entry, and the siding loads without the flag
