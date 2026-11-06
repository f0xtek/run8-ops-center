## Why

The Windows test job takes 100 to 220 seconds, against about 70 for Linux, and it runs on every branch push as well as on pull requests and `main`. Most of that time repeats what the pull request run covers, and the race detector accounts for much of the Windows cost. Race detection doesn't depend on the OS, so Linux can keep it.

## What Changes

- Plain branch pushes with no open pull request run the checks on Linux only.
- Pull request runs and the `main` release run keep Windows, so the merge gate and the release gate are unchanged.
- The Windows job runs `go test ./...` without `-race`. Linux keeps `go test -race ./...`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `continuous-integration`: the checks requirement changes which systems run on which events, and where the race detector runs.

## Impact

- `.github/workflows/tests.yml` takes a `windows` input and sets the race flag per system. `ci.yml` passes it. `release.yml` is unchanged and keeps the default, which includes Windows.
- `README.md` and `.omp/AGENTS.md` describe where CI runs Windows.
- Job names (`test (ubuntu-latest)`, `test (windows-latest)`) stay as they are, so required checks on pull requests keep matching.
- A Windows-only failure now shows when the pull request opens, not on the first branch push.
