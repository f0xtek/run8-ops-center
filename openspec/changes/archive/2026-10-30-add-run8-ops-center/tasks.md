# Tasks

## 1. Project setup

- [x] 1.1 Initialise the Go module (`go mod init`, Go 1.26) with the `cmd/run8ops` and `internal/*` package layout from design D3. Add the `fsnotify` and `yaml.v3` dependencies. Verify that `go build ./...` and `go vet ./...` succeed.
- [x] 1.2 Copy the fixtures into `testdata/`: `WorldSave.xml`/`.info`, `Auto Save World 2026.10.28 17.28.02.xml`/`.info`, `29K.xml`/`.info`, `Bessemer Yard.xml`/`.info`, `IndustryConfiguration.csv` and `yard_and_train_limits.txt`. Verify that the files are present and byte-identical to `references/`.

## 2. World save parsing (`internal/run8save`)

- [x] 2.1 Implement `ScnInfo` sidecar decoding (`SaveType`, `Filename`, `DisplayFilename`, `FileTimestamp`, which may be missing). Verify that the unit tests return `World`, `Train` and `ExtraBoard` for the three fixtures.
- [x] 2.2 Implement the streaming `ScnLoader` decoder that produces a Snapshot with trains, vehicles and both truck positions, game date and save type, skipping signals and switches; `description` is optional. Verify that a test on the `WorldSave.xml` fixture gives 31 trains, 498 vehicles, 40 locomotives, 458 freight cars and the game date `2026-10-28T17:22:23.7433957Z`. Also verify that the auto-save fixture (no `description`) parses to the same counts with game date `2026-10-28T17:12:47.1275649Z`.
- [x] 2.3 Classify each train as a cut or a powered train and take the symbol from the lead locomotive's tag (spec: Cut and train distinction). Verify that a test shows the `29K.xml` fixture's train with symbol `29K` and that the fixture's loco-free trains are cuts.
- [x] 2.4 Make invalid or truncated XML return an error and never a partial snapshot. Verify with a test that feeds the reference save truncated at its midpoint.

## 3. Route profiles (`internal/profile`)

- [x] 3.1 Implement the industry CSV loader (title row, header row, continuation rows, quoted fields, tag trimming and whitespace collapsing). Verify that the fixture test gives 46 industries and 68 car-type entries, that CMC has 3 entries, and that BRR's incoming tag is `BRR`.
- [x] 3.2 Implement the limits TSV loader. It must stop at the first blank line, parse thousands separators and treat blank cells as no limit. Verify that the fixture test maps class code `NOL1` to train 131 at 8000 ft / 8000 t and that the subdivision table is ignored.
- [x] 3.3 Define the `profile.yaml` schema:
  - yards;
  - local jobs: id, symbol, home yard, industries, yard lines with carried tags;
  - road trains: symbol, optional origin, class codes, off-map industries, ordered yard/industry stops with set-out tags, pick-up tags and pick-up car types, limit overrides, thresholds;
  - car-type patterns and defaults, tag aliases, yard-internal tags and route prefixes.

  Implement loading, with industries defaulted from the CSV local symbol. Verify with a unit test that loads a minimal YAML containing one local with a yard line and one road train with a stop.
