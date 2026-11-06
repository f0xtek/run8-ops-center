# ops-dashboard Specification

## Purpose
The local web interface the user keeps open next to Run8. It shows the state of the railroad, lets the user pick and manage orders, run learn mode and change settings, and updates live as new saves arrive.

## Requirements

### Requirement: Local web server
On start, the binary SHALL serve the interface over HTTP on a configurable port, bound to `127.0.0.1` by default. An explicit setting SHALL allow binding to all interfaces so other devices on the network can use it. On start, the binary SHALL print the URL and open it in the default browser, unless that is disabled.

#### Scenario: Default start
- **WHEN** the user runs the binary with no arguments
- **THEN** the interface is reachable at `http://127.0.0.1:<port>/` from that PC and nowhere else, and the browser opens it

#### Scenario: Port in use
- **WHEN** the configured port is already in use
- **THEN** the binary exits with an error naming the port and the setting to change

### Requirement: Live updates
Open pages SHALL update without a manual reload when a new snapshot is ingested or an order changes state. The interface SHALL show the active save's name, the in-game date and the time of the last ingest.

#### Scenario: Save written while viewing
- **WHEN** Run8 writes a new world save while the dashboard is open
- **THEN** the dashboard's counts, orders and last-ingest time update by themselves

### Requirement: Dashboard view
The dashboard SHALL show:
- summary tiles: cars on route, at industries, ready to pull, awaiting delivery, awaiting road trains, and unmapped or unknown cars;
- In Progress and Completion Detected orders;
- Available orders grouped by kind;
- each road train's build percentage toward its threshold;
- an attention list of ingest warnings, profile validation problems, unknown-tag or unmapped cars, unusable Run8 route files, and the Run8 track data notice.

#### Scenario: Attention items
- **WHEN** 12 cars are on unmapped sections and the profile reports an orphan tag
- **THEN** the attention list shows both, with links to learn mode and to the profile report

#### Scenario: Track data changed
- **WHEN** the Run8 track data fingerprint differs from the one recorded at the last import
- **THEN** the attention list says the Run8 track data changed, with a link to the import preview

### Requirement: Order detail and actions
Opening an order SHALL show it as a switchlist. Local job orders show a set-out and pick-up table for each industry line and yard line. Road train orders show the departing consist, then each stop's set-outs and pick-ups in order. Every car SHALL show its current location and progress. The view SHALL offer only the actions valid for the order's state: start, start line, confirm, continue, complete or cancel.

#### Scenario: Actions by state
- **WHEN** the user opens a Completion Detected order
- **THEN** only confirm and continue are offered

### Requirement: Yards and industries view
The interface SHALL list every yard with its tracks and the cars on each. For a yard with a track list, it SHALL also show every listed track: labelled ones with the listed length next to the measured length, and unlabelled ones marked as not labelled yet. It SHALL list every industry, grouped by serving job, with its cars, service states, and occupancy against capacity for each car type.

#### Scenario: Industry occupancy
- **WHEN** HTI has 10 Two Bay Covered Hoppers and a capacity of 24
- **THEN** the industry shows 10/24 for that car type

#### Scenario: Listed track not labelled yet
- **WHEN** yard TUS lists TY06 and no section is labelled `TUS / TY06`
- **THEN** the yards view shows TY06 under Tuscaloosa as not labelled yet

### Requirement: Learn mode view
The interface SHALL provide the learn mode described by track mapping: a list of unmapped cuts with suggestions, label entry, a list of existing labels with edit and remove actions, and the import from Run8 with its preview. The preview SHALL offer confirm and cancel. Each label in the list SHALL show whether it came from an import or was set by hand.

#### Scenario: Label from the UI
- **WHEN** the user accepts a suggested label for an unmapped cut
- **THEN** the cut disappears from the unmapped list, and the dashboard counts update

#### Scenario: Import from the UI
- **WHEN** the user starts the import, reviews the preview and confirms it
- **THEN** the imported labels appear in the label list marked as imported, cuts on those sections leave the unmapped list, and the dashboard counts update

### Requirement: Save source display
The interface SHALL show the active world save name, and the save and auto-save folders. These are set on the CLI and SHALL be read-only in the UI.

#### Scenario: Save source shown read-only
- **WHEN** the app was started with `--save ForRealThisTime`
- **THEN** the setup view shows `ForRealThisTime` and both folders, with no control to change them

### Requirement: Setup view
The interface SHALL let the user set:
- the optional Run8 install folder;
- the active route profile;
- per-train minimum length percentage and maximum car age;
- the server port and bind option.

It SHALL also show the profile validation report. Changes SHALL be saved and applied without a restart, except for port and bind changes, which SHALL say that a restart is needed.

#### Scenario: Threshold change applies
- **WHEN** the user lowers 108's minimum from 50% to 30%
- **THEN** available orders are recomputed immediately

### Requirement: Activity log view
The interface SHALL show recent activity log entries, newest first, covering snapshot changes and order state changes.

#### Scenario: Order started entry
- **WHEN** the user starts the AB10 order
- **THEN** the activity log shows an entry for it with the time

### Requirement: Departure track marks
The yards view SHALL show, for each yard, whether orders take cars only from its departure tracks or from every yard track, and SHALL let the user mark or unmark as a departure track any labelled yard track and any listed yard-kind track. A track whose state differs from the profile SHALL be shown as changed. Marks SHALL be saved in the user's settings, take effect on orders at once, and survive restarts and profile upgrades.

#### Scenario: Receiving track marked for a busy yard
- **WHEN** the user marks `NOR / R07` as a departure track
- **THEN** R07 shows as a departure track changed from the profile, and the available orders and road-train build percentages count its cars without a restart

#### Scenario: Forwarding track unmarked
- **WHEN** the user unmarks `NOR / WF01`
- **THEN** the cars on WF01 leave the available orders, and WF01 shows as changed from the profile

#### Scenario: Marks survive a profile upgrade
- **WHEN** the user has marked `NOR / R07` and the bundled profile is upgraded to a newer schema version
- **THEN** R07 is still a departure track after the upgrade

#### Scenario: Every departure track unmarked
- **WHEN** the user unmarks every Norris departure track
- **THEN** the yards view says Norris orders take cars from every yard track, and cars on any NOR track count

#### Scenario: Yard rule shown
- **WHEN** Norris has 30 departure tracks and Tuscaloosa has none
- **THEN** the yards view says Norris orders take cars from its 30 departure tracks only, and Tuscaloosa orders from every yard track
