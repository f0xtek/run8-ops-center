# Route profiles

A route profile tells the Run8 Operations Centre how one Run8 route is operated. It lists the industries, yards, local jobs, road trains, train limits and car types. The app has no route knowledge of its own, so a new route needs a new profile and no code change.

On first run the app installs the AGS South Phase 1 profile into `<data>/profiles/ags-south-p1/`. It never overwrites an installed profile, so you can edit those files freely.

## Profile folder

Each profile is a folder under `<data>/profiles/`. The folder name is the profile ID.

| File | Required | Content |
|---|---|---|
| `profile.yaml` | yes | Yards, jobs, road trains, car types and everything else described below. |
| `IndustryConfiguration.csv` | yes | The route's industry table, copied from Run8. |
| `train_limits.tsv` | no | The route's train limits table, copied from Run8. Without it, nothing has a limit unless `profile.yaml` sets one. |

The app reads these files and never writes to them after the first-run install. It also never writes anywhere inside the Run8 install folder.

## IndustryConfiguration.csv

Copy the file from the route's Run8 folder without changes. The loader expects this layout:

- A title row, then a header row that starts with `Industry Name`. Rows above the header are ignored.
- One row per industry car type. The columns are industry name, incoming tag, local symbol, car type, time in hours, capacity, produces (`Loads` or `Empties`) and outgoing tags.
- A row with a blank industry name adds another car type to the industry above it.
- Outgoing tags are one comma-separated cell, usually quoted.
- An industry name may contain an unquoted comma. The loader counts back from the seven fixed columns after the name.

Every tag is trimmed and has repeated spaces collapsed. An incoming tag of ` BRR` becomes `BRR`, and a car tagged `AB10  BRR` matches it. Outgoing tags keep their file order. A tag that still contains a space after cleanup, such as `ATL NOL3`, and a tag listed twice in one row are kept so the validation report can show them.

## train_limits.tsv

The file is tab-separated. The first line is the header. The table ends at the first blank line, so any text below it, such as a subdivision table, is ignored.

| Column | Meaning |
|---|---|
| Base train symbol | The train or job symbol, for example `131` or `A55`. |
| Class codes | Comma-separated outgoing tags the train carries. |
| Destination | Free text shown for road trains. |
| Min HP/T | Minimum horsepower per ton. |
| Max length | Maximum length in feet. `4,000` and `4000` both work. |
| Max tons | Maximum tons. |
| Max trailing tons | Read but not used. |

A blank numeric cell means no limit.

A road train takes its class codes, destination and limits from the row with its symbol. A local job takes its limits from the row with its symbol. A train or job with no row has no limits. `profile.yaml` can override any of these values.

## profile.yaml

Unknown keys are an error, so a misspelled field stops the load instead of being ignored. Tags are cleaned the same way as in the CSV. The tables below list every field. Paths use `[]` for a list of objects.

### Identity and folders

| Field | Meaning |
|---|---|
| `id` | Profile ID. Defaults to the folder name. |
| `name` | Display name. Defaults to the ID. |
| `defaultSaveDir` | Run8 folder that holds the world saves. |
| `defaultAutoSaveDir` | Run8 folder that holds the auto-saves. |

### Yards

| Field | Meaning |
|---|---|
| `yards[].code` | Short code used everywhere else in the file, for example `NOR`. |
| `yards[].name` | Display name, for example `Norris`. |

### Local jobs

A local job switches industries from a home yard and can also serve satellite yards.

| Field | Meaning |
|---|---|
| `localJobs[].id` | Unique job ID. Defaults to the lower-cased symbol. |
| `localJobs[].symbol` | Train symbol, for example `A55`. |
| `localJobs[].homeYard` | Code of the yard the job works from. |
| `localJobs[].industries` | Incoming tags of the industries the job serves. If you leave the key out, the job serves every CSV industry whose local symbol equals the job symbol. A list, even an empty one, replaces that default. |
| `localJobs[].yardLines[].yard` | Code of a satellite yard the job visits. |
| `localJobs[].yardLines[].carries` | Tags the job brings to or takes from that yard. Each tag can be a local symbol, an industry tag or a class code. |
| `localJobs[].limit.maxLengthFt` | Overrides the maximum length from the limits file. `0` means no limit. |
| `localJobs[].limit.maxTons` | Overrides the maximum tons. `0` means no limit. |
| `localJobs[].limit.minHpt` | Overrides the minimum horsepower per ton. `0` means no limit. |

A limit field you leave out keeps the value from the limits file.

### Road trains

A road train with an `origin` is built in that yard. A road train without one is a through train that only works its stops.

| Field | Meaning |
|---|---|
| `roadTrains[].symbol` | Train symbol. Quote numeric symbols, for example `'131'`. |
| `roadTrains[].origin` | Code of the origin yard. Leave it out for a through train. |
| `roadTrains[].destination` | Display text. Defaults to the limits file destination. |
| `roadTrains[].classCodes` | Outgoing tags the train carries. Defaults to the class codes in the limits file. |
| `roadTrains[].offMapIndustries` | Incoming tags of industries the train carries off the map, such as the simulated industry `A28`. |
| `roadTrains[].stops[].kind` | `yard` or `industry`. If you leave it out, the loader looks the target up as a yard code and then as an industry tag. |
| `roadTrains[].stops[].target` | Yard code or industry tag. |
| `roadTrains[].stops[].setOut` | Tags the train sets out at a yard stop. |
| `roadTrains[].stops[].pickUp` | Tags the train picks up at the stop. |
| `roadTrains[].stops[].pickUpCarTypes` | Car types the stop picks up. Empty means any type. |
| `roadTrains[].limit.maxLengthFt` | Overrides or supplies the maximum length. `0` means no limit. |
| `roadTrains[].limit.maxTons` | Overrides or supplies the maximum tons. `0` means no limit. |
| `roadTrains[].limit.minHpt` | Overrides or supplies the minimum horsepower per ton. `0` means no limit. |
| `roadTrains[].minLengthPct` | Percent of the maximum length the waiting cars must reach before an order is available. Defaults to 50 with an origin and 0 without. |
| `roadTrains[].maxCarAgeDays` | Makes an order available once the oldest waiting car has waited this many days. `0` means unset. |

