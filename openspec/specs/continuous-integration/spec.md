# continuous-integration Specification

## Purpose

Checks every code change automatically before it reaches `main`, so a broken build or a failing test shows up on the branch or pull request instead of in a release.

## Requirements

### Requirement: Checks on feature branches and pull requests
The repository SHALL run `go vet ./...` and the Go tests on every push to a branch other than `main` and on every pull request that targets `main`. A pull request run SHALL check on Linux and Windows, because the app is released for Windows. A branch push with no open pull request SHALL check on Linux only. Linux SHALL use the race detector and Windows SHALL NOT. A failure on any system that ran SHALL fail the run.

#### Scenario: Push to a feature branch
- **WHEN** a commit is pushed to the branch `feat/dashboard-filters` and no pull request is open for it
- **THEN** the checks run on Linux for that commit and report pass or fail on the commit, and no Windows job runs

#### Scenario: Pull request opened
- **WHEN** a pull request targeting `main` is opened or receives new commits
- **THEN** the checks run on Linux and Windows and their result appears on the pull request

#### Scenario: Failing test
- **WHEN** a test fails on Windows but passes on Linux
- **THEN** the pull request run fails and names the Windows job

#### Scenario: Race detector
- **WHEN** the checks run
- **THEN** the Linux job runs `go test -race ./...` and the Windows job runs `go test ./...`

#### Scenario: Push to main
- **WHEN** a commit lands on `main`
- **THEN** the branch and pull request workflow does not start for it, and only the release workflow runs, which tests on Linux and Windows before it publishes

### Requirement: One run per push
A push SHALL start at most one copy of the checks. When the pushed branch has an open pull request targeting `main` from the same repository, the checks SHALL run as part of the pull request, and the branch push SHALL NOT run them again. When the branch has no such pull request, the checks SHALL run for the branch alone. Closing a pull request without merging it SHALL return the branch to running its own checks.

#### Scenario: Push with no pull request
- **WHEN** a commit is pushed to `feat/dashboard-filters` and no pull request is open for that branch
- **THEN** one copy of the checks runs, attached to the commit

#### Scenario: Push with an open pull request
- **WHEN** a commit is pushed to `feat/dashboard-filters` and an open pull request from that branch targets `main`
- **THEN** one copy of the checks runs, attached to the pull request, and the branch push shows no check jobs that ran

#### Scenario: Pull request opened after a push
- **WHEN** a pull request is opened for a branch whose latest commit already has a passing branch run
- **THEN** the pull request gets its own run, so its required checks report on the pull request

#### Scenario: Pull request to another branch
- **WHEN** a commit is pushed to a branch whose only open pull request targets a branch other than `main`
- **THEN** the branch run happens, because no pull request run will cover it

#### Scenario: Pull request closed
- **WHEN** the pull request for a branch is closed without merging and a new commit is pushed to the branch
- **THEN** the checks run for the branch alone

### Requirement: Stale runs are cancelled
When a new commit arrives on a branch or pull request while a check run for an earlier commit on the same ref is still going, the earlier run SHALL be cancelled.

#### Scenario: Two quick pushes
- **WHEN** a second commit is pushed to a branch while the first commit's checks are running
- **THEN** the first run is cancelled and the second run completes

### Requirement: Pull request titles follow Conventional Commits
Every pull request that targets `main` SHALL have a title in Conventional Commits form, `type(scope)!: description`, where scope and `!` are optional. The allowed types SHALL be `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`, `chore`, `style` and `revert`. A title that does not match SHALL fail a check on the pull request.

#### Scenario: Valid title
- **WHEN** a pull request is titled `feat(web): add order filters`
- **THEN** the title check passes

#### Scenario: Breaking title
- **WHEN** a pull request is titled `feat(store)!: rename the order file`
- **THEN** the title check passes

#### Scenario: Invalid title
- **WHEN** a pull request is titled `Update stuff`
- **THEN** the title check fails and says the title must follow Conventional Commits

#### Scenario: Title edited
- **WHEN** the author changes an invalid title to a valid one
- **THEN** the title check runs again and passes

### Requirement: Checks never touch the Run8 install
The workflows SHALL NOT read or write the Run8 install directory. Tests SHALL run only against `testdata/` and temporary directories.

#### Scenario: Runner has no Run8
- **WHEN** the checks run on a GitHub runner
- **THEN** they pass without a Run8 install present
