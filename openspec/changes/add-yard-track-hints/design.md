# Design

## Context

- Learn mode (`trackmap.Candidates`) already gives each entry `Track`: the unmapped sections of the switch-bounded tracks it stands on. Its suggestions pre-fill a yard from a connected label or the nearest anchor. The label form (`internal/web/templates/learn.html`) has a kind select, a yard select and a free-text track name.
- `run8route.Section.LengthM` gives every section's length, so a candidate's measured length is the sum over the whole switch-bounded runs it occupies, labelled sections included.
- The Depot's maps are raster images (`references/maps/*.pdf`, no text layer), so the lists are transcribed by hand into the bundled profile. The user confirmed that track names and lengths may be published; the maps as a whole are copyrighted and stay git-ignored.
- **Spike results** (throwaway, on `references/AutoLabel/`):
  - Bryant to Tuscaloosa: 38 unlabelled runs. Within 6% by length, TY02, TY05, TY07, TY08 and Pass have two or three candidates each, TY01 matches only a main-line run, and TM01, TAGS and Chevron match none.
  - BRI's 25 `Config.ind` sections lie on two runs of 6,879 ft and 6,147 ft, which are MRH1 (6,645 ft, BRIP) and MRH2 (5,977 ft, BRIM). MST1 to MST4 and MCET are unlabelled.
  - Non-CTC runs near McCalla measure 5,327, 5,255, 5,059, 4,801 and 4,799 ft, against the listed MST1 5,141, MST2 4,890, MST3 4,555 and MST4 4,555 ft. Ranking puts the right group first, but not always the right track.
  - Run8's `RCV 1` to `RCV 14` run across the receiving yard in physical order, but their names don't match the map's R01 to R12. The user chose to keep `Receiving n`.

## Goals / Non-Goals

**Goals:**
- One click picks the right name in the common case, and the user can always override.
- Pure data for new routes: a route gets hints by adding `tracks` to its yards, with no code change.

**Non-Goals:**
- Automatic assignment, ladder or ordering matchers.
- Lists for places that aren't profile yards (Sloss Yard, Highline, the Mercedes plant, the CSX Boyles and West End maps).
- Renaming imported labels.

## Decisions

### D1. Profile schema
```yaml
yards:
  - code: TUS
    name: Tuscaloosa
    anchorMileposts: [Tuscaloosa MP 199.1]
    tracks:
      - {name: TY01, lengthFt: 1379}
      - {name: Pass, lengthFt: 752, kind: siding}
```
- `kind` is one of `yard` (the default), `siding` or `main`. `lengthFt` is optional; 0 or absent means unknown.
- **Validation codes:** `invalid-track` covers an empty name, a negative length and an unknown kind. `duplicate-track` covers a name that repeats within the yard after `NormTag`. The bad entry is dropped and the rest load.
- `profile.Yard` gains `Tracks []YardTrack{Name string; LengthFt int; Kind TrackKind}`.
- `BundledSchemaVersion` becomes 3, so the D10 upgrade from the previous change replaces the installed AGS profile and writes `.v2.bak` backups.

### D2. Measured length and hint ranking (`internal/trackmap`)
- `Candidate` gains `TrackLengthFt int`, the sum of `LengthM × 3.28084` over the full switch-bounded runs of every section the train occupies, switch sections included. It is 0 without route data.
- `func NameHints(y profile.Yard, used map[string]bool, measuredFt int) []NameHint` returns `NameHint{Track profile.YardTrack; DiffFt int; HasDiff bool}`:
  - entries whose name is in `used` are dropped, `used` holding every track name labelled in that yard;
  - when `measuredFt > 0`, entries with a length come first, sorted by `|lengthFt − measuredFt|` and then by profile order; entries without a length follow in profile order;
  - when `measuredFt == 0`, the profile order is kept.
- `Map` gains `TrackNamesInYard(code string) map[string]bool`.
- **Alternative considered:** a hard length tolerance that hides far-off entries. Rejected, because the spike showed map and Run8 lengths can differ by over 25%, as with TM01 and TAGS.

### D3. Learn mode UI (`internal/web`)
- The view model gives each candidate its hints for every yard that has a list, plus `TrackLengthFt`. Hints are computed server-side per yard, so a yard change needs no round trip.
- **Track name field.**
  - The field gets a per-candidate `<datalist>` for the selected yard. Each option's value is the name, and its label is `710 ft listed, 717 ft measured` or `listed length unknown`.
  - A small `app.js` handler swaps the datalist when the yard select changes.
  - It sets the kind select to `ignored` when the chosen name belongs to a `siding` or `main` entry, using `data-kind` on the option.
  - The candidate card shows the measured length.
- **Without JavaScript.** The datalist for the pre-filled yard is rendered by default, and the server applies the same siding and main rule on submit: a yard label whose name matches a `siding` or `main` entry is stored as `ignored`. So the behaviour doesn't depend on the script.
- **Alternatives considered:**
  - A `<select>` in place of the datalist. Rejected, because free text must stay possible.
  - Hint buttons. Rejected, because they crowd the card when a yard has 50 or more tracks.

### D4. Yards view
- For each yard with a list, a "Listed tracks" table shows every entry:
  - name;
  - listed length;
  - measured length when the track has labelled sections and route data is loaded;
  - status, either the car count or "not labelled yet".
- The measured length comes from the labelled sections of that location, using `run8route` section lengths.

### D5. Bundled AGS lists
Transcribed from The Depot's maps: AGS South v04.04.2026, pages 3 to 6, and Norris Yard v03.27.2026. Kinds are judgement calls the user can edit. Lengths are in feet.

