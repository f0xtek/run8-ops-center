# Proposal

## Why

The repository has no automation. Nobody runs `go vet` or `go test -race` before a merge, and the only way to get a Windows binary is to build it by hand. The app ships as a single `windows/amd64` executable for a user who plays Run8 on Windows, so every merge to `main` should produce a versioned, downloadable build without manual steps. The project already uses Conventional Commits, so the commit history can decide each version number.

## What Changes

- Add a GitHub Actions workflow that runs `go vet ./...` and `go test -race ./...` on Linux and Windows for every push to a branch other than `main` and for every pull request that targets `main`.
- Run one copy of the checks per push. When the pushed branch has an open pull request into `main`, only the pull request run executes. When it has none, the branch run executes.
- Add a check that a pull request title follows Conventional Commits. Squash merges use the title as the commit subject on `main`, and the release version depends on it.
- Add a GitHub Actions workflow that runs on every push to `main`. It reruns the tests, works out the next version from the Conventional Commits since the last release tag, builds `windows/amd64`, tags the commit and publishes a GitHub release with a zip, a checksum and notes grouped by commit type.
- Number versions from `0.1.0`. While the major version is 0, a breaking change raises the minor number, a feature raises the minor number and a fix raises the patch number. From `1.0.0` onward, a breaking change raises the major number.
- Skip the release when the commits since the last tag contain nothing releasable, such as only `docs`, `ci`, `chore`, `build`, `test` or `refactor` commits.
- Add scripts under `.github/scripts/`. One computes the next version and the release notes from git history, and one checks a pull request title. Each has a test that builds throwaway git repositories, so the rules are checked without running a release.
- Add a `--version` flag to the binary and stamp the release version into it at build time. Local builds report `dev`.
- Document the commit types, the version rules and the release process in the README, and add the release facts to `.omp/AGENTS.md`. The README text goes through the `unslop` skill.

## Capabilities

### New Capabilities

- `continuous-integration`: what runs automatically on feature branches and pull requests, and what blocks a merge.
- `release-automation`: how merging to `main` becomes a tagged, versioned Windows release, including the version rules and what the binary reports about its own version.

### Modified Capabilities

None. `openspec/specs/` is empty. The active change `add-run8-ops-center` has not been archived, and this change does not alter its specs.

## Impact

- New files under `.github/workflows/` and `.github/scripts/`.
- `cmd/run8ops/main.go` gains a `version` variable and a `--version` flag. `main_test.go` gains a test for the flag.
- `README.md` and `.omp/AGENTS.md` change.
- GitHub repository settings change outside the repo. Squash merging must use the pull request title as the commit message, and the test checks should be required on `main`. Neither is a file in the repo, so the tasks list them as manual steps.
- No new Go dependencies. The workflows use first-party GitHub actions and the preinstalled `gh` CLI.
- The workflows only read and build. They never touch the Run8 install directory.
