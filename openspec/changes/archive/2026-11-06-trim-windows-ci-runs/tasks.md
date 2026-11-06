# Tasks

## 1. Workflows

- [x] 1.1 Add the `windows` input and the per-system race flag to `tests.yml`, and pass `windows` from `ci.yml` (design D1). Verify with `actionlint` if available, or by reading the expanded matrix, that `release.yml` still gets both systems.
- [x] 1.2 Update the CI lines in `README.md` and `.omp/AGENTS.md`, using `skill://unslop` for the README. Verify that they say Windows runs on pull requests and `main`, and that Windows tests run without `-race`.

## 2. Verification

- [x] 2.1 Push the branch with no pull request and check on GitHub that the run has only the Linux test job. Verify that job names and the `gate` job are unchanged.
- [x] 2.2 Open the pull request and check that it runs both test jobs, with the Windows job running `go test ./...`. Verify with `openspec validate trim-windows-ci-runs --strict`.

## Workflow follow-up

- Archive the change after 2.2 passes.