Stops are worked in the order you list them. The first train in the file that has an origin and carries a tag takes that tag, ahead of through trains.

### Car types

| Field | Meaning |
|---|---|
| `carTypes[].prefix` | Start of a car model file name, for example `R8_Gon_`. Matching is case-sensitive. |
| `carTypes[].type` | Industry car type for matching models. Use the spelling from the CSV, for example `Box Car` or `Bulkhead_Flat`. |
| `carDefaults.<type>.lengthFt` | Default length in feet for the car type. |
| `carDefaults.<type>.tareTons` | Default empty weight in tons for the car type. |
| `unknownDefaults.lengthFt` | Length in feet for a car whose model matches no rule. Defaults to 60. |
| `unknownDefaults.tareTons` | Tare in tons for a car whose model matches no rule. Defaults to 30. |

The first matching rule wins, so put longer or more specific prefixes first. A model that matches no rule has type `Unknown`. A save has no car length or weight, so the app uses the defaults for every car of a type.

### Tags and prefixes

| Field | Meaning |
|---|---|
| `aliases.<tag>` | Routes the outgoing tag `<tag>` as if it were the tag given as the value. For example `MERCP: MER` sends `MERCP` cars to the train that carries `MER`. |
| `yardInternalTags` | Tags that mean the car stays in its yard, for example `NOR Hump`. |
| `routePrefixes` | Track section prefixes that belong to this route. |

## Validation report

Loading a profile never fails because of a validation problem. The loader loads everything that is valid and lists the rest in the profile report.

| Code | Meaning |
|---|---|
| `orphan-tag` | An outgoing tag that no road train, stop pick-up, local job, yard line, yard-internal tag or alias handles. The message lists the industries that produce it. |
| `malformed-tag` | A tag that contains a space. |
| `unserved-industry` | An industry that no local job lists, no road train stops at and no road train carries off the map. |
| `unknown-ref` | A yard, industry or alias target that does not exist. |
| `duplicate-tag` | An incoming tag, job ID, train symbol or class code used twice, or an outgoing tag listed twice in one row. |
| `unmapped-car-type` | An industry car type that no `carTypes` rule produces. |
| `unmatched-model` | A car model in a save that matches no `carTypes` rule. This code comes from checking a save's models against the profile, not from loading. |

To clear an `orphan-tag` problem, add an alias, add the tag to a train's `classCodes`, or add it to a stop's `pickUp`.

## Adding a route

1. Create `<data>/profiles/<id>/`. The folder name is the profile ID.
2. Copy the route's `IndustryConfiguration.csv` into it. If Run8 ships a train limits table for the route, save its table as `train_limits.tsv`, with tabs between columns and a blank line after the last row.
3. Create `profile.yaml` with `id`, `name`, `defaultSaveDir` and `defaultAutoSaveDir`.
4. Add the `yards`. Pick a short code for each.
5. Add the `localJobs`. Start with `symbol` and `homeYard`. Add `industries` when a job serves industries whose CSV local symbol differs from the job symbol. Add `yardLines` for satellite yards.
6. Add the `roadTrains`. Give a train an `origin` if it starts at one of your yards. Add `stops` for en-route work.
7. Add `carTypes` and `carDefaults`. Find the model file names in a world save by searching for `rvXMLfilename`.
8. Start the app, select the profile and read the profile report. Fix each orphan tag, unserved industry and unknown reference, or leave the ones that are intentional.

## The bundled AGS profile

The AGS South Phase 1 profile follows the AGS NS Birmingham Guide. Some parts are approximations or choices you may want to change.

- The AGS profile defines no aliases. The report lists `MERCP` and `MERUP`, because the limits file has no train for them. Add `MERCP: MER` if you want `MERCP` cars to go on train 339.
- The report also lists `ATL NOL3` (a malformed tag in the CSV), `CSX1` (no train), `ROS` (no train) and a duplicate `CSX`. The industry MB (Mercedes Benz, local symbol 395) is left unconfigured because the guide gives no job for it, so it appears as unserved.
- The bare `NOL` and `MER` tags are picked up at McCalla by trains 131 and 339.
- Default car lengths and tares are approximate averages for the Run8 models. For example, an autorack is 89 ft and 48 tons, and a well car is 65 ft and 33 tons. They are not read from the Run8 car files.
- The `Reefer Car`, `Intermodal Flat Car` and `AggregateHopper` rules are inferred from model names (`R8_Reefer_`, `R8_Pig_` and `R8_Ortner_`). No AGS industry takes reefers or intermodal flats, so those cars never match an industry.
- In the AGS world save, the locomotive models and `R8_FCA_AutoFlood_III_NS` match no car-type rule.
