# Design

## Context

- Order generation decides "in a yard" in one function, `genInYard(c, yard)` in `internal/orders/generate_cars.go`. It checks only `c.Loc.Kind == LocYard && c.Loc.Yard == yard`. It has five callers:
  - `generate_local.go:58`, industry-line set-outs from the home yard;
  - `generate_yardline.go:18`, yard-line set-outs;
  - `generate_yardline.go:20`, yard-line pick-ups;
  - `generate_road.go:121`, `waitingFor`, which feeds `WaitingFt`, `Pct`, the threshold, the age trigger and the consist;
  - `generate_stops.go:88`, yard-stop pick-ups.
- `carstatus.Location.Track` holds the label's track name, such as `Bowl 14`, `R07` or `WF03`. The lifecycle's `inYard` and `atYardOrServed` (`lifecycle_reconcile.go`) also check only the yard. They decide when pick-ups have returned, not what's available.
- `profile.YardTrack{Name, LengthFt, Kind}` and `(*Yard).TrackByName`, which compares names after `NormTag` and ignores case, already exist. Today only learn mode and the yards view read them.
- Bowl and receiving names come from the Run8 import (`Bowl <n>`, `R01`–`R12`, the receiving thoroughfares). The track list leaves them out, so a departure list can't mark them by accident.

## Goals / Non-Goals

**Goals:**
- Pure data: any route marks its departure tracks in the profile, with no code change.
- Yards that mark nothing behave exactly as before.

**Non-Goals:**
- Car service states, the dashboard tiles and the yards view. A car in the bowl still reads as awaiting delivery or awaiting train.
- Order lifecycle: cars returned to any NOR track still complete a pick-up.
- Any UI for editing the flag. Users edit `profile.yaml`.

## Decisions

### D1. Profile schema
```yaml
tracks:
  - {name: WF01, lengthFt: 11448, departure: true}
```
- `YardTrack` gains `Departure bool`. The YAML key is `departure`, a boolean that defaults to false.
- **Validation (`invalid-track`):** `departure: true` on a `siding` or `main` entry is reported, and the entry loads without the flag. Sidings and mains are labelled ignored, so a car on one is never in the yard.
- `BundledSchemaVersion` becomes 5, so the existing upgrade replaces installed AGS profiles (v4 on the user's PC) and writes `.v4.bak` backups.
- **Alternatives considered:**
  - A `forwarding` kind. Rejected, because the local yard L02–L14 also feeds orders and isn't a forwarding yard. The flag also leaves `kind` to mean only how learn mode labels a track.
  - A yard-level `departureTracks: [...]` list. Rejected, because it repeats names already in `tracks`, and the two lists could drift apart.
  - Excluding the tracks named by `humps` and `trackEnds` instead. Rejected, because the user wants Norris tracks outside the forwarding and local yards (pullbacks, engine area, unlisted names) left out too.

### D2. Availability check (`internal/orders`)
- `newGenCtx` precomputes `departure map[string]map[string]bool`, from yard code to the normalised names of its departure tracks. Only yards that mark at least one track get an entry.
- `genInYard` becomes the method `(g *genCtx) inYard(c, yard)`. It returns false unless the car is `LocYard` in `yard`. When the yard has a departure entry, it also needs `NormTag` of `c.Loc.Track`, in upper case, to be in that set. All five callers switch to it, and the free function is deleted.
- **Lookup cost:** one map lookup per call. The set is built once per generation.

### D3. Bundled AGS data
- NOR sets `departure: true` on the 30 tracks EF01–EF07, Southern Outbound, WF01–WF07, CAB 01, CAB 02 and L02–L14. These are the forwarding and local yards the user chose.
- TUS, MCC, BES and 32ND mark nothing.
- The header comment in `profile.yaml` explains the flag.

### D4. Dwell
- Dwell is keyed by yard and track, so a car moved from the bowl to `WF03` starts a new dwell. For departure-track cars, oldest-first selection and a road train's age trigger therefore measure the time since trimming. That is the time a car has been ready to leave, which suits both rules. No change.

## Risks / Trade-offs

- **[A Norris label with a non-listed name, like `NOR / All` or `NOR / WF 1` in another spelling, silently stops feeding orders]** → Matching ignores case and extra spaces. The yards view's "Listed tracks" table already shows listed tracks that are not labelled yet. The README says that Norris orders draw only from departure tracks, and that labels need the listed names.
- **[A real-world operator builds a train on a receiving track ("anything goes", AGS guide p.29)]** → The user chose the strict rule. Unmarking happens per profile, and removing every `departure` flag from a yard restores the old behaviour.
- **[The test fixtures assume any NOR track counts]** → Engine, e2e and web fixtures move their NOR cars onto departure tracks. The orders unit tests keep a profile with no lists, which shows that unmarked yards behave as before. New tests cover marked yards.
- **[Another profile upgrade overwrites user edits]** → This was accepted in earlier changes. `.v<old>.bak` copies are kept, and `trackmap.json` is untouched.

## Migration Plan

- The first start of the new binary upgrades the installed AGS profile to version 5 and writes `.v4.bak` backups. Labels are untouched.
- Cars still in the bowl or receiving drop out of available orders on the next regeneration. In-progress orders keep their cars, because reservation doesn't depend on location.
- **Rollback:** restore `profile.yaml.v4.bak` and run the previous binary.
