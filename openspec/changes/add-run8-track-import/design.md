# Design

## Context

- Track labels live in `profiles/<id>/trackmap.json` (`internal/trackmap`). A label is `{kind, yard, track, industries}`, keyed by `"<prefix>/<section>"`. Every write goes through `store.Guard`.
- Learn mode (`trackmap.Candidates`) groups snapshot trains by their unmapped sections. It suggests a label from destination tags, or from a labelled section whose index is ±1 away. The archived design already called the ±1 hint unreliable.
- Order generation reads only a car location's yard code. The yard track name is used for display, the dwell key and the yards view.
- The app already has an optional Run8 install folder setting (`store.Settings.RunInstallDir`). The guard protects it, and the built-in default is `C:\Run8Studios\Run8 Train Simulator V3`.
- The route files are undocumented binary formats. A throwaway parse of the AGS copies in `references/AutoLabel/` established the layouts below and decoded each file to its last byte. Where they differ, these findings correct the community notes (Puyodead1's `Run8-V3-reverse-engineering` and the `R8-Industry-Tool` `r8lib.py`).
- Install locations, as the user reported them: `Config.ind` and `Hump.r8` are in `Content\V3Routes\Regions\GulfCoast\`; `TrackDatabase.r8`, `HumpControllerList.r8`, `AISpecialLocations.r8` and `MilepostDatabase.r8` are in `Content\V3Routes\NS_AGS_Phase01\`.

## Goals / Non-Goals

**Goals:**
- Decoders that accept a file only when it decodes exactly to its end, and that cannot be made to allocate unbounded memory by a corrupt count.
- One topology model shared by the import, whole-track labelling and suggestions.
- No new dependencies: `encoding/binary`, `regexp` and `math` from the standard library are enough.

**Non-Goals:**
- Writing any Run8 format.
- Reading `Hump.r8`. It holds per-track tag assignments, and labels don't need them.
- Signals, blocks, speeds, terrain and any other route file.
- Guessing yards or main line for track the files don't name (see the proposal).

## Decisions

### D1. New leaf package `internal/run8route`
- It sits beside `run8save` and imports nothing from the project. `trackmap` imports it; `engine` loads it.
- **Strings ("R8String").** An `int32` byte count, then that many bytes. Each pair of bytes encodes one character as `b[i]<<4 | b[i+1]>>4`.
- **Bounds.** Every count is checked against the bytes left in the file, divided by the minimum record size, before anything is allocated. A failed check, a short read or leftover trailing bytes rejects the whole file.
- **Layouts (little-endian):**

| File | Header | Record |
|---|---|---|
| `Config.ind` | `i32` reserved, `i32` count | `i32`, name, local, tag, `u8` process-in-blocks, `i32` track count × {`i32`, `i32` prefix, `i32` section, `i32` node}, `i32` car count × {`i32` version, `u8` car type, `u8` produces-empties, `i32` hours, `i32` capacity, `i32` n × tag, `i32` n × filter string}. The filter count is present in every version; the AGS file decoded to its end with it read unconditionally |
| `TrackDatabase.r8` | `i32` reserved, `i32` count | `i32`, `i32` node count × 79-byte node, `i32` index, `u8` lever, `i32` n × `i32` next section, `u8` type, `f64` retarder, `u8` occupied, `u8` stand side, `i32` stand type, `u8` CTC |
| track node (79 B) | | `i32`, `i32`×2 tile, `f32`×3 start, `f32`×3 tangent, `f32`×3 end, `i32` index, `u8` switch, `u8` reverse, `f32` curve, `i32` sign, `f32` radius, `f32` arc length, `i32` segments, `i32` section, `u8` sound |
| `HumpControllerList.r8` | `i32` reserved, `i32` count | `i32`, name, `i32`×2 tile, `f32`×3 position, string, `i32` path count × {`i32`, track name, `i32` n × {`i32`, `i32` section, `u8` reversed}} |
| `AISpecialLocations.r8` | `i32` reserved, `i32` count | `i32`, name, `u8` type, `i32` prefix, `i32` section, `u8`, `u8`, 2 B, `i32`, `i16` time, 2 B, `u8` skip |
| `MilepostDatabase.r8` | `i32` reserved, `i32` count | `i32`, milepost string, station name, `i32`×2 tile, `f32`×3 position |

- The package keeps only what is used. Per section that is its next sections, whether it is a switch, its length (the first node's arc length; the two nodes are the two directions of the same path), and its midpoint.
- **Fingerprint:** one `{path, size, modTime}` per file.
- **Alternatives considered:**
  - Kaitai-generated parsers: there is no Go target in the existing community work.
  - Shelling out to community tools: Windows-only, and they write files.

### D2. Topology model
- **Switches.** A section with four nodes is a switch: 990 of the 10,630 AGS sections. Its next-section list is `[points, normal, reverse]`. This held for all 296 switch steps on the 56 Norris hump paths.
- **Switch-bounded track.** The maximal chain of non-switch sections reached by following next links, stopping at a switch or a dead end. A walk never goes back to the section it came from.
- **Adjacency for suggestions.** The sections linked to a track's ends, plus the sections linked to a switch at either end.
- **World coordinates.** `x = tile.x·W + local.x` and `z = tile.z·H + local.z`. The tile size is estimated from the database itself: the median over straight sections whose two ends fall in adjacent tiles. For AGS that comes to about 844.3 m by −1025.8 m. Hard-coded constants were rejected because the tile size is not known to be the same on every route. Coordinates are used only for anchor distance.

### D3. Import builder in `trackmap`
- `BuildImport(route data, profile)` returns the proposed labels, every one with source `run8`, plus skipped items with reasons and unavailable features.
- **Industries.** `profile.NormTag(tag)` must name a profile industry. Every listed section gets `industry [tag]`. A section listed by two industries gets both tags. None do in AGS.
- **Hump bowl.** For each path, take the last switch, follow `next[1 + reversed]`, and walk the switch-bounded track. The label is `yard / <prefix> <track name>`. A section claimed by two tracks is reported and left out.
- **Receiving tracks.** The profile regex has named groups `n` and `end` and is applied case-insensitively. Ends with the same `n` are paired north to south. A breadth-first search finds the path between the two end sections, through non-switch sections only and for at most 60 sections. A pair that is incomplete, unreachable or crosses a switch is skipped with a reason.
- `Diff(current, proposed)` sorts every section into one of: new, changed `run8`, removed `run8`, kept manual, or unchanged. The preview groups those by location.
- `ApplyImport` applies a diff to a copy of the map and records the fingerprint.

### D4. `trackmap.json` additions
```json
{
  "labels": { "280/548": { "kind": "industry", "industries": ["BRL"], "source": "run8" } },
  "run8Import": { "at": "2026-10-30T14:00:00Z",
                  "files": [ { "path": "...\\Config.ind", "size": 41094, "modTime": "..." } ] }
}
```
- `source` is omitted for manual labels, so existing files load unchanged and their labels count as manual. An unknown `source` value is a load error, just like an unknown kind.
- `LabelSection`, `LabelCandidate` and `RenameLocation` write manual labels, which matches the spec rule that an edit makes a label manual.

### D5. Engine wiring
- **Loading.** The engine holds the decoded route data and reloads it at startup, on a profile switch and on an install folder change. A failed file becomes a warning naming its path and reason.
- **Notice.** On every snapshot ingest the engine stats the five files, without decoding them, and compares the result with `run8Import.files`. A Run8 update or an industry-file swap during a session is therefore noticed on the next save. The View gains a `Run8` struct: data status, unavailable files, last import time, and a changed or import-available flag.
- **Preview.** `PreviewImport()` re-reads the files, builds the diff and returns it with a token: a hash of the route data fingerprint plus the track map's revision.
- **Confirm.** `ConfirmImport(token)` recomputes both. A mismatch is refused with "review a new preview". That is stricter than the spec, which names only route data changes. A label edit made between preview and confirm also invalidates the diff.
- **Commit.** Confirm uses the existing `commitMap` path, so the write goes through the guard and the recompute and reconcile run.

### D6. Learn mode changes
- A `Candidate` gains `Track []SectionRef`: the unmapped sections of every switch-bounded track the train occupies, plus any switch sections it occupies. `Apply` labels `Track`. Without route data, `Track` equals the occupied sections.
- **Suggestions,** in order:
  1. The tag suggestion, only when that industry has no labelled sections, or when one of its labelled sections is adjacent to the candidate's track. After an import, the 19-car `BRI BRI` cut in the Norris East Forwarding Yard (608 m from that milepost) no longer gets a `BRI` suggestion.
  2. The connected-label suggestion. An industry label is offered whole. A yard label offers only its yard, with the track name blank.
  3. The nearest anchor within 3 km, offering its yard.
- The ±1 index neighbour code is deleted.
- A suggestion with an empty track name pre-fills the label form and has no one-click accept.
- New source constants: `connected` and `anchor`. They replace `neighbour`.

### D7. Profile schema
```yaml
yards:
  - {code: NOR, name: Norris, anchorMileposts: [Receiving Yard, West Forwarding Yard, East Forwarding Yard]}
  - {code: MCC, name: McCalla, anchorIndustries: [BRI]}
run8:
  region: GulfCoast
  routes:
    - {folder: NS_AGS_Phase01, prefix: 280}
  humps:
    - {controller: Norris Yard Hump Controller, yard: NOR, trackPrefix: Bowl}
  trackEnds:
    - {pattern: '^Norris RCV (?P<n>\d+) (?P<end>North|South) end$', yard: NOR, trackPrefix: Receiving}
```
- **Prefixes.** A route's `prefix` must also appear in `routePrefixes`. Hump paths carry only section indices, so they take the prefix of their route folder.
- **Validation.** The profile report checks folder names (single component, no separator, no `..`), unknown yards, regexes that fail to compile or lack the `n` and `end` groups, and anchor industries that aren't profile industries. Anchor milepost names can only be checked once route data is loaded, so the engine adds those findings to the report.
- **AGS anchors:**
  - NOR: the three milepost names above;
  - 32ND: milepost `32nd St Yard`;
  - TUS: milepost `Tuscaloosa MP 199.1`;
  - MCC: industry `BRI`, because the McCalla milepost lies 2.4 km from the yard;
  - BES: industry `BRL`, because no milepost names Bessemer yard.

### D8. Web
- **Learn page.** A new "Import from Run8" card shows the data status, the last import and the notice. `POST /trackmap/import/preview` renders the preview: counts per category, then collapsible lists grouped by location. `POST /trackmap/import/confirm` carries the token, and cancelling is a link back to `/learn`.
- **Label list.** Imported labels get an "imported" pill.
- **Dashboard.** The attention list gains the notice and unusable-file items, linking to `/learn#import`.
- **Setup.** The install folder help text changes to "Used to read car files and Run8 track data".

### D9. Fixtures and tests: no Run8 binaries in the repository
- **Rule.** `.r8` and `.ind` files are part of the closed-source, paid Run8 software and are never committed. `.gitignore` gains `*.r8` and `*.ind`, so a copy dropped anywhere in the tree stays out of git. `Config.ind` is treated like the `.r8` files: the industry CSV export is public data, the binary file is not. Committed tests also hold no data transcribed from those files beyond names and the oracle counts and section numbers below.
- **Synthetic fixtures (CI).** A small package, `internal/run8route/run8routetest`, imported only from `_test.go` files, encodes each format byte for byte. It writes a fake install tree (`Content/V3Routes/Regions/<region>/Config.ind`, `Content/V3Routes/<route>/*.r8`) to `t.TempDir()`. The synthetic route is small but covers every case the code branches on:
  - a main line with a CTC switch;
  - a hump lead whose switches feed four bowl tracks, with normal and reverse legs, and a controller path for each track;
  - a two-track receiving yard whose end names differ in case (`South End` and `South end`), plus an unpaired end and a pair whose path crosses a switch;
  - a multi-section industry spur, an industry with a padded tag (` BBB`), and an industry unknown to the profile;
  - an anchor milepost near the yard and one more than 3 km away.

  Decoder tests round-trip the encoder. The trackmap, engine and web tests build on this tree, so CI exercises the whole feature without Run8 files. Truncation and inflated-count tests cut down or patch the synthetic files.
- **Real-data tests (local only).** A helper copies the git-ignored `references/AutoLabel/` files into a temp install tree and calls `t.Skip` when they are absent, so CI skips these tests. They check the numbers measured during exploration:
  - industries: 46, with BRL on 8 sections and BRR on 280/8346;
  - track database: 10,630 sections and 990 switches;
  - bowl: 56 tracks and 336 sections;
  - receiving: 14 tracks and 120 sections;
  - `WorldSave.xml`: 221 of 458 freight cars placed after the import;
  - the `BRI BRI` cut's track is 6 sections long.
- **Bowl check (local only).** The 49 bowl cars in `WorldSave.xml` each sit on the track whose `Hump.r8` assignment matches their tag. A test-only decoder reads `Hump.r8` from the local copies (header, then hump name, configs, and tracks with their tags, all as R8Strings), so the assignment table is never committed.

### D10. Bundled profile upgrade
- `profile.yaml` gains `schemaVersion` (an integer; absent means 1). The bundled AGS profile becomes version 2. The loader accepts any version up to the bundled one; a newer version is a load error naming both numbers.
- `InstallBundled` keeps its first-run path. When the folder exists, it reads only `schemaVersion` from the installed `profile.yaml`:
  - older than bundled: for each of `IndustryConfiguration.csv`, `train_limits.tsv` and `profile.yaml`, in that order, it writes the old file to `<name>.v<old>.bak` and then the bundled file over it. `profile.yaml` goes last, so an interrupted upgrade leaves the old version number and the upgrade runs again at the next start;
  - equal or newer: nothing is written;
  - unparseable: nothing is written, and the normal load reports the error.
- Every write goes through the guard's `FileWriter`, as on first run. `trackmap.json` and any other file in the folder are never opened for writing.
- **Alternatives considered:**
  - Merging user edits into the new schema: rejected, because the user accepted overwrites and a YAML merge would be fragile.
  - Keeping "never overwrite" and asking the user to copy the block by hand: rejected for the same reason.

## Risks / Trade-offs

- **[A Run8 update changes a format]** → Whole-file decoding rejects the file. Each file fails on its own and is named on the dashboard, and learn mode keeps working.
- **[Tile size estimate off on another route]** → Anchors only pre-fill a yard. Adjacency, the import and whole-track labelling don't use coordinates.
- **[An industry lists only part of its spur]** → The rest of the spur stays unmapped. Learn mode's connected-industry rule offers that industry for cuts on the rest of the spur.
- **[Removing an imported label, then re-importing, brings it back]** → The preview shows it as new. To suppress a section for good, label it `ignored`, which is a manual label.
- **[The profile upgrade discards user edits to the three bundled files]** → The user accepted this. Each old file is kept as `<name>.v<old>.bak`, and `trackmap.json` is never touched.
- **[CI never sees real Run8 files]** → The synthetic route covers every code branch. The real-data tests run on any machine that has `references/AutoLabel/`, and task 7.2 checks the real install on the Run8 PC.
- **[A Run8 file committed by mistake]** → `.gitignore` excludes `*.r8` and `*.ind` everywhere in the tree.
- **[Run8's `node` field in `Config.ind` (0 or 1) is not understood]** → It is ignored, and the whole section is labelled.

## Migration Plan

- Track maps from earlier versions load unchanged, with every label manual. The first import adds `source` and `run8Import`.
- An installed AGS profile is upgraded to schema version 2 at the first start of the new binary (D10). The old files are kept as `.v1.bak`, and the track map is untouched.
- **Rollback:** an older binary rejects the upgraded `profile.yaml`, because `schemaVersion` and `run8` are unknown keys to it. Restore the `.v1.bak` files to roll back. An older binary still reads a `trackmap.json` that has `source` fields, because the current `Load` uses plain `json.Unmarshal`, which ignores them, and it treats every label as manual.
