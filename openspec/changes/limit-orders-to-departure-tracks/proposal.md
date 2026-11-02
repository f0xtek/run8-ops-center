## Why

Order generation treats every car labelled `NOR / <track>` as ready to leave Norris. Cars still in the receiving yard or the hump bowl are counted towards local set-outs, yard-line blocks and road-train build percentages, so orders become available before the cars have been trimmed out of the bowl. At Norris, trim crews pull cars from the bowl into the East or West Forwarding yard, where trains are built. Only cars standing there, or in the local yard, should feed orders.

## What Changes

- A yard's track list can mark tracks as departure tracks with `departure: true`. Only yard-kind tracks can be departure tracks.
- When a yard marks at least one departure track, order generation counts a car as in that yard only if it stands on a departure track. Cars in the bowl, in receiving or on any other NOR track, including names that aren't in the list, are left out until they are moved. A yard with no departure tracks keeps today's behaviour, where every yard track counts.
- The rule applies wherever order generation looks for cars in a yard: industry-line set-outs from the home yard, yard-line set-outs and pick-ups, cars waiting for a road train at its origin (which set the build percentage, the threshold and the age trigger), and road-train yard-stop pick-ups.
- In the bundled AGS profile, Norris marks 30 departure tracks: EF01–EF07, Southern Outbound, WF01–WF07, CAB 01, CAB 02 and the local yard L02–L14. Tuscaloosa, McCalla, Bessemer and 32nd St mark none.
- The bundled profile moves to schema version 5. Installed copies are replaced with a `.v<old>.bak` backup, as before. `trackmap.json` is not touched.
- **BREAKING**: On Norris, cars labelled with track names that aren't departure tracks, such as a hand-typed `NOR / All`, no longer feed orders. To make those cars available, relabel them with listed track names.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `order-generation`: a new requirement defining which yard cars are available to orders, limited to departure tracks when the yard lists any.
- `route-profiles`: the yard track list gains the `departure` flag, with validation and the AGS Norris departure tracks.

## Impact

- `internal/profile`: the `YardTrack.Departure` field, the YAML key, validation, a yard-level helper, the bundled `profile.yaml` (`schemaVersion: 5`) and `BundledSchemaVersion`.
- `internal/orders`: replace the yard-code-only `genInYard` with a check that knows about departure tracks, on every generation path that reads yard cars. The lifecycle (`lifecycle_reconcile.go`) stays as it is, so cars returned to any NOR track still complete a pick-up.
- Tests:
  - Engine, e2e and web fixtures that label Norris cars `NOR / All` or `NOR / Bowl n` and expect orders need departure-track labels.
  - The profile track-list tests need the new counts.
- Docs: `README.md` (when local and road-train orders appear) and `docs/route-profiles.md` (the `departure` key).
- Unchanged: car service states and dashboard tiles. A car in the bowl still shows as awaiting delivery or awaiting train. The road-train build percentage on the dashboard follows the new rule, because it comes from the same waiting-car list.
