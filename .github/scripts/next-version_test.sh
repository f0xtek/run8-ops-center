#!/usr/bin/env bash
# Tests for next-version.sh. Each case builds a throwaway git repository.
#
#   bash .github/scripts/next-version_test.sh
set -uo pipefail

script=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/next-version.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

failures=0
cases=0

fail() {
	failures=$((failures + 1))
	printf 'FAIL %s\n  want: %s\n  got:  %s\n' "$1" "$2" "$3"
}

# repo starts an empty repository for one case and changes into it.
repo() {
	local dir
	dir=$(mktemp -d "$work/case.XXXXXX")
	cd "$dir" || exit 1
	git init -q
	git config user.name test
	git config user.email test@example.com
	git config commit.gpgsign false
	git config tag.gpgsign false
}

commit() { git commit -q --allow-empty -m "$1"; }

# check NAME WANT: compares the script's stdout with WANT.
check() {
	cases=$((cases + 1))
	local got
	got=$(bash "$script" 2>/dev/null)
	if [[ $got != "$2" ]]; then fail "$1" "$2" "$got"; fi
}

# check_notes NAME WANT: compares the notes with the short hashes masked.
check_notes() {
	cases=$((cases + 1))
	local got
	got=$(bash "$script" --notes "$3" 2>/dev/null | sed -E 's/\([0-9a-f]{7}\)/(SHA)/')
	if [[ $got != "$2" ]]; then fail "$1" "$2" "$got"; fi
}

# --- first release -----------------------------------------------------------

repo
commit "docs: only docs here"
check "no tags, docs only, still 0.1.0" "0.1.0"

repo
commit "feat!: breaking and new"
check "no tags, breaking feature, still 0.1.0" "0.1.0"

# --- patch, minor, breaking before 1.0 ---------------------------------------

repo
commit "chore: start"
git tag v0.3.1
commit "fix(engine): handle empty snapshots"
check "fix only raises patch" "0.3.2"

repo
commit "chore: start"
git tag v0.3.1
commit "fix: a fix"
commit "feat(web): a feature"
check "feature raises minor and resets patch" "0.4.0"

repo
commit "chore: start"
git tag v0.3.1
commit "feat(store)!: rename the order file"
check "bang breaking before 1.0 raises minor, not major" "0.4.0"

repo
commit "chore: start"
git tag v0.3.1
commit $'refactor(profile): split the loader\n\nBREAKING CHANGE: profile.yaml needs a version key'
check "BREAKING CHANGE footer before 1.0 raises minor" "0.4.0"

repo
commit "chore: start"
git tag v0.3.1
commit $'fix: a fix\n\nBREAKING-CHANGE: hyphenated footer'
check "BREAKING-CHANGE footer counts" "0.4.0"

repo
commit "chore: start"
git tag v0.3.1
commit $'fix: a fix\n\nThis is not a BREAKING CHANGE: it only mentions one mid-line'
check "mid-line breaking text is not a footer" "0.3.2"

repo
commit "chore: start"
git tag v0.5.0
commit "fix: a fix"
commit "feat: a feature"
commit "docs: a doc"
check "highest rule wins" "0.6.0"

repo
commit "chore: start"
git tag v0.3.1
commit "perf(web): faster render"
check "perf raises patch" "0.3.2"

repo
commit "chore: start"
git tag v0.3.1
commit 'revert: undo "feat: x"'
check "revert raises patch" "0.3.2"

# --- from 1.0 ----------------------------------------------------------------

repo
commit "chore: start"
git tag v1.2.3
commit "feat!: breaking"
check "breaking after 1.0 raises major" "2.0.0"

repo
commit "chore: start"
git tag v1.2.3
commit "feat: new"
check "feature after 1.0 raises minor" "1.3.0"

repo
commit "chore: start"
git tag v1.2.3
commit "fix: bug"
check "fix after 1.0 raises patch" "1.2.4"

# --- nothing releasable ------------------------------------------------------

repo
commit "chore: start"
git tag v0.2.0
commit "docs: a"
commit "ci: b"
commit "chore: c"
commit "build: d"
commit "test: e"
commit "style: f"
commit "refactor: g"
check "docs, ci, chore, build, test, style, refactor release nothing" ""

repo
commit "chore: start"
git tag v0.2.0
commit "wip"
commit "Merge branch 'x'"
check "non-conventional subjects release nothing" ""

repo
commit "feat: before the tag"
git tag v0.2.0
check "no commits since the tag releases nothing" ""

# --- tags --------------------------------------------------------------------

repo
commit "chore: start"
git tag v0.9.0
commit "chore: middle"
git tag v0.10.0
git tag v0.11.0-rc1
commit "fix: after"
check "latest tag is by version, not text, and ignores pre-releases" "0.10.1"

# --- merge commits -----------------------------------------------------------

repo
commit "chore: start"
git tag v0.1.0
git checkout -q -b topic
commit "fix: inside the branch"
commit "feat: also inside the branch"
git checkout -q -
commit "docs: on main"
git merge -q --no-ff topic -m "Merge branch 'topic'"
check "commits inside a merged branch count" "0.2.0"

# --- notes -------------------------------------------------------------------

repo
commit "chore: start"
git tag v0.1.0
commit "docs: update readme"
commit "fix(engine): handle empty snapshots"
commit "feat(web): add filters"
commit "feat(store)!: rename the order file"
commit "wip"
commit $'refactor: split loader\n\nBREAKING CHANGE: needs a version key'
check_notes "notes order, scope prefix, merge noise omitted" "## v0.2.0

### Breaking changes

- split loader (SHA)
- store: rename the order file (SHA)

### Features

- web: add filters (SHA)

### Fixes

- engine: handle empty snapshots (SHA)

### Other changes

- update readme (SHA)" "0.2.0"

repo
commit "chore: start"
git tag v0.1.0
commit "fix: only a fix"
check_notes "empty sections are omitted" "## v0.1.1

### Fixes

- only a fix (SHA)" "0.1.1"

if ((failures > 0)); then
	printf '%d of %d cases failed\n' "$failures" "$cases"
	exit 1
fi
printf 'all %d cases passed\n' "$cases"
