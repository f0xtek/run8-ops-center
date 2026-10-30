# Run8 Operations Centre

Run8 Operations Centre reads a Run8 world save and tells you which trains to run. It follows the save while you play, works out where every car is, and builds orders in the style of Depot+. You open a web page next to the sim, start an order, run the train, and the app notices when the work is done. You never move cars around in the app.

The default profile is the AGS South Phase 1 route (NS Birmingham to Powers). Other routes work by adding a profile folder. See [docs/route-profiles.md](docs/route-profiles.md).

The app runs on the same Windows PC as Run8. It only reads from the Run8 folders and never writes there.

## Build

You need Go 1.26 or later.

```
go build -o run8ops.exe ./cmd/run8ops
```

To build the Windows binary from macOS or Linux:

```
GOOS=windows GOARCH=amd64 go build -o run8ops.exe ./cmd/run8ops
```

The binary has no other files to install. The AGS profile is built in. The app copies it to the data folder on the first run and upgrades it when a newer version ships (see [Profile upgrade](#profile-upgrade)).

`run8ops.exe --version` prints the version. A build made with plain `go build` reports `dev`. Release builds report the release version.

## Run

```
run8ops.exe --save MyWorld
```

`--save` is required because the Trains folder can hold several world saves. Give the file name from that folder, with or without `.xml`. The match ignores case. If the name is missing, unknown, or belongs to a train save or extra board save, the app lists the world saves it found and exits before starting the web page.

On start the app prints the address of the web page and opens it in your browser. The default address is http://127.0.0.1:8080/. Only your PC can reach it unless you turn on the all-interfaces option in Setup and restart the app.

| Flag | Meaning |
|---|---|
| `--save <name>` | The world save to follow. Required. |
| `--save-dir <dir>` | The folder that holds world saves. The default is the route's Trains folder, `C:\Run8Studios\Run8 Train Simulator V3\Content\V3Routes\Regions\GulfCoast\Trains` for AGS. |
| `--autosave-dir <dir>` | The folder that holds world auto-saves. The default is `...\GulfCoast\AutoSaves`. |
| `--data <dir>` | Where the app keeps its own files. The default is `%APPDATA%\Run8OpsCenter`. |
| `--port <n>` | The web server port for this run. The saved setting is 8080 until you change it in Setup. |
| `--no-browser` | Do not open the browser on start. |
| `--version` | Print the version and exit. |

### Saves and auto-saves

The app reads the named save and also follows the world auto-saves, which are files named `Auto Save World YYYY.mm.dd HH.MM.SS.xml`. It uses whichever is newer. At startup it picks the newest matching auto-save if that is newer than your named save.

The auto-save folder holds auto-saves from every world, and an auto-save's `.info` file does not say which world it came from. The app therefore accepts an auto-save only when at least 90 percent of its cars are in the last save it accepted. A rejected auto-save shows as a warning with its match percentage. Your next manual save of the named world is always accepted.

The app waits until a file stops changing before it reads it, so it never reads a half-written save.

## First run

At first start the app does not know where any car is. A world save records track section numbers and nothing else. Run8 keeps the industry tracks, the hump bowl and the receiving tracks in its route files, so the app reads those and labels them for you. You label what is left in learn mode.

### Import the track labels from Run8

1. Open the Learn mode page and find the Import from Run8 card. The app reads the route files from the Run8 install folder, which is `C:\Run8Studios\Run8 Train Simulator V3` unless you choose another folder in Setup. The card says whether the files could be read and names any file it could not use, with the reason.
2. Press Preview the import. The preview changes nothing. It counts the new labels, the changed labels, the removed labels, the sections that keep a label you made by hand, and the skipped items, with a reason for each skipped item. It lists the locations one by one, and you open a location to see its sections.
3. Press Confirm the import to save the labels, or Cancel to leave the track map as it was. If a Run8 file or a label changed after you opened the preview, the app refuses to confirm and asks you to review a new preview.

For AGS the import labels the industry tracks from `Config.ind`, the Norris hump bowl tracks `NOR / Bowl 1` to `NOR / Bowl 56`, and the Norris receiving tracks `NOR / Receiving 1` to `NOR / Receiving 14`. The Labelled sections list marks each imported section with an "imported" tag. An import never changes a label you made by hand. If you relabel an imported section, it becomes a manual label and later imports leave it alone.

The dashboard attention list tells you when an import is available, when the Run8 track data changed since your last import, and when a route file could not be used. Each item links to the import card. After a Run8 update or a swap of the industry file, preview a new import to pick up the changes.

### Label what is left

1. The Learn mode page lists every cut and train that sits on an unlabelled section, with its cars, its destination tags and the sections it occupies.
2. Label a cut as a yard track (pick the yard and give the track a name), an industry track (pick the industries), or ignored (mainline and other track that is not a location).
3. When the route data is available, one label covers the whole track the cut stands on, not only the sections under its cars. The list shows the other unmapped sections of that track before you apply the label. The Apply to choice labels a single section instead.
4. The page offers a hint when it can guess a label. It never applies a hint until you accept it. It tries these in order.
   - The industry that all the cars in the cut are tagged for, when that industry has no labelled section yet or has one next to the cut's track.
   - The label of a section the cut's track joins, directly or across one switch. An industry label is offered as it is. For a yard label the app offers the yard only.
   - The yard whose anchor, a milepost or an industry named in the profile, is nearest to the cut and within 3 km.
5. A hint that names a yard and no track gets no accept button. The app fills the yard into the label form and opens it, and you type the track name.
6. The unmapped count on the dashboard drops as you go. Cars on unlabelled track stay out of orders.

The labels live in the profile folder and survive restarts. You only label a track once.

### Profile upgrade

The import needs the version 2 AGS profile, which adds the Run8 folders, the hump and receiving-track rules and the yard anchors. When a newer bundled profile ships, the app upgrades the installed one at the next start. For each of `IndustryConfiguration.csv`, `train_limits.tsv` and `profile.yaml`, it saves the installed file as `<name>.v<old>.bak` and then writes the bundled file. An installed profile with the same or a newer version is left alone. The app never touches `trackmap.json`, so your labels stay. If you edited one of the three files, copy your changes back from the `.bak` copy by hand. To go back to the old profile, restore the `.bak` files.

## Orders

The dashboard shows the cars on the route, what is ready to pull, and the orders you can start.

A local job order belongs to one local, such as AB45. It has one line for each industry that is ready and one line for each satellite yard the local serves.

- An industry line sets out the cars waiting in the local's home yard for that industry, up to the industry's capacity for each car type, and pulls the cars the industry has finished with. An industry only gets a line when none of its cars is still tagged for that industry, because Run8 is still processing those.
- A yard line drops a block at a satellite yard and brings back what other locals pulled there. A55 serves Tuscaloosa and A44 serves McCalla this way.

A road train order becomes available when the cars waiting for the train reach a share of its maximum length. The default share is 50 percent, and you can change it per train in Setup. The order lists cars oldest first and stops at the train's maximum length and tonnage. It also shows the horsepower the train needs. Some road trains work a yard or an industry on the way, such as 131 setting out autoracks at McCalla. Through trains such as 346 have no origin yard and only pick up at their stops.

### Order lifecycle

An order moves through these states.

1. Available. The app builds and rebuilds these on every new save.
2. In Progress. You start an order, or one line of a local job order. The app reserves its cars so no other order lists them.
3. Completion Detected. A later save shows the work done. Local job set-outs are at their industry or yard, the pickups are back in the home yard, and a road train's cars have left the map.
4. Completed. You confirm the detection. You can also choose Continue to keep working, and the app waits for another car to move before it checks again.

You can complete an In Progress order by hand, or cancel it to release its cars. If a car an order still expects goes missing from the save, the order shows a warning and does not complete itself. Finished orders stay in the order history.

## Files and safety

The app keeps these files in the data folder.

- `profiles/<id>/` holds the route profile, the industry configuration, the train limits and `trackmap.json`.
- `settings.json` holds the port, the Run8 install folder, the profile choice and your per-train thresholds.
- `state.json` holds started and finished orders, how long each car has sat in place, and the activity log.

The app never creates, changes, renames or deletes a file under `C:\Run8Studios\Run8 Train Simulator V3`. It refuses to start with a `--data` folder inside that directory, and it checks every write against the same rule, including paths that reach the install folder through a symbolic link.

## Releases

Every merge to `main` that holds a fix, a feature or a breaking change publishes a release on the GitHub releases page. A release has three things:

- `run8ops_v<version>_windows_amd64.zip`, which holds `run8ops.exe` and this README.
- A `.sha256` file with the checksum of the zip.
- Notes that list the changes since the last release, with breaking changes first.

To check a download, put the zip and the `.sha256` file in one folder and run `shasum -a 256 -c run8ops_v0.1.0_windows_amd64.zip.sha256`. On Windows, run `Get-FileHash` on the zip in PowerShell and compare the hash with the one in the `.sha256` file.

### How the version is chosen

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/). On each merge to `main`, the release job reads every commit since the last `v<major>.<minor>.<patch>` tag and applies the biggest change it finds.

| Commit | Before 1.0 | From 1.0 |
|---|---|---|
| Breaking: `!` after the type, or a body line that starts with `BREAKING CHANGE:` | minor | major |
| `feat` | minor | minor |
| `fix`, `perf`, `revert` | patch | patch |
| `docs`, `test`, `build`, `ci`, `chore`, `style`, `refactor` | no release | no release |

The first release is 0.1.0. A merge that holds only commits from the last row publishes nothing. The project stays below 1.0 until you push the tag `v1.0.0` yourself. That tag has no download of its own, and the next release after it uses the "From 1.0" column.

### Pull request titles

The repository uses squash merges, so the pull request title becomes the commit message on `main` and decides the version. A check on every pull request rejects a title that is not `type(scope)!: description`, where the scope and the `!` are optional and the type is one of `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`, `chore`, `style` or `revert`. For example, `feat(web): add order filters` or `fix(engine)!: change the snapshot format`.

## Development

```
go vet ./...
go test -race ./...
bash .github/scripts/next-version_test.sh
bash .github/scripts/check-title_test.sh
```

The last two commands test the release scripts in `.github/scripts/`. CI runs the Go checks on Linux and Windows and the script tests on Linux. It does this for each push to a branch with no open pull request and for each pull request into `main`.

The tests use copies of Run8 saves in `testdata/`. The app itself never reads from `testdata/`.

You can run the app without Run8 by pointing it at temporary folders that stand in for the Run8 folders:

```
mkdir -p /tmp/run8/Trains /tmp/run8/AutoSaves
cp testdata/WorldSave.xml testdata/WorldSave.info /tmp/run8/Trains/
go run ./cmd/run8ops --save WorldSave --save-dir /tmp/run8/Trains --autosave-dir /tmp/run8/AutoSaves --data /tmp/run8/data --no-browser
```

Copy a second save into `/tmp/run8/AutoSaves` as `Auto Save World 2026.10.30 19.15.00.xml`, with a `.info` file that has a later `FileTimestamp`, and the open page updates by itself.

## Known limits

- Car lengths and empty weights are standard values for each car type, because a save does not contain them. Length and tonnage checks can be a car or two off.
- A few outgoing tags in the AGS industry file have no train to carry them (`MERCP`, `MERUP`, `ROS`) and one has a typo (`ATL NOL3`). They appear on the dashboard as profile warnings and do not stop anything.
- The Mercedes Benz plant and its 395 and 394 trains are not set up.
- A road train order completes when its cars leave the map. If you park the train on the map instead, complete the order by hand.
