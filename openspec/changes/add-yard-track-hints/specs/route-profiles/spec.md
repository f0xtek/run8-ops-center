# Spec Delta

## ADDED Requirements

### Requirement: Yard track lists
A yard SHALL be able to list its tracks. Each entry has a name, an optional length in feet and a kind: yard track, siding or main line. Names SHALL be unique within a yard after whitespace normalisation. An entry with an empty or duplicate name, a negative length or an unknown kind SHALL be reported in the validation report and dropped, while the rest of the yard still loads.

#### Scenario: AGS Tuscaloosa list
- **WHEN** the AGS profile is active
- **THEN** yard TUS lists TY01 at 1,379 ft, TY02 at 877 ft, TY04 at 334 ft, TY05 at 376 ft, TY06 at 543 ft, TY07 at 500 ft, TY08 at 710 ft, TM01 at 1,081 ft, TS01 at 1,044 ft, TAGS at 1,500 ft, Pass at 752 ft and Chevron at 167 ft

#### Scenario: Receiving yard left out
- **WHEN** the AGS profile is active
- **THEN** yard NOR lists no `R01` to `R12` and no receiving thoroughfare, because the Run8 import names those tracks

#### Scenario: Duplicate name in a yard
- **WHEN** yard NOR lists `Shop 1` twice
- **THEN** the validation report flags the second entry, and the first one still loads

## MODIFIED Requirements

### Requirement: Hump and receiving-track mappings
A profile SHALL be able to map a Run8 hump controller name to a yard and a track name prefix. It SHALL be able to map a Run8 special-location name pattern, which identifies a track number and which end of the track, to a yard, a track name prefix and an optional list of track names by number. A mapping that names an unknown yard, or lists a number below 1 or a name twice, SHALL be reported in the validation report.

#### Scenario: AGS hump and receiving yard
- **WHEN** the AGS profile is active
- **THEN** `Norris Yard Hump Controller` maps to yard NOR with prefix `Bowl`, and the `Norris RCV <n> <North|South> end` locations map to yard NOR with names `Receiving East Thoroughfare` for 1, `R12` to `R01` for 2 to 13, and `Receiving West Thoroughfare` for 14

#### Scenario: Unknown yard in a mapping
- **WHEN** a hump mapping names yard `XYZ`, which the profile doesn't define
- **THEN** the validation report flags the mapping, and the import skips that hump
