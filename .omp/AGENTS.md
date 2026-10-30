# Run8 Operations Centre

A Go app that follows a Run8 Train Simulator V3 world save and generates freight orders for the AGS South Phase 1 route (NS Birmingham, AL), similar to Depot+. The user runs it on the same Windows PC as Run8.

## Where things are

- `notes.txt`: the user's original requirements. It is git-ignored.
- `examples/switchlist-tracker.html`: the UI mockup to follow for look and feel. It is git-ignored.
- `references/`: Run8 sample saves, `IndustryConfiguration.csv`, `yard_and_train_limits.txt`, `TrainSymbolRoutings.xml` and the AGS guide and routing PDFs. It is git-ignored. The AGS guide (`AGS NS Birmingham Guide.pdf`) is the source of truth for how locals and yards operate.
- `openspec/changes/add-run8-ops-center/`: the active plan (proposal, seven specs, design, tasks). Implement it with `/opsx-apply` and keep the artifacts in sync when decisions change. Run `openspec validate add-run8-ops-center --strict` after editing them.
- `cmd/run8ops/` is the entry point (flags, wiring, end-to-end tests). `internal/` holds `run8save`, `profile`, `trackmap`, `carstatus`, `orders`, `store`, `watch`, `engine` and `web`. Dependency order is `run8save`, `profile`, then `trackmap`, `carstatus` and `orders`, with `engine` above them and `web` on top. Lower packages never import higher ones. Each package's `types.go` is its contract with the others.
- `testdata/` holds byte-exact copies of the Run8 fixtures. `.gitattributes` marks it and `internal/profile/bundled/` as non-text so git keeps the CRLF line endings.
- Check everything with `go vet ./... && go test -race ./...`. All writes go through `store.Guard`.

## Decisions the user has made

- Single Go binary, released as `windows/amd64`. Storage is flat JSON/YAML files, with no database.
- The UI is a local web page served on `127.0.0.1`, with live updates.
- Track locations are labelled by the user in an in-app learn mode, because saves only contain numeric track section indices.
- Orders are local jobs with industry lines and yard lines. Road train orders can have en-route stops, and through trains only pick up at their stops.
- There are no standalone transfer orders. In AGS every yard-to-yard move belongs to a local or a road train:
  - A55 serves Tuscaloosa for A37, A56 and AGR.
  - A44 serves McCalla for A39.
  - AB11 takes BNSF cars to 32nd St.
  - AB45 takes BRL and WWL cars to Bessemer.
  - 131 and 339 set out BRI racks at McCalla.

  Transfers may come later for routes such as SoCal.
- Road train orders become available at a configurable percentage of maximum length (default 50%).
- `--save <name>` is required, because the trains folder can hold several world saves.
- Default save folders:
  - world saves: `C:\Run8Studios\Run8 Train Simulator V3\Content\V3Routes\Regions\GulfCoast\Trains`;
  - auto-saves: `...\GulfCoast\AutoSaves`, named `Auto Save World YYYY.mm.dd HH.MM.SS.xml`.
- At runtime the app reads saves only from the Run8 install, never from `references/` or `testdata/`.

## Run8 save format facts

- An auto-save's `.info` file names only the auto-save itself, not its source world. The app therefore matches an auto-save to the selected world by car overlap: at least 90% of its car keys must be in the last accepted save.
- Car keys (`rvXMLfilename` plus `unitNumber`) and `trainID` stay the same between saves in a session.
- Destination tag spacing can change between saves (`AB10 BRR` versus `AB10  BRR`). Always normalise tags before comparing them.
- Auto-saves have no `<description>` element.
- A save has no car type, length, reporting mark, speed or track name. Car type comes from the model file name. A train's symbol comes from the lead locomotive's `destinationTag`.

## CI and releases

- Workflows are in `.github/workflows/`: `ci.yml` (branches and pull requests), `release.yml` (push to `main`) and `tests.yml` (the shared test job, run on Linux and Windows). Both must pass on both systems.
- CI runs once per push. A branch with an open pull request into `main` is tested by the pull request run, and the branch push run skips its jobs. This is the `gate` job in `ci.yml`.
- `.github/scripts/` holds `next-version.sh`, `check-title.sh`, their shared grammar `conventional.sh`, and a test for each script. Run the tests with `bash .github/scripts/<name>_test.sh`.
- Every push to `main` that holds a releasable commit publishes a GitHub release. The first release is `v0.1.0`. Before 1.0 a breaking change and a `feat` raise the minor number, and a `fix`, `perf` or `revert` raises the patch number. From 1.0 a breaking change raises the major number. Commits that are only `docs`, `ci`, `chore`, `build`, `test`, `style` or `refactor` publish nothing.
- The repository uses squash merges, so the pull request title becomes the commit subject on `main` and must be a Conventional Commit. `check-title.sh` enforces this on pull requests.
- The release build stamps the version with `-ldflags "-X main.version=<version>"`. A plain `go build` reports `dev`.

## Working conventions

- Commit messages follow Conventional Commits.
- `references/` is git-ignored, so test fixtures are copied into `testdata/`.
