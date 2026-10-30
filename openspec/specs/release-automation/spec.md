# release-automation Specification

## Purpose

Turns every releasable merge to `main` into a tagged, versioned Windows build on the repository's releases page, with the version number taken from Conventional Commits.

## Requirements

### Requirement: Release on merge to main
On every push to `main`, the system SHALL run the project checks on the pushed commit. When they pass and the commits since the last release are releasable, the system SHALL create a tag for the new version on the pushed commit and publish a GitHub release for that tag. When the checks fail, no tag or release SHALL be created.

#### Scenario: Feature merged
- **WHEN** a pull request titled `feat(web): add order filters` is squash merged and the last release is `v0.1.0`
- **THEN** the checks pass, the tag `v0.2.0` is created on the merge commit, and a release named `v0.2.0` is published

#### Scenario: Checks fail on main
- **WHEN** the checks fail on the pushed commit
- **THEN** no tag and no release are created, and the run fails

#### Scenario: Nothing releasable
- **WHEN** the only commits since the last release have types `docs`, `ci`, `chore`, `build`, `test`, `style` or `refactor`
- **THEN** the run succeeds, creates no tag and no release, and says why

#### Scenario: Two merges in quick succession
- **WHEN** a second push to `main` arrives while the first release run is still going
- **THEN** the second run waits for the first to finish and then computes its version from the tag the first created

### Requirement: First release is 0.1.0
When the repository has no version tag, the next version SHALL be `0.1.0` regardless of the commit types in the history.

#### Scenario: No tags yet
- **WHEN** the first push to `main` after this change arrives and no `v*` tag exists
- **THEN** the release is `v0.1.0`

### Requirement: Breaking changes
A commit SHALL count as breaking when its subject has `!` before the colon, or its body has a line starting with `BREAKING CHANGE:` or `BREAKING-CHANGE:`. The footer SHALL count whatever the commit type.

#### Scenario: Breaking footer
- **WHEN** a commit has the subject `refactor(profile): split the loader` and a body line `BREAKING CHANGE: profile.yaml needs a version key`
- **THEN** it counts as breaking

#### Scenario: Breaking subject
- **WHEN** a commit has the subject `feat(store)!: rename the order file`
- **THEN** it counts as breaking

### Requirement: Version number from Conventional Commits
The system SHALL read every commit since the latest `v<major>.<minor>.<patch>` tag, including the commits inside a merged branch, and apply the highest-ranking rule that matches. A `fix`, `perf` or `revert` commit SHALL raise the patch number. Other types, and subjects that are not in Conventional Commits form, SHALL NOT raise the version.

#### Scenario: Fix only
- **WHEN** the last tag is `v0.3.1` and the only releasable commit is `fix(engine): handle empty snapshots`
- **THEN** the next version is `0.3.2`

#### Scenario: Non-conventional commit
- **WHEN** a commit subject is `Merge branch 'x'` or `wip`
- **THEN** it is ignored when choosing the version, and the commits inside the merged branch still count

### Requirement: Bump rules while the major version is 0
While the major version is 0, a breaking commit or a `feat` commit SHALL raise the minor number and reset the patch number. A breaking commit SHALL NOT raise the major number.

#### Scenario: Feature before 1.0
- **WHEN** the last tag is `v0.3.1` and the commits include a `feat` and a `fix`
- **THEN** the next version is `0.4.0`

#### Scenario: Breaking change before 1.0
- **WHEN** the last tag is `v0.3.1` and a commit is `feat(store)!: rename the order file`
- **THEN** the next version is `0.4.0`, not `1.0.0`

#### Scenario: Highest rule wins
- **WHEN** the commits since `v0.5.0` are a `fix`, a `feat` and a `docs`
- **THEN** the next version is `0.6.0`

### Requirement: Bump rules from major version 1
From major version 1 onward, a breaking commit SHALL raise the major number and reset the minor and patch numbers. A `feat` commit SHALL raise the minor number and reset the patch number.

#### Scenario: Breaking change after 1.0
- **WHEN** the last tag is `v1.2.3` and a commit is breaking
- **THEN** the next version is `2.0.0`

#### Scenario: Feature after 1.0
- **WHEN** the last tag is `v1.2.3` and the highest commit is a `feat`
- **THEN** the next version is `1.3.0`

### Requirement: Windows build attached to the release
Each release SHALL carry a `windows/amd64` build of `run8ops` that reports the release version, packaged as a zip file named with the version. The zip SHALL contain `run8ops.exe` and the README. The release SHALL also carry a SHA-256 checksum file for the zip. The build SHALL need no files beyond the executable.

#### Scenario: Download and check
- **WHEN** a user downloads `run8ops_v0.2.0_windows_amd64.zip` and its checksum file from the `v0.2.0` release
- **THEN** the checksum matches the zip, and the zip holds `run8ops.exe` and `README.md`

#### Scenario: Binary runs on Windows
- **WHEN** the user runs `run8ops.exe --version` from the zip
- **THEN** it prints `run8ops 0.2.0` and exits with code 0

### Requirement: Release notes from commit messages
Each release SHALL have notes that list the changes since the previous release, grouped by Conventional Commit type, with breaking changes listed first and the commit or pull request reference next to each entry.

#### Scenario: Notes on a mixed release
- **WHEN** a release includes one `feat`, one `fix` and one breaking commit
- **THEN** the notes list the breaking change first, then the feature, then the fix

### Requirement: Binary reports its version
The binary SHALL accept `--version`. It SHALL print the program name and version on one line to standard output, exit with code 0, and not read or write any save or data folder. Release builds SHALL report the release version without the leading `v`. A build made without the release process SHALL report `dev`.

#### Scenario: Release build
- **WHEN** `run8ops --version` runs on the `v0.2.0` release build
- **THEN** it prints `run8ops 0.2.0`

#### Scenario: Local build
- **WHEN** `run8ops --version` runs on a binary built with plain `go build`
- **THEN** it prints `run8ops dev`

#### Scenario: Version without save
- **WHEN** `run8ops --version` runs without `--save`
- **THEN** it prints the version and exits with code 0 instead of the missing-save error
