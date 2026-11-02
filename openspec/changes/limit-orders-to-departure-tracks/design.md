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
- Bowl and receiving names come from the Run8 import (`Bowl <n>`, `R01`–`R12`, the receiving thoroughfares). The track list leaves them out, so the profile can't mark them by accident. Only a user mark can make one a departure track.
- `SetThreshold` already stores user overrides of profile values in `settings.json` (`store.Settings.Thresholds`). `effectiveProfile()` merges them into `e.eff`, which feeds `orders.Generate` and the `View`. Profile upgrades never touch `settings.json`.

## Goals / Non-Goals

**Goals:**
- Pure data: any route marks its default departure tracks in the profile, with no code change.
- The user can change any yard's departure tracks at runtime, without editing YAML.
- Yards with no departure tracks behave exactly as before.

**Non-Goals:**
- Car service states and the dashboard tiles. A car in the bowl still reads as awaiting delivery or awaiting train.
- Order lifecycle: cars returned to any NOR track still complete a pick-up.
- Writing user marks into `profile.yaml`.

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

### D2. Availability check (`internal/orders`, `internal/profile`)
- `profile.Yard` gains `Marks map[string]bool`, from normalised track name (`NormTag`, upper case) to the user's mark: true marks the track, false unmarks it. Only the effective profile fills it (D5).
- `(*Yard).DepartureSet() map[string]bool` returns the yard's departure tracks: listed tracks with `Departure`, plus names marked true, minus names marked false. A nil result means every yard track counts.
- `newGenCtx` builds `departure map[string]map[string]bool` once per generation, from each yard's `DepartureSet()`. Only yards with a non-empty set get an entry.
- `genInYard` becomes the method `(g *genCtx) inYard(c, yard)`. It returns false unless the car is `LocYard` in `yard`. When the yard has an entry, `NormTag(c.Loc.Track)`, in upper case, must also be in the set. All five callers switch to it, and the free function is deleted.
- **Lookup cost:** one map lookup per call.

### D3. Bundled AGS data
- NOR sets `departure: true` on the 30 tracks EF01–EF07, Southern Outbound, WF01–WF07, CAB 01, CAB 02 and L02–L14. These are the forwarding and local yards the user chose.
- TUS, MCC, BES and 32ND mark nothing.
- The header comment in `profile.yaml` explains the flag.

### D4. Dwell
- Dwell is keyed by yard and track, so a car moved from the bowl to `WF03` starts a new dwell. For departure-track cars, oldest-first selection and a road train's age trigger therefore measure the time since trimming. That is the time a car has been ready to leave, which suits both rules. No change.

### D5. Storing the user's marks (`internal/store`, `internal/engine`)
- `store.Settings` gains `Departure map[string]map[string]map[string]bool` (JSON `departure,omitempty`), keyed by profile id, then yard code, then track name as displayed. It is keyed by profile, unlike `Thresholds`, so a mark on one route's yard code never applies to another route.
- `effective()` copies the active profile's marks into each yard's `Marks`, after normalising the names. Marks for yards the profile doesn't define are ignored.
- The new engine command `SetDeparture(yard, track string, on bool) error` follows `SetThreshold`:
  - it rejects an unknown yard, an empty track name, and a listed `siding` or `main` track;
  - it stores the mark, or deletes it when `on` equals the profile default, so `settings.json` holds only differences;
  - it saves through the guard, rebuilds `e.eff`, recomputes and finishes.
- The `Backend` interface gains `SetDeparture`, and the web fakes implement it.
- **Alternatives considered:**
  - Writing the flag into the installed `profile.yaml`. Rejected, because the next schema upgrade replaces that file and the marks would survive only in a `.bak`.
  - Storing marks in `trackmap.json`. Rejected, because that file holds section labels. Marks are per location, and settings already hold the other user overrides of profile values.

### D6. Yards view (`internal/web`)
- Each yard heading states its rule: "Orders take cars from its N departure tracks only" or "Orders take cars from every yard track".
- Each labelled yard track row, and each `yard`-kind row in "Listed tracks", shows its departure state and a small form that posts `yard`, `track` and `on` to `POST /yards/departure`. The form works without JavaScript and redirects back to `/yards` with a flash message. Siding and main rows show no toggle.
- A row whose state differs from the profile is tagged "changed from profile". Toggling it back removes the mark (D5).
- Live updates already re-render the page when the engine publishes.

## Risks / Trade-offs

- **[A Norris label with a non-listed name, like `NOR / All` or `NOR / WF 1` in another spelling, silently stops feeding orders]** → Matching ignores case and extra spaces. The yards view shows each yard's rule and lets the user mark the track. The README says that Norris orders draw only from departure tracks.
- **[A real-world operator builds a train on a receiving track ("anything goes", AGS guide p.29)]** → The user marks that receiving track in the yards view and unmarks it afterwards.
- **[A mark is left behind after a label is renamed]** → It does nothing while no label uses the name, and applies again if the name comes back. Only labelled and listed tracks are shown, so these marks aren't visible. That's acceptable at this scale.
- **[Unmarking every Norris departure track switches Norris to every yard track]** → This follows the rule for yards with no departure tracks, and the yard heading states it.
- **[The test fixtures assume any NOR track counts]** → Engine, e2e and web fixtures move their NOR cars onto departure tracks. The orders unit tests keep a profile with no lists, which shows that unmarked yards behave as before. New tests cover marked yards.
- **[Another profile upgrade overwrites user edits]** → This was accepted in earlier changes. `.v<old>.bak` copies are kept, and `trackmap.json` is untouched.

## Migration Plan

- The first start of the new binary upgrades the installed AGS profile to version 5 and writes `.v4.bak` backups. Labels are untouched.
- Cars still in the bowl or receiving drop out of available orders on the next regeneration. In-progress orders keep their cars, because reservation doesn't depend on location.
- User marks in `settings.json` aren't touched by the upgrade.
- **Rollback:** restore `profile.yaml.v4.bak` and run the previous binary. It ignores the unknown `departure` key in `settings.json`.
