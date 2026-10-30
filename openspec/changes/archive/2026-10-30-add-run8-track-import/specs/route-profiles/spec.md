# Spec Delta

## ADDED Requirements

### Requirement: Run8 route data folders
A profile SHALL be able to name its Run8 region folder and its route folders, each route folder with the track section prefix it covers. Each name SHALL be a single folder name. A name that is empty, absolute, or contains a path separator or `..` SHALL be rejected in the validation report and not used.

#### Scenario: AGS folders
- **WHEN** the AGS profile is active
- **THEN** its Run8 region folder is `GulfCoast`, and its one route folder is `NS_AGS_Phase01` with prefix 280

#### Scenario: Escaping folder name
- **WHEN** a profile names its region folder `..\..\Windows`
- **THEN** the validation report flags it, and no route data is read for that profile

### Requirement: Hump and receiving-track mappings
A profile SHALL be able to map a Run8 hump controller name to a yard and a track name prefix. It SHALL be able to map a Run8 special-location name pattern, which identifies a track number and which end of the track, to a yard and a track name prefix. A mapping that names an unknown yard SHALL be reported in the validation report.

#### Scenario: AGS hump and receiving yard
- **WHEN** the AGS profile is active
- **THEN** `Norris Yard Hump Controller` maps to yard NOR with prefix `Bowl`, and the `Norris RCV <n> <North|South> end` locations map to yard NOR with prefix `Receiving`

#### Scenario: Unknown yard in a mapping
- **WHEN** a hump mapping names yard `XYZ`, which the profile doesn't define
- **THEN** the validation report flags the mapping, and the import skips that hump

### Requirement: Yard anchors
A yard SHALL be able to list anchors. An anchor is either a Run8 milepost station name, matched exactly after whitespace normalisation, or an industry tag of the profile. Anchors that match no milepost or industry SHALL be reported in the validation report once route data has been read.

#### Scenario: AGS anchors
- **WHEN** the AGS profile is active
- **THEN** NOR is anchored by `Receiving Yard`, `West Forwarding Yard` and `East Forwarding Yard`, 32ND by `32nd St Yard`, TUS by `Tuscaloosa MP 199.1`, MCC by industry `BRI`, and BES by industry `BRL`

#### Scenario: Misspelled anchor
- **WHEN** yard TUS lists the anchor `Tuscalosa MP 199.1` and route data has been read
- **THEN** the validation report flags the anchor as matching no milepost

### Requirement: Bundled profile upgrade
A profile SHALL declare a schema version; a profile without one is version 1. At startup, when the bundled profile's version is newer than the installed copy's, the system SHALL replace the installed profile definition, industry CSV and limits file with the bundled ones, after saving each old file as a backup. It SHALL leave the track map and every other file in the profile folder untouched.

#### Scenario: Older installed profile
- **WHEN** the installed AGS profile has no schema version and the bundled one is version 2
- **THEN** the three bundled files replace the installed ones, the old files are kept as backups next to them, and `trackmap.json` is unchanged

#### Scenario: Current installed profile
- **WHEN** the installed AGS profile has the same schema version as the bundled one and contains user edits
- **THEN** no file in the profile folder changes

#### Scenario: Newer installed profile
- **WHEN** the installed AGS profile's schema version is newer than the bundled one
- **THEN** no file in the profile folder changes

#### Scenario: Unreadable installed profile
- **WHEN** the installed `profile.yaml` cannot be parsed
- **THEN** no file in the profile folder changes, and the profile load reports the parse error
