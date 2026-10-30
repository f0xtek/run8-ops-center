#!/usr/bin/env bash
# Tests for check-title.sh.
#
#   bash .github/scripts/check-title_test.sh
set -uo pipefail

script=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/check-title.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

failures=0
cases=0

# expect VALID TITLE: VALID is "valid" or "invalid".
expect() {
	cases=$((cases + 1))
	local want=$1 title=$2 code=0
	bash "$script" "$title" >/dev/null 2>&1 || code=$?
	if [[ $want == valid && $code -ne 0 ]] || [[ $want == invalid && $code -ne 1 ]]; then
		failures=$((failures + 1))
		printf 'FAIL want %s, exit %d, title %q\n' "$want" "$code" "$title"
	fi
}

for type in feat fix perf refactor docs test build ci chore style revert; do
	expect valid "$type: x"
done
expect valid "fix(web): x"
expect valid "feat(store)!: x"
expect valid "feat!: x"
expect valid "feat(web): add order filters (#12)"

expect invalid "Update stuff"
expect invalid "feat:x"
expect invalid "feature: x"
expect invalid "Feat: x"
expect invalid ""
expect invalid "feat: "
expect invalid "feat:   "
expect invalid "feat(): x"
expect invalid "feat(web)!x"
expect invalid $'feat: x\nUpdate stuff'
expect invalid $'Update stuff\nfeat: x'

# A title is data. Shell syntax in it is a valid description and must not run.
cases=$((cases + 1))
marker=$work/marker
bash "$script" "fix: \$(touch $marker) \`touch $marker\`" >/dev/null 2>&1
if [[ -e $marker ]]; then
	failures=$((failures + 1))
	echo "FAIL a command inside the title ran"
fi
expect valid "fix: \$(id) \`id\`"

# A missing argument is an empty title.
cases=$((cases + 1))
code=0
bash "$script" >/dev/null 2>&1 || code=$?
if [[ $code -ne 1 ]]; then
	failures=$((failures + 1))
	echo "FAIL no argument exit $code, want 1"
fi

if ((failures > 0)); then
	printf '%d of %d cases failed\n' "$failures" "$cases"
	exit 1
fi
printf 'all %d cases passed\n' "$cases"
