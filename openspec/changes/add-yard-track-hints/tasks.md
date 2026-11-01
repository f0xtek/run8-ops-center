# Tasks

## 1. Profile track lists (`internal/profile`)

- [x] 1.1 Add `tracks` (name, `lengthFt`, `kind` of `yard`, `siding` or `main`) to the yard schema and `profile.Yard.Tracks` (design D1). Add the `invalid-track` and `duplicate-track` findings, which drop only the bad entry. Verify with unit tests: a valid list loads in order with kind defaulting to `yard`; an empty name, a negative length, an unknown kind and a duplicate name after whitespace normalisation are each flagged and dropped while the rest of the yard loads.
- [x] 1.2 Document `tracks` in `docs/route-profiles.md`, including the siding and main-line rule and that lists come from route maps as names and lengths only. Read `skill://unslop` before writing, and run the finished text through it. Verify that the doc test keeping the schema and the docs in step passes, and that the unslop pass was applied.

## 2. Bundled AGS lists

- [x] 2.1 Add the lists from design D5 to the bundled AGS `profile.yaml` and set `schemaVersion: 3` (`BundledSchemaVersion = 3`). Verify with the bundled-profile test: no validation findings; the entry counts per yard match the D5 table (NOR 70, 32ND 8, TUS 12, BES 7, MCC 8); TUS matches the route-profiles scenario; NOR has no `R01`–`R12` or receiving thoroughfare. Also verify that the upgrade test now upgrades a version-2 install with `.v2.bak` backups.
- [x] 2.2 Check the transcription. Re-read every entry against a zoomed crop of the maps in `references/maps/`, rendered to a temp folder and never committed, and fix any mismatch. Then, in a local-only test that skips without `references/AutoLabel/`, print the lengths of the unlabelled Run8 runs near the WF01, WF02 and WF03 west forwarding tracks and record in design D5 whether 11,148 ft or 11,448 ft is closer for WF01. Verify that every D5 entry was checked and that the WF01 note is updated.

## 3. Hints (`internal/trackmap`)

- [x] 3.1 Add `Candidate.TrackLengthFt`, the full switch-bounded runs the train occupies with switch sections included, and 0 without route data. Also add `Map.TrackNamesInYard` and `NameHints` (design D2). Verify with synthetic tests: the measured length equals the sum of the synthetic run lengths; hints sort by length difference and then profile order; entries without a length go last; used names are dropped; without a measured length the profile order is kept. Then verify with a local-only test that a cut on the 717 ft Tuscaloosa run ranks TY08 then Pass first, as in the track-mapping scenario. If no 717 ft run carries a cut in the fixtures, use a synthetic candidate with that measured length.

## 4. Web UI (`internal/web`)

- [x] 4.1 Add the per-candidate hint datalists for each listed yard, the measured length on the card, and the `app.js` handler that swaps the datalist on a yard change and sets kind to ignored for `siding` or `main` entries (design D3). Add the server-side rule that stores a yard label whose name matches the chosen yard's `siding` or `main` entry as `ignored`. Verify with handler and view tests: the hints are rendered in ranked order with both lengths; used names are absent; posting yard `TUS` with name `Pass` stores an ignored label; posting a free-text name stores it unchanged. Then verify in a browser against a temp install built from `references/AutoLabel/`: choose a yard, pick a hint, and apply it.
- [x] 4.2 Add the "Listed tracks" table to the yards view (design D4). Verify with view tests that labelled entries show the listed and measured lengths and that unlabelled entries show "not labelled yet" (the TY06 scenario). Then check it in a browser.
- [x] 4.3 Update the README learn-mode section for track name hints, the siding and main rule, and the yards view table. Read `skill://unslop` before writing, and run the finished text through it. Verify that the steps match the UI from 4.1 and 4.2 and that the unslop pass was applied.

## 5. Integration

- [x] 5.1 Smoke test on macOS: seed a version-2 AGS profile with a `trackmap.json`, run the binary against a temp install from `references/AutoLabel/`, and confirm the import. In learn mode, label one Tuscaloosa cut and one 32nd St cut from their hints. Verify that the profile was upgraded with `.v2.bak` backups and the labels were kept, that the hints rank as expected, that the two labels use the map names, and that the yards view lists the remaining tracks as not labelled yet.
- [x] 5.2 Run `go vet ./... && go test -race ./...` with and without `references/AutoLabel/`, and `openspec validate add-yard-track-hints --strict`. Verify that all pass and that `git ls-files '*.r8' '*.ind' 'references/*'` prints nothing.
- [ ] 5.3 On the Run8 PC, pull the branch, build it, and label a few yard tracks from the hints. Verify that the profile upgrade kept the track map and that no file under `C:\Run8Studios\Run8 Train Simulator V3` changes.
