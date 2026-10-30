# Design

## Context

- The repo has no `.github/` directory. `origin` is `git@github.com:f0xtek/run8-ops-center.git`, and the only branch is `main`. There are no tags.
- `go.mod` declares `go 1.26`, and the module has three dependencies. `go vet ./... && go test -race ./...` passes on macOS in about 25 seconds. The Windows run is unverified.
- `-race` needs cgo, which GitHub's Linux and Windows runners provide through gcc.
- `cmd/run8ops/main.go` parses flags with `flag.NewFlagSet` and returns an exit code from `run`. It has no version flag. `main_test.go` tests `run` directly.
- The build command today is `GOOS=windows GOARCH=amd64 go build -o run8ops.exe ./cmd/run8ops`, and the app is pure Go, so cross-building from Linux works. `.gitattributes` keeps `testdata/` and `internal/profile/bundled/` byte-exact, which matters on Windows checkouts.
- The user merges with squash or ordinary merges. [ASSUMPTION] Squash is the plan, so the pull request title becomes the commit subject on `main`.

## Goals / Non-Goals

**Goals:**
- Feature branches and pull requests get a pass or fail from the same commands a developer runs locally.
- A merge to `main` produces a release with no manual step, and the version follows from the commit history.
- The version rules are testable without publishing anything.

**Non-Goals:**
- Moving from 0.x to 1.0.0. The script follows the 1.x rules once a `v1.*` tag exists, but nothing creates that tag. The user pushes it by hand when the time comes, and a later change can add a dispatch input if wanted.
- Code signing, installers, macOS or Linux builds, and publishing outside GitHub releases.
- A `CHANGELOG.md` file. The release notes are the changelog.
- Linting beyond `go vet`, coverage reports, and dependency update automation.
- Branch protection and merge settings as code. They are manual repository settings.

## Decisions

### Two workflows, not one

`ci.yml` triggers on `push` to every branch except `main` (`branches-ignore: [main]`) and on `pull_request` targeting `main`. `release.yml` triggers on `push` to `main` only. Permissions differ: CI needs `contents: read`, release needs `contents: write`. Separate files keep the write token away from branch code.

Alternative: one workflow with conditional jobs. Rejected because the `if:` conditions make the permissions harder to read.

### One run per push

A push to a branch with an open pull request fires both the `push` and the `pull_request` event, so without a guard each push would run the checks twice. A per-ref `concurrency` group does not merge them, because the refs differ (`refs/heads/x` against `refs/pull/N/merge`). Workflow-level path or branch filters cannot see whether a pull request exists.

`ci.yml` therefore starts with a `gate` job that runs for both events and outputs `run`:

- On `pull_request`, `run` is `true`.
- On `push`, the gate asks GitHub for open pull requests from the pushed branch into `main`: `gh pr list --repo "$GITHUB_REPOSITORY" --head "$BRANCH" --base main --state open --json isCrossRepository --jq '[.[] | select(.isCrossRepository | not)] | length'`. A count of 0 sets `run` to `true`, and anything else sets it to `false`. The cross-repository filter keeps a fork's same-named branch from suppressing the run. The `--base main` filter matters because a pull request into another branch never triggers the `pull_request` run in `ci.yml`.

`Tests`, `scripts` and `pr-title` all declare `needs: gate` and `if: needs.gate.outputs.run == 'true'`. `pr-title` also keeps its `pull_request`-only condition. The gate needs `pull-requests: read` and `contents: read`, and it sets `GH_TOKEN` from `secrets.GITHUB_TOKEN`. It passes the branch name through an `env:` entry, never by interpolating it into the script.

The pull request run wins when both could apply, because it is the one that attaches the required checks to the pull request and tests the merge result. A skipped job counts as passing for branch protection, which is harmless because the required checks come from the pull request run.

Alternative: let both runs happen and accept the cost. Rejected on request. Alternative: trigger only on `pull_request`, so a branch with no pull request gets no checks. Rejected because branches without a pull request must still be checked.

### Test matrix on Linux and Windows

