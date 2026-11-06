## MODIFIED Requirements

### Requirement: Checks on feature branches and pull requests
The repository SHALL run the project checks on every push to a branch other than `main` and on every pull request that targets `main`. The checks are `go vet ./...` and the Go tests. The pull request run SHALL check on Linux and on Windows, because the app is released for Windows and developed on other systems. A branch push with no open pull request SHALL check on Linux only. The Linux tests SHALL use the race detector, and the Windows tests SHALL NOT. A failure in any check on any system that ran SHALL fail the run.

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
