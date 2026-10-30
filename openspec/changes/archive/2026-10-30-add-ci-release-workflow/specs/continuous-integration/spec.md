# Spec Delta

## Purpose

Checks every code change automatically before it reaches `main`, so a broken build or a failing test shows up on the branch or pull request instead of in a release.

## ADDED Requirements

### Requirement: Checks on feature branches and pull requests
The repository SHALL run the project checks, `go vet ./...` and `go test -race ./...`, on every push to a branch other than `main` and on every pull request that targets `main`. The checks SHALL run on Linux and on Windows, because the app is released for Windows and developed on other systems. A failure in either check on either system SHALL fail the run.

#### Scenario: Push to a feature branch
- **WHEN** a commit is pushed to the branch `feat/dashboard-filters`
- **THEN** the checks run on that commit and report pass or fail on the commit

#### Scenario: Pull request opened
- **WHEN** a pull request targeting `main` is opened or receives new commits
- **THEN** the checks run and their result appears on the pull request

#### Scenario: Failing test
- **WHEN** a test fails on Windows but passes on Linux
- **THEN** the run fails and names the Windows job

#### Scenario: Push to main
- **WHEN** a commit lands on `main`
- **THEN** the branch and pull request workflow does not start for it, and only the release workflow runs

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