- [x] 3.4 Implement the validation report: orphan outgoing tags (stop pick-up tags count as resolved), malformed tags, industries served by no job, yard lines or stops that reference unknown yards or industries, duplicate tags, and unmatched car models. Verify that the AGS fixture test reports `ATL NOL3`, `CSX1`, `MERCP`, `MERUP`, `ROS` and MB, does not report `JAX`, and does not fail the load.
- [x] 3.5 Author the bundled AGS South Phase 1 profile following design D10 and the D7 car-type patterns (add a Well Car pattern after checking the fixture's filename families), and embed it. Implement first-run installation into `<data>/profiles/ags-south-p1/`. Verify that a test on an empty temp data dir installs the profile and that it loads. The test must also check that A55 serves KYK and ABS with a TUS yard line carrying A37/A56/AGR, that 131 has a MCC stop, and that 346 is a through train.
- [x] 3.6 Document the profile file formats and how to add a new route in `docs/route-profiles.md`. Read `skill://unslop` before writing, and run the finished file through it. Verify that each documented field exists in the schema struct and that the unslop pass was applied.

## 4. Track mapping (`internal/trackmap`)

- [x] 4.1 Implement the label model (yard, industry or ignored, keyed by prefix/section), lookup, and atomic load/save of `trackmap.json`. Verify with a round-trip unit test.
- [x] 4.2 Implement learn-mode candidates: group snapshot trains by their unmapped sections, with car count, sample numbers and tags. Verify that on the reference fixture with an empty map, every train is a candidate and the section lists are correct for one sampled cut.
- [x] 4.3 Implement suggestions: an industry track when all of a cut's cars are tagged for one industry, and a neighbouring label when an adjacent section is labelled. Also implement bulk labelling of a candidate's sections. Verify that a test cut of 19 `BRI BRI` cars gets the suggestion BRI and that bulk labelling covers all of its sections.

## 5. Car status (`internal/carstatus`)

- [x] 5.1 Implement car identity (key, reporting mark derivation), type from patterns, load state, gross weight and length from defaults. Verify with table tests over model names seen in the fixture (`…_ARMN`, `…_TTJX01`, `Run8_Tank107GATX02`).
- [x] 5.2 Implement destination classification: industry-bound, train-bound (including aliases and off-map industries), foreign, unassigned and unknown. Verify with a table test covering `AB45 HTI`, `AB10  BRR`, `CHA`, `NOL1`, `A28 A28`, `A28 PHS`, `CPKC .`, `NOR Hump`, `None` and `XYZ123`.
- [x] 5.3 Implement location resolution (leading truck, then trailing truck) and the service states (awaiting delivery, working, ready to pull, awaiting train, en route, other). Verify with synthetic-snapshot tests, one per state.
- [x] 5.4 Implement dwell tracking (first-seen game date per car location, reset on move) and the snapshot diff for the activity log (appeared, disappeared, moved, retagged), comparing normalised tags. Verify with a two-snapshot test in which a car is retagged `AB45 HTI` → `NOL1` and another car moves. Also verify that diffing the auto-save fixture against the `WorldSave.xml` fixture reports 300 moved vehicles, all in trains 999962, 999967, 999965 and 999949, with 0 appeared, 0 disappeared and 0 retagged (the `AB10 BRR` / `AB10  BRR` spacing is not a retag).

## 6. Order generation (`internal/orders`)

- [x] 6.1 Implement local job orders with industry lines: the readiness gate, set-outs from the home yard under per-type capacity (unknown types count against total capacity), oldest dwell first, pick-ups, and the job limit flag. Verify with tests matching each order-generation scenario (still processing, all retagged, capacity of 6 with 8 waiting, outside home yard, pick-up only, over length).
- [x] 6.2 Implement local yard lines: set-outs by carried tags from the home yard, and pick-ups of satellite-yard cars that don't belong there (not carried, not industry-bound for a job homed there, not claimed by a road train stop there). Verify with tests for the A55 Tuscaloosa block, the AB45 Bessemer block, pulls returned from Tuscaloosa, and autorack empties left at McCalla for 131.
- [x] 6.3 Implement road train orders: origin yard gathering (train-bound cars plus stop set-outs), minimum percentage threshold, maximum-age trigger, length/ton cap, and required HP. Verify with tests for below threshold, capped at 8000 ft, stop set-outs counting toward the threshold, the age trigger and 6,200 HP.
- [x] 6.4 Implement road train stops and through trains: yard stops (set-out tags; pick-up tags and car types within remaining length/tons), industry stops (readiness gate, capacity, ready pick-ups), and pick-up-only through-train orders. Verify with tests for 131 working McCalla, pick-ups limited by length, autorack vs well car `ATL` split between 346 and 28J, A28 working PHS, and 346 collecting ATL racks.
- [x] 6.5 Implement reservation exclusion and stable available-order IDs, including overlapping available orders (`BRI BRI` cars on both 131 and 339) that resolve when one is started. Verify with a test in which the started order's cars are absent from regenerated orders and IDs stay the same across two regenerations.

## 7. Order lifecycle (`internal/orders`)

- [x] 7.1 Implement the state transition table: start a whole order or a single line, confirm, continue with fingerprint suppression, manual complete, and cancel. Invalid transitions must be rejected. Verify with transition tests for every state/action pair.
- [x] 7.2 Implement completion detection:
  - local jobs: industry set-outs at their industry, yard-line set-outs delivered to the satellite yard (in the yard or at an industry served from it), pick-ups in the home yard, per-car and per-line progress;
  - road trains: stop set-outs delivered to their stop, all other cars absent;
  - the missing-car flag, covering local cars and road set-outs carried off-map.

  Verify with a test per lifecycle scenario, including continue followed by an identical snapshot not re-detecting.
- [x] 7.3 Implement history records with real and in-game start and finish times. Verify with a test that confirmed and cancelled orders appear in history with their timestamps.

## 8. Persistence and engine (`internal/store`, `internal/engine`)

- [x] 8.1 Implement atomic JSON persistence for `settings.json` and `state.json` (orders, history, dwell, activity log capped at 2,000 entries) and data directory resolution (`--data`, `%APPDATA%\Run8OpsCenter`). Verify with a test that writes, reloads and checks equality, and that a simulated crash mid-write leaves the old file intact.
- [x] 8.2 Implement the protected-path guard in `internal/store`, following design D2 and the save-ingestion requirements "Data and profile paths outside the Run8 install directory" and "Write guard". Apply it to `--data`, profile paths, install-folder changes and every write, rename or remove, including first-run profile installation from task 3.5. Verify with table tests for an inside path, case variants, a relative path, a symlinked path, a not-yet-existing target, the sibling `…V3 Ops` (allowed), and an install folder set after the data dir (rejected). Also verify that a startup test with `--data` inside the install dir exits before creating any file.
- [x] 8.3 Implement the single-writer engine pipeline (derive → reconcile → generate → persist → publish immutable View) with a command channel for UI actions and profile/settings changes. Verify with an engine test that feeds two snapshots and a start command and checks View contents. Run it under `go test -race`.
- [x] 8.4 Implement profile switching. Orders from other profiles are retained but hidden. Verify with an engine test that switches profiles and back.

## 9. Save watching (`internal/watch`)

- [x] 9.1 Implement the watcher: fsnotify plus a 5 s poll fallback and a 1 s settle check. It follows the named save file and the auto-save folder, filtered to `Auto Save World YYYY.mm.dd HH.MM.SS.xml`, and accepts only candidates newer than the last ingest. Verify with temp-dir tests:
  - a rewrite of the named save is ingested;
  - a different world save and a Train save in the save folder are ignored;
  - a non-matching file in the auto-save folder is ignored;
  - a matching newer auto-save is ingested;
  - an older one is skipped;
  - a file still growing is not ingested until stable.
- [x] 9.2 Wire watcher errors (missing folder, parse failure) into ingest warnings that keep the previous snapshot. Verify with an engine test that a truncated file produces a warning and leaves the View unchanged.
- [x] 9.3 Implement CLI save selection and the auto-save world check:
  - `--save` is required, with a case-insensitive name match with or without `.xml`, and must be a world save;
  - `--save-dir` and `--autosave-dir` default to the profile's `defaultSaveDir` and `defaultAutoSaveDir`;
  - startup picks the newest of the named save and the matching auto-saves;
  - the world check requires at least 90% of the auto-save's car keys to be present in the last accepted snapshot; the sidecar is not used for it, because it names only the auto-save itself.

  Verify with unit tests: a missing flag, an unknown name and a Train-save name each exit with the list of world saves; a missing save folder exits with no fallback to `references/` or `testdata/`; a missing auto-save folder only warns. Also verify that the sample auto-save sidecar decodes as `World` with its `FileTimestamp`, that 440 of 458 cars matching (96%) is accepted, that 40 of 300 is rejected with a warning that shows the percentage, and that a newer matching auto-save wins at startup.

## 10. Web UI (`internal/web`)

- [x] 10.1 Implement the HTTP server (default bind `127.0.0.1`, configurable port and all-interfaces option), the port-in-use error, the `--no-browser` flag, browser opening, and embedded static assets. Verify that a manual run prints the URL and opens it, and that a second instance exits with the port error.
- [x] 10.2 Port the example's styling (CSS variables, light/dark, tiles, cards, pills, chips, offline font stacks) into a base layout with a header showing save name, game date and last ingest. Verify visually in a browser against `examples/switchlist-tracker.html`.
- [x] 10.3 Implement the dashboard: summary tiles, active orders, available orders by kind, road train build percentage and the attention list. Verify by loading the reference save and checking that the tile counts match the car-status test expectations.
- [x] 10.4 Implement the order detail switchlist views: local orders with industry and yard lines, and road orders with departing consist and ordered stops. Show only the actions valid for each state, and mark cars shared between available orders. Verify in a browser by starting a whole job and a single yard line, then continuing, confirming and cancelling orders against fixture-driven state.
- [x] 10.5 Implement the yards & industries view, with per-type occupancy against capacity. Verify that HTI's occupancy displays correctly with a labelled fixture track map.
- [x] 10.6 Implement the learn-mode view (candidates, suggestions, accept or override, label list with rename, relabel and remove). Verify in a browser by labelling a cut, then checking that it leaves the list and the counts update.
- [x] 10.7 Implement the setup view (read-only world save name and save/auto-save folders from the CLI, Run8 install folder, profile selection, per-train thresholds and max age, port and bind with a restart notice, validation report) and the activity log view. Verify that the setup view shows the `--save` name read-only, and that changing 108's minimum recomputes the orders immediately.
- [x] 10.8 Implement the SSE `/events` endpoint and the fragment refresh JS. Verify that dropping a new matching auto-save into the watched auto-save folder updates an open dashboard without reloading.

## 11. Integration and release

- [x] 11.1 End-to-end smoke test on macOS: run the binary with `--data <tmp>/data --save WorldSave --save-dir <tmp>/Trains --autosave-dir <tmp>/AutoSaves`, with the temp folders standing in for the Run8 folders. Copy in the reference save, label a few tracks and start an order. Then drop an edited copy that completes it into `AutoSaves` as `Auto Save World 2026.10.30 19.15.00.xml`, and confirm. Verify the full lifecycle in the UI and in `state.json`.
- [x] 11.2 Cross-compile `run8ops.exe` (`GOOS=windows GOARCH=amd64`), and run it on the Run8 PC with only `--save <your world>`, against a live session with manual and auto-saves. Verify that:
  - the setup view shows the AGS `Trains` and `AutoSaves` folders;
  - manual saves and auto-saves are each ingested within 10 s;
  - an auto-save from a different world is rejected with a warning;
  - no files in the Run8 folders change.
- [x] 11.3 Write a `README.md` covering build, run flags (`--save` required, `--save-dir`, `--autosave-dir`, `--data`, `--no-browser`), first-run learn mode workflow and the order lifecycle. Read `skill://unslop` before writing, and run the finished file through it. Verify that the documented commands run as written and that the unslop pass was applied.
