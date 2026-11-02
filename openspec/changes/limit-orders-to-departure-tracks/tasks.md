# Tasks

## 1. Profile departure flag

- [ ] 1.1 Add `YardTrack.Departure` and the `departure` YAML key. Validation reports `departure: true` on a `siding` or `main` entry under `invalid-track` and loads the entry without the flag (design D1). Document the key in `docs/route-profiles.md` using `skill://unslop`. Verify with profile tests showing that the flag loads on yard tracks, that it is flagged and dropped on a siding and on a main, and that `TestDocumentedFieldsMatchSchema` passes.
- [ ] 1.2 In the bundled AGS profile, mark the 30 NOR departure tracks (EF01–EF07, Southern Outbound, WF01–WF07, CAB 01, CAB 02, L02–L14), explain the flag in the header comment, and set `schemaVersion: 5` with `BundledSchemaVersion = 5` (design D3). Verify with `TestBundledYardTracks`, which checks that NOR marks exactly those 30 and no other yard marks any, plus the schema-version test and an upgrade test showing that a v4 install is replaced with a `.v4.bak` backup and an untouched `trackmap.json`.

## 2. Order availability

- [ ] 2.1 Replace `genInYard` with a `genCtx` method that uses the departure sets built in `newGenCtx`, and switch all five callers: local set-outs, yard-line set-outs and pick-ups, road-train waiting cars, and yard-stop pick-ups (design D2). Delete the free function. Verify with new orders tests using a profile where NOR marks departure tracks:
  - a bowl car is not a set-out, and on `WF03` it is;
  - 108 at 3,000 ft on `R07` plus 1,200 ft on `EF05` is building at 15% with no order;
  - an `A37 CYC` car on `NOR / All` is not on A55's Tuscaloosa line;
  - an `L08` car is an AB10 set-out;
  - TUS with no marks still yields its pick-ups.

  The existing tests on the unmarked profile must pass unchanged.
- [ ] 2.2 Move the engine, e2e and web fixtures that expect Norris orders onto departure tracks: `labelAllYard` in `engine/helpers_test.go`, `track=All` in `cmd/run8ops/e2e_test.go`, and the `Bowl n` labels in `web/dash_fixture_test.go` and `web/views_fake_test.go`. Keep the cars that the tests expect to stay unavailable where they are. Verify that the expected orders, build percentages and tile counts in those tests hold, or record each changed number with its reason in the commit message.
- [ ] 2.3 Update `README.md`: at Norris, local and road-train orders draw only from departure tracks, and labels need the listed track names. Write it using `skill://unslop`. Verify that the README describes the rule and the `NOR / All` caveat.

## 3. Integration

- [ ] 3.1 Run `go vet ./... && go test -race ./...` with and without `references/AutoLabel/`, and `openspec validate limit-orders-to-departure-tracks --strict`. Verify that all pass and that `git ls-files '*.r8' '*.ind' 'references/*'` prints nothing.
- [ ] 3.2 Smoke test on macOS. Run the binary against a temp install, with a v4 AGS profile and a track map that labels some NOR cars on bowl, receiving and WF tracks. Verify that the profile upgrades with a `.v4.bak` backup, that only the WF cars appear in available orders and build percentages, and that relabelling a bowl cut as `WF02` makes its cars available on the next regeneration.
- [ ] 3.3 On the Run8 PC, pull, build and run against the real save. Verify that cars still in the bowl or receiving don't trigger orders, and that orders appear once those cars are trimmed into the forwarding or local yard.

## Workflow follow-up

- Archive the change with `/opsx-archive` after 3.3 passes.
- Open the pull request only when the user asks.
