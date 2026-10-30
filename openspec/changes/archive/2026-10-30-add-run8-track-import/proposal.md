# Proposal

## Why

Learn mode makes the user label every track section by hand, cut by cut, before cars show up in orders. Run8 already ships that knowledge in its install folder. `Config.ind` lists every industry's track sections. The Norris hump controller and the AI special locations name the bowl and receiving tracks, and the track database gives the full track layout. A parse of the AGS copies in `references/AutoLabel/` read every file to its last byte. Industry, Norris bowl and Norris receiving labels alone placed 221 of the 458 freight cars in `WorldSave.xml`. The other 237 are 218 cars in three powered trains and one 19-car cut parked on unnamed track. All 49 cars in the bowl sat on the track their destination tag is assigned to.

## What Changes

- New reader for Run8's static route files, opened read-only from the Run8 install folder (the configured folder, or the default `C:\Run8Studios\Run8 Train Simulator V3`):
  - from the region folder (`Content\V3Routes\Regions\<region>\`): `Config.ind`;
  - from each route folder (`Content\V3Routes\<route>\`): `TrackDatabase.r8`, `HumpControllerList.r8`, `AISpecialLocations.r8` and `MilepostDatabase.r8`.

  These formats are undocumented. A file that is missing or fails to parse disables only what depends on it, and learn mode keeps working.
- New **Import from Run8** action in learn mode. It builds labels for the sections Run8 names:
  - every profile industry's sections from `Config.ind`;
  - each hump bowl track (e.g. `NOR / Bowl 14`), from the hump controller's switch paths walked through the track database;
  - each receiving track (e.g. `NOR / Receiving 3`), from its named north and south end points.

  The user sees a preview of new, changed and removed labels, plus sections skipped because the user already labelled them. Nothing changes until the user confirms.
- Labels record their source, `run8` or manual. Manual labels always win, and editing an imported label makes it manual. Re-importing replaces only `run8` labels.
- The app stores a fingerprint of the files it imported from. When the files change, for example after a Run8 update or a swapped industry file, the dashboard says so and links to the import.
- Learn mode uses the track database for track the files don't name:
  - labelling a cut covers the whole switch-bounded track it stands on, not only the occupied sections;
  - the neighbour suggestion uses real track connections in place of section index ±1;
  - yard labels are pre-filled with the yard whose anchor (a Run8 milepost or a profile industry) lies nearest.
- The route profile gains a `run8` block that names the Run8 region and route folders and maps hump controllers and receiving-track names to yards. Each yard can also list anchors. The bundled AGS profile fills these in.
- Profiles gain a schema version. When the bundled profile's version is newer than the installed copy's, the app replaces the installed `profile.yaml`, `IndustryConfiguration.csv` and `train_limits.tsv` with the bundled ones, backs up the old files, and keeps the track map. Before this change an installed profile was never overwritten.
- **No Run8 binary files in the repository.** `.r8` and `.ind` files belong to the closed-source, paid Run8 software, and committing them could breach its licence. Tests that need them run only when local copies are present in the git-ignored `references/AutoLabel/`. CI tests use small synthetic files written by a test-only encoder.
- **BREAKING (spec):** the track-mapping rule "no label is applied automatically" changes to "labels change only by a user action". The import is such an action.
- **Out of scope:** guessing yard membership or main line for track the Run8 files don't name, which covers the Norris forwarding yards, 32nd St, Tuscaloosa, McCalla, Bessemer and the main line. `Hump.r8` is not read either, because its per-track tag assignments don't affect labels. The app never writes to any Run8 file.

## Capabilities

### New Capabilities
- `run8-route-data`: reading Run8's static route files (industry configuration, track database, hump controllers, AI special locations, mileposts) read-only from the install folder. Each file is located from the profile's region and route folders, may fail on its own without stopping the rest, and is fingerprinted so changes can be detected.

### Modified Capabilities
- `track-mapping`: adds the Run8 import with its preview, label provenance and the change notice. Whole-track labelling replaces per-cut-section labelling, and suggestions use real adjacency plus the nearest yard anchor in place of index ±1.
- `route-profiles`: adds the Run8 region and route folder settings, the hump and receiving-track mappings, per-yard anchors, and the schema-version upgrade of the bundled profile.
- `ops-dashboard`: the learn mode view gains the import action and its preview, and the dashboard attention list gains the Run8 track data notice.

## Impact

- New internal package for the binary route formats, below `trackmap` in the dependency order. `trackmap` gains the import builder and topology-based suggestions, and `engine` gains the import commands and file fingerprinting.
- `trackmap.json` gains a per-label `source` and import metadata. Files written by earlier versions still load, and their labels count as manual.
- `profile.yaml` gains optional fields and a `schemaVersion`. The installed AGS profile is upgraded in place on first start of the new version. Edits to its three files are lost, but backups are kept, and `trackmap.json` and any other files in the profile folder are untouched.
- No new committed fixtures from Run8. `.gitignore` gains `*.r8` and `*.ind`. The real-data tests read `references/AutoLabel/` and skip when it's absent, so CI runs only the synthetic tests.
- Depends on undocumented Run8 binary formats, which a Run8 update could change. Every write still goes through `store.Guard`, and the install folder is only ever opened for reading.
- Docs: `README.md` (first run and learn mode) and `docs/route-profiles.md` (the new profile fields).
