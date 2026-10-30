# Design

## Context

The project starts from nothing: there's no code yet. The inputs we have are:
- `notes.txt` (the requirements);
- `examples/switchlist-tracker.html` (the target look and feel);
- `references/`: a real world save (`WorldSave.xml`), the industry CSV, the train limits file, routing PDFs, and Run8 train, location and unit-database saves.

Format facts the design depends on, observed in `references/WorldSave.xml` and the reference auto-save:

- **Root and contents.** The root is `ScnLoader`. It contains `trainList/TrainLoader`. Each train holds `trainID`, `TrainWasAI` and `unitLoaderList/RailVehicleStateClass`. Further down are `date`, `saveGameType`, `signals`, `switches`, an optional `description` and `AllIndustryStatus`. World saves have `description` (`World Save-As`); auto-saves do not.
- **Vehicle fields.** Each vehicle has exactly these fields:
  - `rvXMLfilename` and `unitType` (`US_Freightcar` or `US_DieselEngine`);
  - two-element arrays, one entry per truck: `currentRoutePrefix`, `currentTrackSectionIndex`, `startNodeIndex`, `distanceTravelledInMeters`, `reverseDirection`;
  - `loadWeightUSTons`, `destinationTag`, `unitNumber`, `hazmatPlacardIndex`.

  There is no car type, length, reporting mark, speed, brake state or track name.
