# run8-route-data Specification

## Purpose
Reads Run8's static route description files from the Run8 install folder, so the app can learn track names and layout that world saves don't contain. It reads them only and never changes the install.

## Requirements

### Requirement: Route data files and location
The system SHALL read route data from the Run8 install folder: the configured install folder, or `C:\Run8Studios\Run8 Train Simulator V3` when none is set. It SHALL read `Config.ind` from `Content\V3Routes\Regions\<region>\`, and `TrackDatabase.r8`, `HumpControllerList.r8`, `AISpecialLocations.r8` and `MilepostDatabase.r8` from `Content\V3Routes\<route>\` for each route folder. The region and route folders come from the active profile.

#### Scenario: AGS file locations
- **WHEN** the AGS profile is active and no install folder is configured
- **THEN** the system reads `C:\Run8Studios\Run8 Train Simulator V3\Content\V3Routes\Regions\GulfCoast\Config.ind` and the four `.r8` files from `C:\Run8Studios\Run8 Train Simulator V3\Content\V3Routes\NS_AGS_Phase01\`

#### Scenario: Profile without Run8 folders
- **WHEN** the active profile names no Run8 region or route folder
- **THEN** no route data is read, and the import reports that the profile needs its Run8 folders set

### Requirement: Read-only access to route data
The system SHALL open route data files for reading only. It SHALL NOT create, modify, rename or delete any file or folder in the Run8 install while reading route data, and SHALL NOT store copies of the files outside the install.

#### Scenario: Install unchanged after reading
- **WHEN** the system reads all five route data files
- **THEN** every file and folder in the Run8 install has the same contents, size and modification time as before

### Requirement: Whole-file decoding
A route data file SHALL be accepted only when it decodes completely: every record parses and the last record ends exactly at the end of the file. A file that fails these checks SHALL be rejected as a whole, so a truncated or changed format never yields partial data.

#### Scenario: AGS files decode
- **WHEN** the AGS route data files are read
- **THEN** `Config.ind` yields 46 industries, `TrackDatabase.r8` 10,630 track sections, `HumpControllerList.r8` the `Norris Yard Hump Controller` with 56 tracks, `AISpecialLocations.r8` 50 locations and `MilepostDatabase.r8` 319 mileposts

#### Scenario: Truncated file
- **WHEN** `TrackDatabase.r8` ends partway through a section record
- **THEN** that file is rejected with a reason, and no sections from it are used

### Requirement: Independent file failures
A route data file that is missing, unreadable or rejected SHALL be reported with its path and the reason. The other files SHALL still be used, and only the features that need the failed file SHALL be unavailable.

#### Scenario: Track database missing
- **WHEN** `TrackDatabase.r8` is missing but `Config.ind` decodes
- **THEN** industry labels can still be imported, bowl and receiving tracks and the topology features are reported unavailable, and the missing file's path is shown

### Requirement: Route data fingerprint
The system SHALL record a fingerprint of each route data file it reads, made of its path, size and modification time. Two readings SHALL count as the same route data only when every file's fingerprint matches.

#### Scenario: Industry file swapped
- **WHEN** the user replaces `Config.ind` with a different file after an import
- **THEN** the fingerprint no longer matches the one recorded at the import

### Requirement: When route data is read
The system SHALL read route data at startup, when the active profile changes, when the install folder setting changes, and again just before building an import preview, so the preview reflects the files as they are at that moment.

#### Scenario: Install folder set
- **WHEN** the user sets the Run8 install folder in the setup view
- **THEN** route data is read from the new folder without a restart