The tests job runs `go vet ./...` and `go test -race ./...` on `ubuntu-latest` and `windows-latest`, with `go-version-file: go.mod`. Linux is the fast check. Windows is where the app runs, and the Run8 install guard and the `%APPDATA%` handling are Windows-shaped. If Windows exposes failures, they are real defects, and fixing them belongs to this change because the release is a Windows binary. The release workflow runs the same job through the reusable workflow `tests.yml`, so there is one definition of what "tests pass" means.

### Reusable `tests.yml`

`tests.yml` has the `workflow_call` trigger and holds the matrix job. `ci.yml` and `release.yml` each call it. A called job's check name is the caller's job name plus the called job name, so the check that branch protection requires is shaped like `Tests / test (ubuntu-latest)`. The tasks record the exact names after the first run shows them.

### Pull request title check

A shell step in `ci.yml`, on `pull_request` events `opened`, `edited`, `synchronize` and `reopened`, runs `.github/scripts/check-title.sh` against the title. The grammar is small and fixed, and the script shares the type list with the version script through `.github/scripts/conventional.sh`, so the allowed types live in one place. A third-party action such as `amannn/action-semantic-pull-request` would add supply-chain exposure for about ten lines of regular expression. The step passes the title in an environment variable, never by interpolating it into the command, so a crafted title cannot inject shell.

### Version computed by a script in the repo

`.github/scripts/next-version.sh` is bash. It runs on the Ubuntu runner and on a developer's Mac.

Input: the git history of the current checkout. Output on stdout: the next version without `v`, or nothing when no release is due. Exit code 0 in both cases. A `--notes <version>` mode prints the release notes for the range. Steps:

1. Find the latest tag matching `v[0-9]*.[0-9]*.[0-9]*` by version sort (`git tag --list 'v*' --sort=-v:refname`), ignoring pre-release suffixes. No tag means print `0.1.0` and stop.
2. Read `git log <tag>..HEAD --format=%H%x1f%s%x1f%b%x1e`. This includes commits inside merged branches and does not rely on first-parent history, so both squash and merge-commit workflows work.
3. Classify each commit. The subject regex is `^(feat|fix|perf|revert|...)(\([^)]+\))?(!)?: .+`. A commit is breaking when `!` is present or a body line starts with `BREAKING CHANGE:` or `BREAKING-CHANGE:`. A commit whose subject does not match is treated as type `other` for versioning, but a breaking footer on it still counts.
4. Pick the level: breaking, then `feat`, then `fix`, `perf` and `revert`. In 0.x, breaking maps to minor. In 1.x and later, breaking maps to major.
5. Bump and print.

Alternatives considered:

- `release-please`. It is well known, but it works through a long-lived release pull request. The user asked for a release when a change merges to `main`, and release-please releases when its pull request merges. Its pre-1.0 defaults also differ (a feature bumps the patch number).
- `semantic-release` or `go-semantic-release`. They fit the model, but they add Node or a binary download, a config file and a plugin surface for what is a 100-line rule set.
- `mathieudutour/github-tag-action`. It computes the version and tags, but its `0.x` behaviour and the initial version need extra configuration, and it hides the rules from the repo.
- `git cliff` or `svu`. Fine tools, and an option to swap in later, but they add a download and pinned versions. A script that the repo owns and tests keeps the 0.x rules, the footer handling and the first-tag rule in one visible place.

The script has a test, `.github/scripts/next-version_test.sh`. It builds throwaway git repos in a temp directory, writes commit sequences and asserts the output for each scenario in the spec. `ci.yml` runs it on Ubuntu. It needs only `git` and `bash`.

### Release notes from the same script

`gh release create --generate-notes` groups by pull request label, not by commit type. The script's `--notes` mode prints Markdown with `### Breaking changes`, `### Features`, `### Fixes` and `### Other changes` sections, each line `- description (<short sha>)`. The pull request number stays in the description because squash commits carry `(#123)` in the subject. The release job passes this to `gh release create --notes-file`.

### Release job steps

`release.yml`, `permissions: contents: write`, `concurrency: { group: release, cancel-in-progress: false }`:

1. `tests` job calls `tests.yml`.
2. `release` job, `needs: tests`, on `ubuntu-latest`:
   1. `actions/checkout` with `fetch-depth: 0` so tags and full history exist.
   2. Run `next-version.sh`. If it prints nothing, write a job summary line explaining that no releasable commits exist, and stop with success.
   3. `actions/setup-go` with `go-version-file: go.mod`.
   4. Build with `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o dist/run8ops.exe ./cmd/run8ops`.
   5. Zip `run8ops.exe` and `README.md` into `run8ops_v$VERSION_windows_amd64.zip`, and write `sha256sum` output to a `.sha256` file next to it.
   6. `gh release create "v$VERSION" --target "$GITHUB_SHA" --title "v$VERSION" --notes-file notes.md dist/*.zip dist/*.sha256`. `gh` creates the tag as part of the release, so a failed upload leaves no tag when the release call fails before creation. If the release is created and an asset upload fails, the run fails and the maintainer deletes the release and tag by hand. That is the rare case, and the script's `v*` lookup would otherwise see a half-made release.

Concurrency with `cancel-in-progress: false` queues a second run until the first finishes, so the second computes from the new tag. GitHub keeps only one pending run per group and drops older pending ones. When three pushes arrive in a row, the middle one is dropped, but the last run still computes from the full history since the last tag, so no commit is lost from the version or the notes.

Because the run builds `$GITHUB_SHA`, and `actions/checkout` fetches that exact commit, a later push cannot change what is released. If `main` has moved on by the time the run starts, the run still tags its own commit. The notes and version cover the commits up to that commit only, and the later push gets its own release.

Tag and release creation uses the built-in `GITHUB_TOKEN`. Tags it creates do not start other workflows, and nothing else listens for tags, so that is fine.

### Version in the binary

`cmd/run8ops/main.go` gets `var version = "dev"` and a `--version` bool flag. When set, `run` prints `run8ops <version>\n` to stdout and returns 0 before any other flag checks, so it works without `--save`. The usage text gains one line. The flag is evaluated right after `fs.Parse`. `-ldflags -X main.version=...` stamps the release version.

Alternative: derive the version from `debug.ReadBuildInfo`. For a `go build` in a git checkout it yields a pseudo-version, but the release job needs an exact tag version, and `-X` is explicit.

### Windows line endings and test data

`.gitattributes` already marks `testdata/` and `internal/profile/bundled/` as binary. Windows runners check out with `core.autocrlf` set by the runner image, and `-text` attributes override it. The tasks include running the Windows leg once before relying on it.

### Required settings outside the repo

Squash merge with "default to pull request title" for the commit message, and the test checks required on `main`. Neither is in the repo. They go in the tasks as manual steps with the exact setting names. The release workflow still runs the tests itself, so a missing branch protection rule cannot ship a failing build.

## Risks / Trade-offs

- **Windows leg finds failures.** Tests were only run on macOS. Fix them as part of this change. If a fix is large, stop and raise it, and do not drop the leg silently.
- **Merge commit vs squash.** With merge commits the script reads the inner commits, so versions are right, but their subjects need to follow Conventional Commits. The title check does not cover them. Squash is the documented path.
- **Bad title on a squashed pull request.** A `feat` merged as `Update stuff` produces no release. The title check blocks this on the pull request, but a direct push to `main` bypasses it.
- **Gate race on a new pull request.** A push that lands in the seconds before a pull request is opened sees no pull request and runs on its own, and the pull request run follows when it opens. That costs one extra run, never a missing one. A failed `gh` call fails the gate and so fails the run, which is visible and not a silent skip.
- **Release race.** Covered by the concurrency group. A cancelled or failed release run leaves commits unreleased until the next releasable merge, and the next run includes them.
- **Pre-1.0 rule choice.** A `feat` raises the minor number and a `fix` raises the patch number, and breaking changes also raise the minor number. This follows common 0.x practice. Changing it later means editing one function and its tests.
- **Half-made release.** If asset upload fails after the release exists, the next run sees the new tag and does not retry. Manual cleanup is the documented recovery.
- **Bash on Windows developers' machines.** The script tests run in CI on Ubuntu. A developer on Windows needs Git Bash to run them locally. Acceptable because CI is the check.

## Open Questions

None that change what gets built. The 0.x bump rules, squash merging and the Linux plus Windows matrix are decisions above, and the user can change them at review.