| Yard | Yard tracks | Sidings and main lines |
|---|---|---|
| NOR, East Forwarding | EF01 2625, EF02 2625, EF03 2432, EF04 2530, EF05 7025, EF06 6835, EF07 6563, Southern Outbound 2478 | East Thoroughfare (main) |
| NOR, West Forwarding | WF01 11448, WF02 11455, WF03 11439, WF04 6188, WF05 6080, WF06 5730, WF07 5725, CAB 01 710, CAB 02 626 | Old AGS Main 7225 (main) |
| NOR, pullbacks and hill | East Pullback 3887, West Pullback 3887, Hill TF 752, Middle 543, Stock Pen 505, Outbehind 752, Ferguson 752, Rail Highway 1713 | Engine Thoroughfare 01 (main), Engine Thoroughfare 02 (main) |
| NOR, RIP | RIP 555, RIP Shop 1 2437, RIP Shop 2 2815, RIP Shop 3 2820, RIP Shop 4 2820, Wash Track 785 | |
| NOR, engine area | Adjust 1 417, Adjust 2 417, Adjust 3 501, Wash Rack 785, Engine Shop 1 585, Engine Shop 2 585, Engine Shop 3 585, Engine Shop 4 668, Engine Shop 5 585, Engine Shop 6 585, Outbound 1 668, Outbound 2 668, Outbound 3 543, Outbound 4 543, Outbound 5 668, Butt Head 334, Inside 2048, Old Outside 995, Outside Ramp 877, Bentley 334 | Shop Thoroughfare West 1170 (main), Shop Thoroughfare East 752 (main) |
| NOR, local yard | L02 292, L03 376, L04 376, L05 710, L06 835, L07 910, L08 1040, L09 1085, L10 1212, L11 1253, L12 1380, L13 1455, L14 1588 | |
| 32ND | NY01 2913, NY02 2745, NY03 2645, SY01 2241, SY02 1905 | Main 1, Main 2, Passenger Main (main) |
| TUS | TY01 1379, TY02 877, TY04 334, TY05 376, TY06 543, TY07 500, TY08 710, TM01 1081, TS01 1044, TAGS 1500, Chevron 167 | Pass 752 (siding) |
| BES | BY01 1170, BY02 912, BY03 961, Hill Track 1648 | Potter Track 2550 (siding), Main 1, Main 2 (main) |
| MCC | MST1 5141, MST2 4890, MST3 4555, MST4 4555, MCET 961 | Running Track, Siding (siding), Main (main) |

- **Renamed where the map repeats a name in one yard.**
  - The map labels both the car shop and the engine shop tracks "Shop 1" and so on; here they become `RIP Shop n` and `Engine Shop n`.
  - It has two "Shop Thoroughfare" tracks, which become `West` and `East` here.
- **Left out:**
  - Norris receiving tracks and the hump bowl, which the import names.
  - MRH1 and MRH2, which belong to industry BRI.
  - Tuscaloosa's "Scale", which has no length and shares a line with Chevron on the map.
  - The local yard's first track, which has no label on the map.
- **WF01** is labelled 11,148 ft at the west end of the map and 11,448 ft at the east end. The list uses 11,448 ft.
  - A local-only check on `references/AutoLabel/` found two unlabelled runs of that length within 1,500 m of the West Forwarding Yard milepost, at 11,572 ft and 11,491 ft. The next unlabelled runs of 3,000 ft or more are 7,804, 7,446, 6,631, 5,656, 5,358, 4,654, 4,578, 4,403 and 3,735 ft, so no run is near 11,148 ft.
  - The two runs are 124 and 43 ft (1.1% and 0.4%) from 11,448 ft, and 424 and 343 ft (3.8% and 3.1%) from 11,148 ft. The east-end figure is the closer one, so 11,448 ft stays.
  - The route data holds only two runs of about 11,500 ft for the three tracks WF01 to WF03, so the third presumably splits at a switch. Ranking for these tracks will be rough, and the user picks the name.
- **Southern Outbound** is labelled 2,395 ft on the vertical lead north of the East Forwarding Yard and 2,478 ft at the row of EF tracks. The list uses 2,478 ft, the label on the row.
- **Second read** (task 2.2). Every entry was compared with a crop of the map, rendered to a temp folder and not kept. All names and lengths match the table above, and the kinds are unchanged. WF01 and Southern Outbound are the only tracks the map labels twice with different lengths. The counts per yard match the table: NOR 70, 32ND 8, TUS 12, BES 7 and MCC 8.

## Risks / Trade-offs

- **[A transcription error puts a wrong name or length in the hints]** → The user picks from the hints and doesn't apply them blindly. Task 2.2 has a second read of every entry against a zoomed crop, and a check that the counts per yard match this table.
- **[Map and Run8 lengths differ, so the right name ranks second or third]** → All unused names stay in the list, ordered by closeness, and both lengths are shown.
- **[Another profile upgrade overwrites user edits]** → This was accepted in the previous change. `.v2.bak` copies are kept, and `trackmap.json` is untouched.
- **[The siding and main rule changes a yard label to ignored]** → It applies only when the typed name exactly matches a `siding` or `main` entry of the chosen yard. The README says so.

## Migration Plan

- The first start of the new binary upgrades the installed AGS profile to version 3, writing `.v2.bak` backups. Labels are untouched. Labels the user made earlier under other names stay as they are, and the hints simply offer the listed names those labels don't use.
- **Rollback:** restore the `.v2.bak` files and run the previous binary.
