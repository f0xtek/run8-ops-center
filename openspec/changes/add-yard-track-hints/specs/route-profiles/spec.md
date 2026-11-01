# Spec Delta

## ADDED Requirements

### Requirement: Yard track lists
A yard SHALL be able to list its tracks. Each entry has a name, an optional length in feet and a kind: yard track, siding or main line. Names SHALL be unique within a yard after whitespace normalisation. An entry with an empty or duplicate name, a negative length or an unknown kind SHALL be reported in the validation report and dropped, while the rest of the yard still loads.

#### Scenario: AGS Tuscaloosa list
- **WHEN** the AGS profile is active
- **THEN** yard TUS lists TY01 at 1,379 ft, TY02 at 877 ft, TY04 at 334 ft, TY05 at 376 ft, TY06 at 543 ft, TY07 at 500 ft, TY08 at 710 ft, TM01 at 1,081 ft, TS01 at 1,044 ft, TAGS at 1,500 ft, Pass at 752 ft and Chevron at 167 ft

#### Scenario: Receiving yard left out
- **WHEN** the AGS profile is active
- **THEN** yard NOR lists no `R01` to `R12` and no receiving thoroughfare, because the Run8 import names those tracks `Receiving 1` to `Receiving 14`

#### Scenario: Duplicate name in a yard
- **WHEN** yard NOR lists `Shop 1` twice
- **THEN** the validation report flags the second entry, and the first one still loads
