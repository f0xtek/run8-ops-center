# Proposal

## Why

Running freight operations on Run8's AGS South Phase 1 route means tracking by hand which cars sit in which yard, which industries are ready for service, and when an outbound road train has enough cars to run. Run8 already records all of that in its world save. The existing switchlist tracker (`examples/switchlist-tracker.html`) is entirely manual: the user types in arrivals and ticks off every move. A Depot+-style operations centre that reads the world save would generate work and track its completion without manual car bookkeeping.

## What Changes

- New Go application (`run8-ops-center`), shipped as a single Windows binary that runs on the same PC as Run8.
- Follows one **named world save**, given on the CLI as `--save <name>` (required), because the trains folder can hold several world saves.
  - The named save is read from the Run8 installation's trains folder: `C:\Run8Studios\Run8 Train Simulator V3\Content\V3Routes\Regions\GulfCoast\Trains` for AGS.
  - The app also follows that world's auto-saves, `Auto Save World YYYY.mm.dd HH.MM.SS`, in `...\GulfCoast\AutoSaves`.
  - Both folders can be overridden on the CLI.
  - Each time either kind of save is written, the app re-parses it into a snapshot of trains, cuts and cars: unit number, car model, load weight, destination tag and track position.
  - `references/WorldSave.xml` in this project is a design reference and test fixture only. The app never reads it at runtime.
- Interprets every car against a **route profile**. The bundled default profile is AGS South Phase 1, built from `references/IndustryConfiguration.csv` and `references/yard_and_train_limits.txt`, plus yard, job and car-type definitions. Other routes can be added as further profiles without code changes.
- Adds a **learn mode** for mapping Run8 track sections to named yard tracks and industry tracks. The save only stores numeric section indices, so this mapping is needed to know where each car physically is.
- Generates two kinds of order:
  - **Local job orders**, grouped by serving local (e.g. AB45).
    - **Industry lines**: each lists cars to set out from the job's home yard and outbound cars to pull. A line is generated only when no car at the industry is still tagged for that industry, and set-outs never exceed the industry's per-car-type capacity.
    - **Yard lines**: some locals drop off another local's block at a satellite yard and bring back its pulls. A55 serves Tuscaloosa for A37 and A56 and the AGR traffic, A44 serves McCalla for A39, AB11 takes BNSF interchange to 32nd St, and AB45 takes BRL and WWL traffic to Bessemer. These are lines within the local's own order, as the AGS guide describes them, not separate jobs.
  - **Road train orders**, generated when the cars waiting for a road train reach a configurable minimum percentage of its maximum length, capped at its maximum length and tonnage.
    - **En-route stops**: some road trains work a yard or industry on the way. 131 and 339 set out BRI autoracks at McCalla and pull empty racks tagged NOL and MER, and A28 works PHS.
    - **Through trains**: these have no origin yard and only collect cars from their stops. 346 collects ATL racks; 28J and 29K collect containers.
- **Out of scope:** standalone yard-to-yard transfer orders. The AGS guide describes no transfer job that isn't a local or a road train. That order type is deferred to a later change for a route that needs it, such as the SoCal routes (Barstow, Needles, Mojave, Cajon), and will be designed against that route's operations.
- Order lifecycle: Available → In Progress → Completion Detected → Completed. Completion is detected from later saves. The user confirms it or chooses to keep working, and can also cancel an order.
- Local web UI served by the binary on localhost. It updates live when a new save is ingested. Its views (dashboard, orders/switchlists, yards & industries, learn mode, setup and activity log) follow the look of the example tracker.
- Data is stored in flat files: route profile files, a track map, and an operations state file holding orders, history and the activity log.

## Capabilities

### New Capabilities
- `save-ingestion`: detects new or updated Run8 world saves in a configured folder, parses them into snapshots, and tolerates partial writes and non-world saves.
- `route-profiles`: loads route definitions: industries, capacities, outgoing tags, yards, jobs, road train limits, car-type and length defaults. Includes AGS South Phase 1 as the default profile.
- `track-mapping`: learn-mode labelling of track sections as yard tracks, industry tracks or ignored track, with persistence and suggestions.
- `car-status`: derives each car's type, load state, destination (industry, road train, foreign or unknown) and current location from a snapshot, route profile and track map.
- `order-generation`: produces available local job orders (industry and yard lines) and road train orders (including en-route stops and through trains) from current car status under the readiness, capacity and length/tonnage rules.
- `order-lifecycle`: moves orders through their states and detects completion from successive snapshots.
- `ops-dashboard`: the localhost web interface for viewing status and orders, managing orders, learn mode and settings.

### Modified Capabilities
None. This is a new project with no existing specs.

## Impact

- New Go module in the repository root: `cmd/` plus `internal/` packages and embedded web assets. Requires Go 1.26 to build. The release target is `windows/amd64`.
- External dependency on the Run8 V3 world save XML format (`ScnLoader`) and its `.info` sidecars, observed in `references/WorldSave.xml`. A future Run8 format change would break parsing.
- New runtime files in the user's data directory: route profiles, the track map and the operations state.
- **The app MUST NOT edit Run8 files.** Under no circumstances may it create, modify, rename or delete any file or directory in the Run8 install directory `C:\Run8Studios\Run8 Train Simulator V3`, including its subfolders. Those subfolders include the world save folder under `Content\V3Routes\...\Trains`. The app only reads from this directory. All app data, including profiles, the track map, settings and operations state, must be written outside it.
