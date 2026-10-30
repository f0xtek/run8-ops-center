# Spec Delta

## Purpose

Turns Run8 world save files, written manually or by auto-save, into up-to-date snapshots of every train, cut and car on the route, without the user re-importing anything.

## ADDED Requirements

### Requirement: Named world save on the command line
The binary SHALL require the name of the world save to follow, given as `--save <name>`. The name SHALL match a save's file name in the save folder, with or without `.xml`, case-insensitively. The match SHALL be a world save. When the flag is missing, the name matches nothing, or the match is not a world save, the binary SHALL exit before serving the UI with an error listing the world saves found in the folder.

#### Scenario: Save named
- **WHEN** the binary starts with `--save ForRealThisTime` and `ForRealThisTime.xml` is a world save in the save folder
- **THEN** the system ingests that save and shows `ForRealThisTime` as the active world

#### Scenario: Flag missing
- **WHEN** the binary starts without `--save`
- **THEN** it exits with an error listing the world saves in the save folder

#### Scenario: Unknown name
- **WHEN** the binary starts with `--save Nope` and no matching world save exists
- **THEN** it exits with an error naming `Nope` and listing the world saves found

#### Scenario: Name is not a world save
- **WHEN** `--save 29K` names a file whose sidecar declares `SaveType` `Train`
- **THEN** it exits with an error saying `29K` is not a world save

### Requirement: Save and auto-save folders
At runtime the system SHALL read saves only from the Run8 installation. Named world saves come from the route profile's trains folder, and auto-saves from its auto-save folder. The system SHALL let either folder be overridden on the CLI. The system SHALL NOT read saves from the project's `references/` or `testdata/`.

#### Scenario: Default folders
- **WHEN** the binary starts with only `--save ForRealThisTime` on a PC with Run8 in the default location and the AGS profile active
- **THEN** it reads the save from `C:\Run8Studios\Run8 Train Simulator V3\Content\V3Routes\Regions\GulfCoast\Trains`, and watches `C:\Run8Studios\Run8 Train Simulator V3\Content\V3Routes\Regions\GulfCoast\AutoSaves`

#### Scenario: Folder missing
- **WHEN** the save folder does not exist or is unreadable
- **THEN** the binary exits with an error naming the path, and does not substitute any other save file

#### Scenario: Auto-save folder missing
- **WHEN** the auto-save folder does not exist
- **THEN** the system keeps following the named save and shows a warning naming the missing auto-save folder

### Requirement: Auto-save following
The system SHALL also ingest world auto-saves from the auto-save folder. These are files named `Auto Save World YYYY.mm.dd HH.MM.SS`, with an `.xml` extension. Files in that folder with other names SHALL be ignored. An auto-save SHALL be accepted only if it is newer than the last ingested save and belongs to the selected world. Auto-saves that fail the world check SHALL be ignored, with a warning.

#### Scenario: Auto-save during the session
- **WHEN** Run8 writes `Auto Save World 2026.10.30 19.15.00.xml` and it belongs to the selected world
- **THEN** the system ingests it as the latest snapshot

#### Scenario: Non-matching file in the auto-save folder
- **WHEN** a file named `Auto Save Train 2026.10.30 19.15.00.xml` appears in the auto-save folder
- **THEN** the system ignores it

#### Scenario: Auto-save from another world
- **WHEN** an auto-save appears that does not belong to the selected world
- **THEN** the system ignores it, keeps the current snapshot, and shows a warning naming the file

#### Scenario: Startup after auto-saves
- **WHEN** the app starts and a matching auto-save is newer than the named world save
- **THEN** the system ingests the auto-save instead of the older named save

#### Scenario: Older auto-save ignored at startup
- **WHEN** the app starts with `--save WorldSave`, and the only auto-save is `Auto Save World 2026.10.28 17.28.02`, written at 17:28:02 before the named save's 17:37:39
- **THEN** the system ingests the named save and skips the older auto-save

#### Scenario: Other saves ignored
- **WHEN** a Train or ExtraBoard save, or a different named world save, is written to the save folder
- **THEN** the system does not ingest it, and the current snapshot is unchanged

### Requirement: Auto-save world match
An auto-save's sidecar names only the auto-save itself, not the world it came from. An auto-save SHALL therefore count as belonging to the selected world when at least 90% of its cars, by model file name and unit number, are present in the last accepted snapshot. At startup that snapshot is the named world save.

#### Scenario: Normal session churn
- **WHEN** an auto-save holds 458 cars and 440 of them were in the last accepted snapshot
- **THEN** it matches (96%) and is ingested

#### Scenario: Different world
- **WHEN** an auto-save holds 300 cars and only 40 of them were in the last accepted snapshot
- **THEN** it does not match, and it is ignored with a warning

#### Scenario: Real auto-save of the same world
- **WHEN** the reference auto-save is checked against the reference world save
- **THEN** all 498 of its vehicles match (100%), and it is accepted

### Requirement: Automatic reload on write
The system SHALL ingest the named world save whenever it is rewritten, and each accepted auto-save as it appears, without user action. It SHALL wait until a file has stopped changing before parsing it.

