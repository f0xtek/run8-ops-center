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
- an attention list of ingest warnings, profile validation problems, and unknown-tag or unmapped cars.

#### Scenario: Attention items
- **WHEN** 12 cars are on unmapped sections and the profile reports an orphan tag
- **THEN** the attention list shows both, with links to learn mode and to the profile report

### Requirement: Order detail and actions
Opening an order SHALL show it as a switchlist. Local job orders show a set-out and pick-up table for each industry line and yard line. Road train orders show the departing consist, then each stop's set-outs and pick-ups in order. Every car SHALL show its current location and progress. The view SHALL offer only the actions valid for the order's state: start, start line, confirm, continue, complete or cancel.

#### Scenario: Actions by state
- **WHEN** the user opens a Completion Detected order
- **THEN** only confirm and continue are offered

### Requirement: Yards and industries view
The interface SHALL list every yard with its tracks and the cars on each. It SHALL list every industry, grouped by serving job, with its cars, service states, and occupancy against capacity for each car type.

#### Scenario: Industry occupancy
- **WHEN** HTI has 10 Two Bay Covered Hoppers and a capacity of 24
- **THEN** the industry shows 10/24 for that car type

### Requirement: Learn mode view
The interface SHALL provide the learn mode described by track mapping: a list of unmapped cuts with suggestions, label entry, and a list of existing labels with edit and remove actions.

#### Scenario: Label from the UI
- **WHEN** the user accepts a suggested label for an unmapped cut
- **THEN** the cut disappears from the unmapped list, and the dashboard counts update

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
