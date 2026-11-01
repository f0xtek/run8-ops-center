# Proposal

## Why

After the Run8 import, the yard tracks Run8 doesn't name are still labelled by hand: the Norris forwarding, local and shop tracks, 32nd St, Tuscaloosa, McCalla and Bessemer. For each one the user has to know or look up the track's name and type it in. The Depot's AGS route maps list those tracks by name with their lengths, for example `TY01 - 1,379'` and `NY02 - 2,745'`. Learn mode already knows which yard a cut is probably in, from the yard anchors, and how long the track is, from `TrackDatabase.r8`. A spike showed that length alone can't pick the track: on the Bryant to Tuscaloosa stretch, five of the 12 map tracks have two or three Run8 runs within 6% of their length. So the user stays the judge, and the map supplies the names and the most likely candidates.

## What Changes

- Each profile yard can list its tracks: a name, a length in feet from the map, and a kind (yard track, siding or main line). The bundled AGS profile carries the lists transcribed from The Depot's AGS South and Norris Yard maps, as names and lengths only, and moves to schema version 3. The existing upgrade replaces the installed copy and backs it up.
- In learn mode, when the user labels a cut, the track name field offers the chosen yard's listed tracks that no section uses yet. They are sorted by how close each listed length is to the measured length of the track the cut stands on, and both lengths are shown. Choosing a siding or main-line entry sets the label to ignored. The user can still type any name.
- The yards view lists every listed track. It shows the labelled ones with the listed length next to the measured one, and the unlabelled ones as not labelled yet.
- Imported labels keep their names. Norris receiving stays `Receiving 1` to `Receiving 14`, so the list leaves out the map's R01 to R12 and the two receiving thoroughfares.
- **Out of scope:** assigning names automatically, matching by ladder structure, and tracks outside the five profile yards, such as Sloss Yard, Highline and the Mercedes plant. Industry tracks stay with `Config.ind`.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `route-profiles`: yards gain a track list (name, length, kind), with validation.
- `track-mapping`: learn mode offers the yard's listed tracks as track name hints, ranked by length.
- `ops-dashboard`: the yards view shows listed tracks that are labelled and not yet labelled.

## Impact

- `internal/profile` gains the yard track list, its validation and bundled data, and `BundledSchemaVersion` becomes 3. `internal/trackmap` gains measured track lengths and hint ranking. The `internal/web` learn and yards views change, with a small `app.js` addition.
- The installed AGS profile is upgraded again on first start. Its files get `.v2.bak` backups, and `trackmap.json` is untouched.
- Data source: The Depot's maps, v04.04.2026 (AGS South) and v03.27.2026 (Norris Yard). Track names and lengths only; the maps themselves stay in the git-ignored `references/maps/`.
- Docs: `docs/route-profiles.md` (the new field) and the README learn-mode section.