#### Scenario: Save rewritten during a session
- **WHEN** Run8 rewrites the named world save
- **THEN** within 10 seconds of the write finishing, the system produces a new snapshot and the UI reflects it

#### Scenario: File still being written
- **WHEN** a save file's size or modification time is still changing
- **THEN** the system does not parse it until it has been stable for the settling interval

### Requirement: Snapshot contents
Each snapshot SHALL contain the save's file name, write time and in-game date. It SHALL contain every train, giving its run-time ID, AI flag and ordered vehicle list. For each vehicle it SHALL contain the model file name, unit type, unit number, load weight in US tons, raw destination tag and, for each truck, the route prefix, track section index and distance.

#### Scenario: Reference save parsed (test fixture)
- **WHEN** the parser is given a test fixture copy of `references/WorldSave.xml`
- **THEN** the snapshot contains 31 trains and 498 vehicles, 40 of them locomotives and 458 freight cars

#### Scenario: Vehicle fields captured
- **WHEN** a vehicle has destination tag `A39 GES`, unit number `194887` and track sections `4535` and `4537`
- **THEN** the snapshot vehicle exposes exactly those values

### Requirement: Cut and train distinction
The system SHALL classify a snapshot train containing no locomotive as a cut of cars. It SHALL classify a train with at least one locomotive as a powered train, and SHALL take the powered train's symbol from the lead locomotive's destination tag when that tag is not `None`.

#### Scenario: Uncoupled cars
- **WHEN** a snapshot train contains only freight cars
- **THEN** it is reported as a cut with no symbol

#### Scenario: Symbol from lead locomotive
- **WHEN** a powered train's first locomotive has destination tag `AB45`
- **THEN** the train is reported with symbol `AB45`

### Requirement: Failed parse isolation
If a world save cannot be parsed, the system SHALL keep the previous snapshot, show an ingestion warning naming the file and reason, and retry on the next change to the file.

#### Scenario: Truncated XML
- **WHEN** the active save contains truncated XML
- **THEN** the previous snapshot remains in use, a warning is shown, and orders are not regenerated from the bad file

### Requirement: Snapshot change log
On each successful ingest after the first, the system SHALL record in the activity log the cars that appeared, disappeared, changed location or changed destination tag since the previous snapshot. Tags SHALL be compared after whitespace normalisation, so spacing differences between saves are not reported as retags.

#### Scenario: Car retagged by Run8
- **WHEN** a car's destination tag changes from `AB45 HTI` to `NOL1` between two saves
- **THEN** the activity log contains an entry for that car showing the old and new tags

#### Scenario: Spacing-only tag difference
- **WHEN** a car is tagged `AB10 BRR` in the reference auto-save and `AB10  BRR` in the later world save
- **THEN** no retag entry is recorded for that car

### Requirement: Read-only access to Run8 files
The system SHALL NOT create, modify or delete any file in the Run8 installation or save folder.

#### Scenario: Ingest leaves files untouched
- **WHEN** the system ingests a world save
- **THEN** the save file and its sidecar are unchanged on disk

### Requirement: Data and profile paths outside the Run8 install directory
The system SHALL refuse any data directory or route profile path that resolves inside a protected Run8 directory. Protected directories are `C:\Run8Studios\Run8 Train Simulator V3` and any configured Run8 install folder. Paths SHALL be compared after making them absolute, cleaning them and resolving symlinks, case-insensitively on Windows, and on whole path components.

#### Scenario: Data flag inside the install directory
- **WHEN** the binary is started with `--data "C:\Run8Studios\Run8 Train Simulator V3\OpsData"`
- **THEN** it exits before writing anything, with an error naming the path and the protected directory

#### Scenario: Profile path inside the install directory
- **WHEN** the user sets or imports a route profile path under `C:\Run8Studios\Run8 Train Simulator V3\Content`
- **THEN** the setting is rejected with an error, and the previous profile path stays in effect

#### Scenario: Case and relative path variants
- **WHEN** the data path is given as `c:\run8studios\RUN8 TRAIN SIMULATOR V3\x`, or as a relative path or symlink that resolves inside the install directory
- **THEN** it is refused in the same way

#### Scenario: Similar sibling name allowed
- **WHEN** the data path is `C:\Run8Studios\Run8 Train Simulator V3 Ops`
- **THEN** it is accepted, because it is not inside the protected directory

#### Scenario: Install folder configured after the data directory
- **WHEN** the user sets the Run8 install folder to a directory that contains the current data directory or profile path
- **THEN** the setting is rejected, and the error explains the conflict

### Requirement: Write guard
Before every file write, rename or delete, the system SHALL check the target path against the protected Run8 directories. It SHALL abort the operation with an error if the target is inside one, even when the path came from a previously accepted setting.

#### Scenario: Guard blocks a stray write
- **WHEN** any component attempts to write a file whose resolved path is inside `C:\Run8Studios\Run8 Train Simulator V3`
- **THEN** the write does not happen and an error is logged naming the path
