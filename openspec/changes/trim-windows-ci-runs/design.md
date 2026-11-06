# Design

## Context

- `tests.yml` is a reusable workflow with a fixed matrix of `ubuntu-latest` and `windows-latest`. Both run `go vet ./...` and `go test -race ./...`. `ci.yml` and `release.yml` call it.
- Recent runs: Linux 67 to 74 s, Windows 99 to 217 s. The repo is public, so runner minutes are free; the cost is waiting.
- Windows-specific code (`store.Guard` case folding, the address-in-use error code, `rundll32`, `fsnotify`'s Windows backend, path handling) is exercised by ordinary tests, not by the race detector.

## Goals / Non-Goals

**Goals:**
- Faster feedback on plain branch pushes.
- Keep Windows on the pull request and release gates.

**Non-Goals:**
- Dropping Windows. Two of the three failed runs so far failed on Windows only.
- Dropping `-race` on Linux.
- Changing the gate job or the one-run-per-push rule.

## Decisions

### D1. A `windows` input on the reusable workflow
- `tests.yml` gains `on.workflow_call.inputs.windows` (boolean, default `true`). The matrix `include` list comes from `fromJSON`: Linux with `-race` always, Windows without a flag when `inputs.windows` is true.
- `ci.yml` passes `windows: ${{ github.event_name == 'pull_request' }}`. The gate already makes a push run happen only when no pull request covers the branch, so a branch push with Windows off is exactly the "plain branch push" case.
- `release.yml` passes nothing and keeps Windows.
- **Alternatives considered:**
  - A separate Windows workflow for pull requests. Rejected: it duplicates the steps and splits "tests pass" across two files.
  - Path filters to skip Windows for docs-only changes. Rejected: it adds rules to maintain for a small gain.

### D2. Job names stay the same
- The job is still `test (${{ matrix.os }})`, so a required check named `Tests / test (windows-latest)` on pull requests keeps matching. The Windows job is simply absent from branch pushes, where it isn't required.

## Risks / Trade-offs

- **[A Windows-only failure appears later, when the pull request opens]** → The pull request run still blocks the merge, and `main` runs Windows again before a release.
- **[A data race that only occurs on Windows goes unseen]** → Accepted. The races the detector finds are in Go code that doesn't branch on the OS.

## Migration Plan

- Merge the workflow change. No setup is needed. To undo it, set `windows: true` for every event and restore `-race` on Windows.