- **Train symbol.** There is no symbol field. The symbol appears only as the lead locomotive's `destinationTag`.
- **Industry status.** `AllIndustryStatus/IndustryStatus` (`IndustryName`, `UnprocessedCars`, `EscrowCars`) is empty for all 54 industries in both reference saves, so the design does not rely on it.
- **Sidecar.** The `.info` sidecar (`ScnInfo`) gives `SaveType` (`World` | `Train` | `ExtraBoard`) and the original path: `C:\Run8Studios\Run8 Train Simulator V3\Content\V3Routes\Regions\GulfCoast\Trains\`.
- **Tag forms seen.** `<LOCAL> <IND>` (e.g. `AB45 HTI`; `AB10  BRR` has a double space), bare outgoing tags (`CHA`, `NOL1`), foreign `<CODE> .`, yard-internal (`NOR Hump`, `NOR Trim W`) and `None`.
- **Auto-save versus world save.** Compared `references/Auto Save World 2026.10.28 17.28.02.xml` (in-game `date` 17:12:47, written 17:28:02) with the later `WorldSave.xml` (in-game `date` 17:22:23, written 17:37:39):
  - **Same railroad.** The schema is identical, and both saves hold the same 31 `trainID`s and the same 498 vehicle keys, so car overlap is 100%. Car keys and `trainID` are stable across saves within a session.
  - **What moved.** 300 vehicles moved, all in the four powered trains (130-28, 384-28, 153-28, Q-BIRFAR-28). No uncoupled cut moved, and no load weight changed.
  - **AI flag can flip.** 153-28's `TrainWasAI` changed from `true` to `false` when the player took control.
  - **Spacing changes.** Four BRR cars are `AB10 BRR` in the auto-save but `AB10  BRR` in the world save. Tags must be compared after normalisation.
  - **Ordering.** The in-game `date` and the sidecar `FileTimestamp` put the two saves in the same order.

User decisions from clarification:
- The app runs on the same Windows PC as Run8.
- Track locations come from in-app learn mode.
- Orders are local jobs with industry lines. Satellite-yard hand-offs (A55→Tuscaloosa, A44→McCalla, AB11→32nd St, AB45→Bessemer) are yard lines inside those locals.
- There are no standalone transfer orders in this change. Road train orders support en-route stops and through trains.
- Road trains trigger at a minimum percentage of maximum length.
- The interface is a local web UI.

## Goals / Non-Goals

**Goals:**
- One self-contained `run8ops.exe`. It needs no installer, runtime, database server or internet access.
- Ingest-to-UI latency under 10 s after Run8 finishes writing a save. Parse time for an 800 KB save well under 1 s.
- All route-specific knowledge lives in profile files. Adding a route means adding a profile, not changing code.
- Core logic (parse → status → orders → lifecycle) is pure and deterministic, so it can be tested with synthetic snapshots on any OS.

**Non-Goals:**
- Writing to Run8 files, controlling Run8, or reading Run8 memory or network state.
- Simulating industry processing time. Run8 retags cars itself, and the app only observes the tags.
- Generating orders for inbound road trains or Amtrak. Arrivals are observed, not planned.
- Multi-user access, authentication, or remote hosting.
- Print or PDF switchlists. These can be added later, since the views are HTML.

## Decisions

### D1. Go stdlib-first, two dependencies
- Use `net/http`, `html/template`, `encoding/xml`, `encoding/csv`, `encoding/json` and `embed`.
- Add exactly two dependencies:
  - `github.com/fsnotify/fsnotify` for change notifications;
  - `gopkg.in/yaml.v3`, because the profile is edited by hand.
- *Alternatives:* a web framework or htmx/React were rejected. The UI is a handful of pages, and a JS build step would add tooling for no gain.

### D2. Flat-file storage, not SQLite
| Data | Format and location |
|---|---|
| Profile | `profiles/<id>/profile.yaml`, `IndustryConfiguration.csv` (verbatim Run8 format), `train_limits.tsv` (verbatim `yard_and_train_limits.txt` format; parsing stops at the first blank line), `trackmap.json` |
| Settings | `settings.json` |
| Operations state | `state.json`: non-available orders, history, per-car dwell records, and the activity log capped at 2,000 entries |

- All writes are atomic: write a temporary file, then rename it.
- The data directory is `os.UserConfigDir()/Run8OpsCenter` (`%APPDATA%\Run8OpsCenter`), or the `--data <dir>` flag.
- **Protected-path guard.** All writes go through `internal/store`, which owns a single `fsguard` check. The protected set is `C:\Run8Studios\Run8 Train Simulator V3` plus the configured Run8 install folder.
  - A path is normalised with `filepath.Abs`, then `filepath.Clean`, then `filepath.EvalSymlinks`. When the target does not exist yet, symlinks are resolved on the nearest existing ancestor.
  - The normalised path is compared component by component, case-insensitively on Windows.
  - The check runs at three points: on `--data` and profile-path validation at startup and in setup, on install-folder changes, and again before every write, rename or remove.
- *Alternative:* SQLite through `modernc.org/sqlite`. It was rejected because volumes are tiny (hundreds of cars, tens of orders per session). JSON files stay inspectable and editable by hand, and need no schema migrations.

### D3. Package layout
```
cmd/run8ops/            main: flags, data dir bootstrap, wiring, browser open
internal/run8save/      ScnLoader/ScnInfo decoding → Snapshot (no route knowledge)
internal/watch/         folder watcher: fsnotify + 5 s poll fallback, settle detection, newest-world-save selection
internal/profile/       profile.yaml + CSV + limits loading, normalisation, validation report; embedded AGS default
internal/trackmap/      section labels, lookup, learn-mode candidates and suggestions
internal/carstatus/     Snapshot + Profile + TrackMap + dwell → []CarStatus
internal/orders/        generation (local industry/yard lines, road trains with stops) and lifecycle state machine
internal/store/         atomic JSON persistence for settings/state/trackmap
internal/engine/        single owner of current state; runs the ingest pipeline; publishes change events
internal/web/           HTTP handlers, templates, SSE, embedded static assets
```

### D4. Single-writer engine with immutable views
- One engine goroutine owns all mutable state.
- It receives three kinds of input over channels:
  - watcher events (new snapshot);
  - UI commands (start, confirm, label and so on);
  - profile or settings changes.
- Each input triggers this pipeline: `carstatus.Derive` → `orders.Reconcile` (lifecycle detection on In Progress orders) → `orders.Generate` (available orders) → persist → publish an immutable `View`.
- HTTP handlers only read the latest `View`, through an atomic pointer, and send commands.
- *Why:* no lock juggling between the watcher, the web layer and persistence, and the pipeline order is deterministic.

### D5. Save detection
- **CLI.**
  - `--save <name>` is required. It is matched case-insensitively against `<save-dir>/*.xml` base names, with or without `.xml`, and must be a world save (D6).
  - `--save-dir` defaults to the profile's `defaultSaveDir`. For AGS that is `C:\Run8Studios\Run8 Train Simulator V3\Content\V3Routes\Regions\GulfCoast\Trains`.
  - `--autosave-dir` defaults to the profile's `defaultAutoSaveDir`. For AGS that is `...\GulfCoast\AutoSaves`.
  - On a missing or unknown `--save`, startup exits non-zero and lists the world saves in the folder, found by scanning their `.info` sidecars.
  - Nothing in the binary refers to the project's `references/` or `testdata/`. No sample save is embedded.
- **What is watched.** Two inputs, both through fsnotify plus a 5 s poll fallback (fsnotify can miss events on some Windows filesystems):
  - the named save file and its `.info`;
  - the auto-save folder, filtered by the regexp `^Auto Save World \d{4}\.\d{2}\.\d{2} \d{2}\.\d{2}\.\d{2}\.xml$`.

  Other files in either folder are ignored.
- **Settle check.** A file is ingested when its size and modification time are unchanged across two checks 1 s apart.
- **Newer-than rule.** A candidate is accepted only if its write time is newer than the last ingested save's. Order comes from the `.info` `FileTimestamp` when present, otherwise from the file's modification time. The timestamp in an auto-save's name is a tiebreaker only.
- **World check for auto-saves.** The auto-save folder holds auto-saves from every world, so each candidate is checked before ingest.
  - **The sidecar doesn't help.** In the sample `references/Auto Save World 2026.10.28 17.28.02.info`, `Filename` and `DisplayFilename` point at the auto-save itself, and the only other fields are `SaveType` (`World`) and `FileTimestamp`. Nothing names the source world.
  - **The check is therefore car overlap.** At least 90% of the auto-save's car keys must exist in the last accepted snapshot. At startup, the snapshot used is the named save's.
  - **Cost.** The check needs the parsed snapshot, so a candidate is fully parsed before it is accepted or rejected. An 800 KB save parses well under 1 s (Goals), so this is acceptable.
  - **Failure.** A failed check produces a warning naming the file and the overlap percentage, and the file is skipped. The 90% threshold tolerates normal session churn: cars arriving or leaving off-map between saves.
- **Startup.**
  1. Parse the named save.
  2. Scan the auto-save folder for matching auto-saves that pass the world check and are newer.
  3. Ingest the newest of them, or the named save if none qualifies.

### D6. Parsing
- The parser uses `encoding/xml` streaming (`Decoder.Token` plus `DecodeElement` per `TrainLoader`). Sections the app does not use (signals, switches, flags) are skipped, which keeps memory flat for large saves.
- A file counts as a world save if its sidecar declares `SaveType=World`; auto-save sidecars do too (observed). With no sidecar, the parser falls back to `saveGameType` in the XML. Because `saveGameType` follows `trainList` in the file, the fallback means reading the whole file.
- Any XML error rejects the whole file, so a partial snapshot is never used.
- `description` is optional, because auto-saves omit it.

### D7. Car identity, type, reporting mark and dimensions
- **Key.** A car's key is `rvXMLfilename + "#" + unitNumber`.
- **Type.** Matched against the profile's ordered prefix patterns. The AGS defaults come from filename families in the reference save:

  | Filename prefix | Car type |
  |---|---|
  | `R8_Autorack_` | Autorack Car |
  | `R8_Boxcar_`, `R8_Box_` | Box Car |
  | `R8_Gon_` | Gondola |
  | `R8_Bulkhead_` | Bulkhead_Flat |
  | `R8_Centerbeam_` | Centerbeam Flat Car |
  | `R8_Coil_` | CoilCar |
  | `R8_C14Hopper_` | Two Bay Covered Hopper |
  | `R8_CoveredHopper_` | Covered Hopper |
  | `R8_PlasticPelletHopper_` | PlasticPellet_Hopper |
  | `R8_ChipHopper_` | Woodchip_Hopper |
  | `R8_Hopper_` | Open Hopper |
  | `Run8_Tank`, `R8_T104_` | Tank Car |

  Unmatched models are listed in the profile report so the user can add patterns.
- **Reporting mark.** The last `_`-separated token of the model name, with trailing digits stripped. Example: `…_TTJX01` → `TTJX`.
- **Length and tare.** Taken from the profile's per-type defaults. An optional Run8 install folder enables reading lengths and tares from car definition files (see Open Questions). Until that reader exists and finds a value, the defaults apply.

### D8. Location resolution
- `trackmap.json` maps `"<prefix>/<section>"` to a label: `{kind: yard|industry|ignored, yard, track, industries[]}`.
- A car's location comes from its truck index 0 section, falling back to truck index 1 (car-status spec).
- **Learn mode** groups snapshot trains by their unmapped sections and offers labels:
  - a tag-majority suggestion: every car in the group is tagged for one industry;
  - a neighbour suggestion: an adjacent section index, ±1, carries a label. *[INFERENCE: Run8 section indices are not guaranteed to be spatially adjacent. The suggestion is only a hint, and the user always confirms.]*

### D9. Order model
```
Order{ID, Kind(local|road), JobID, State, Consist[] (road: cars departing origin),
      Lines[]{Kind(industry|yard|stop), Target, SetOuts[], PickUps[]}  // local: industry + yard lines; road: stops in order
      Totals{LengthFt, GrossTons, RequiredHP}, Flags[], CreatedAt, StartedAt, Detected{...}, FinishedAt, GameDates}
```
- **Available IDs.** Deterministic: `avail:<jobID>`, which keeps the UI stable across saves.
- **Started orders.** Get a new ID (`ord-<seq>`) and a frozen car list.
- **Starting one line.** Copies that line, whether an industry line or a yard line, into a new started order. The job's available order is regenerated without those cars, because the reservation rule removes them. Road train orders can only be started whole.
- **Generation order.**
  1. Local industry lines.
  2. Yard lines.
  3. Road trains with an origin.
  4. Through trains.

  Each step sees the cars reserved by started orders. Available orders may overlap; for example, `BRI BRI` cars can appear on both 131 and 339. Starting one removes the shared cars from the other.
- **Yard-line pick-up exclusion.** Needs the set of stop pick-up rules per yard. That set is precomputed from the profile once per load.
- **"Delivered to a yard"** (yard lines, yard stops) means the car is in that yard, or at an industry served by a local homed there. This tolerates the satellite local having already spotted the cars before the next save.
- **Lifecycle.** Implemented as an explicit transition table. The "continue" suppression stores a fingerprint of the order cars' resolved locations. Detection is skipped while the current fingerprint equals the stored one.

### D10. AGS South Phase 1 default profile (data, editable)
Grounded in the AGS guide (page numbers in brackets). There are no transfer jobs: every yard-to-yard move in AGS belongs to a local or a road train.

**Locals**

| Job ID | Symbol | Home | Industries | Yard lines |
|---|---|---|---|---|
| ab10 | AB10 | NOR | FIN, BSI, WWT, BRR, SPS, CMC, WRG [p21] | — |
| ab11 | AB11 | NOR | ONS, SMC, JSI, ERS, DPC, BFB [p22] | 32ND: carries BNSF [p22, p25] |
| ab45 | AB45 | NOR | STI, GFS, HTI, BLX, JPI, BHM [p22] | BES: carries BRL, WWL [p22–23] |
| a44 | A44 | NOR | LPT, SAS, TMN [p17] | MCC: carries A39 [p16–17] |
| a55 | A55 | NOR | KYK, ABS [p19, p25] | TUS: carries A37, A56, AGR [p15, p19, p25] |
| a37 | A37 | TUS | CYC, SIS, DAB, FRP [p15] | — |
| a56 | A56 | TUS | MOD, MSC, TBP, CVA, HBX, MVW, AGR [p20, p25] | — |
| a39 | A39 | MCC | CPS, 84L, COP, PSC, GES [p16] | — |
| bri | BRI | MCC | BRI (BRI switcher) [p18] | — |
| csx-bes | CSXB | BES | BRL, WWL (CSX Bessemer switcher) [p23, p25] | — |

**Road trains**

| Symbol | Origin | Carries | Stops |
|---|---|---|---|
| 131 | NOR | BRI, NOL1 | MCC: set out `BRI`; pick up `NOL`, `MER` Autorack Car [p18, p27] |
| 339 | NOR | MER, NOL3 | MCC: set out `BRI`; pick up `NOL`, `MER` Autorack Car [p18, p27] |
| 346 | — (through) | — | MCC: pick up `ATL` Autorack Car [p18] |
| 28J | — (through) | — | MCC: pick up `ATL`, `JAX` Well Car [p18] |
| 29K | — (through) | — | MCC: pick up `MER` Well Car [p18] |
| A28 | NOR | A28; off-map industry A28 | industry stop PHS [p15] |
| Y130 | NOR | CSX | — (leaves via Ensley Branch) [p25] |
| 345, 355, 108, 16T, 178, 154, 156, 378, 341, 183, 385, A79 | NOR | class codes from `train_limits.tsv` | — |

- **Thresholds.** Road trains with an origin default to `minLengthPct: 50`. Through trains default to `0`, so a single pick-up car makes an order available. `maxCarAgeDays` is unset by default.
- **Yards.** Norris (NOR), Tuscaloosa (TUS), McCalla (MCC), Bessemer (BES) and 32nd Street (32ND).
- **Yard-internal tags:** `NOR`, `NOR Hump`, `NOR Trim W`, `NOR Trim E`.
- **Limits.** Taken from the matching limits row (AB10, AB11, AB45, A44, A55, A28, and the road trains). Jobs with no row (A37, A56, A39, BRI, CSXB, 346, 28J, 29K) have no limit until the user sets one in the profile.
- **Car types.** The D7 patterns need a Well Car pattern for 28J/29K containers. The reference save's filename families will be checked for it during task 3.5.
- **Remaining assumptions.**
  - Bare `NOL` and `MER` autorack tags are claimed by the 131/339 McCalla stops rather than by 355/339 at Norris, following [p27].
  - MB/395/394 are left unconfigured, and the validation report shows them.

### D11. Web UI
- Server-rendered `html/template` pages, one per view, plus a small vanilla JS file.
- The JS subscribes to `/events` (SSE). On a `view-changed` event, it refetches the current page's main fragment (`?fragment=1`) and swaps it in.
- Actions are `POST` forms (`/orders/{id}/start`, `/orders/{id}/lines/{industry}/start`, `/confirm`, `/continue`, `/complete`, `/cancel`, `/trackmap/label`, `/settings`). They return a redirect, or the fragment when called from JS.
- Styling ports the example's CSS variables, light/dark themes, cards, tiles, pills and chips. Fonts use local stacks (`"IBM Plex Sans", system-ui`, `"IBM Plex Mono", ui-monospace`, `"Barlow Condensed", "Arial Narrow"`), because the app must work offline.
- On a port conflict the binary exits with a clear error. The browser opens with `rundll32 url.dll,FileProtocolHandler` on Windows and `open` on macOS (for development). The `--no-browser` flag disables this.

### D12. Build and test
- **Development:** on macOS, where there is no Run8 install. Run with `--save WorldSave --save-dir <tmp>/Trains --autosave-dir <tmp>/AutoSaves`, where the temp folders stand in for the Run8 folders, and drop fixture copies into them. This is the only way the reference save reaches a running binary.
- **Release:** `GOOS=windows GOARCH=amd64 go build -o run8ops.exe ./cmd/run8ops`.
- **Test fixtures:**
  - `testdata/` holds copies of `WorldSave.xml`, `29K.xml` and `Bessemer Yard.xml` with their `.info` files, plus the CSV and limits file. These are used only by `go test`; they are not embedded in the binary.
  - Order and lifecycle tests build synthetic snapshots in Go, so no Run8 is needed.
- **Documentation:** every documentation file the change produces, including `README.md` and `docs/route-profiles.md`, is written with the `unslop` skill (`skill://unslop`). Read the skill before drafting, and apply its pass to the finished file.

## Risks / Trade-offs

- **[Track mapping effort: the route has thousands of sections]**
  - Learn mode labels a whole cut's sections at once and suggests labels from tags.
  - Unmapped cars are excluded from orders rather than guessed.
  - The coverage indicator shows the remaining work.
- **[Run8 writes the save non-atomically]** The settle check plus whole-file parse validation means a half-written file is retried and never ingested.
- **[Load state inferred from `loadWeightUSTons > 0`]** This holds for all cars in the reference save, where weight is either 0 or 36.6–117 t. If Run8 ever reports residual weight on empties, the profile gets a per-type empty threshold.
- **[Road train completion relies on cars disappearing off-map]** If a train is instead parked or relinquished on-map, the order stays In Progress, and the user completes it manually (spec'd).
- **[Auto-save world check is a car-overlap heuristic, because the sidecar has no source world]**
  - A large session change, such as a mass delete, could drop overlap below 90% and reject a genuine auto-save. A warning appears, and the next manual save of the named world resyncs, because it is always accepted.
  - A copy of the same world (same cars) would pass the check. That is harmless, because its state is the same railroad.
- **[Default lengths and tares are approximations]** Threshold and limit checks may be off by a car or two until real definitions are read. Defaults are editable per type.
- **[Some en-route stop details are inferred]** Car-type filters separate autoracks from containers at McCalla, bare `NOL` and `MER` racks are claimed by the 131/339 stops, and A28 is treated as off-map. All of this is profile data, editable without code changes. The validation report highlights unassigned industries and orphan tags.
- **[Overlapping available orders]** 131 and 339 can both list the same `BRI BRI` cars. Reservation on start resolves this. The dashboard marks shared cars so the overlap is visible.
- **[Car key collision when two cars share a model and number]** This is unlikely in Run8 rosters. A duplicate key in a snapshot is reported as a warning, and the second car gets a `#2` suffix for that snapshot.
- **[Format drift in future Run8 updates]** The parser fails closed: the previous snapshot is kept and a warning shown. Fixture tests pin the known schema.

## Open Questions

These are deferrable: each has a working default, and answering it changes profile data or an optional reader, not the specs or task structure.

- **Car definition files.** What are the location and format of Run8 car definition files under the install folder, for real lengths and tares? Until a sample file is added to `references/`, the profile defaults are used.
- **Orphan outgoing tags.** Which road trains should carry `MERCP`, `MERUP` and `ROS`? They ship unaliased and appear in the validation report until the user adds aliases. `JAX` is now covered by the 28J McCalla stop.
- **Mercedes.** How should MB, the 395 inbound and the 394 outbound (ROS) be modelled? They are left unconfigured in the default profile.
- **Industry status fields.** Do `UnprocessedCars` and `EscrowCars` carry useful data when populated? The design does not need them. They could later corroborate the working state.
