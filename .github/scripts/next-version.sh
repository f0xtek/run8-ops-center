#!/usr/bin/env bash
# Works out the next release version from the Conventional Commits since the
# latest v<major>.<minor>.<patch> tag.
#
#   next-version.sh                 print the next version without the "v", or
#                                   nothing when no release is due
#   next-version.sh --notes <ver>   print release notes for the same commits
#
# The reason for "no release" goes to stderr. Run it inside a git checkout that
# has the tags and full history (fetch-depth: 0 in Actions).
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=conventional.sh
. "$here/conventional.sh"

mode=version
notes_version=
case "${1-}" in
"") ;;
--notes)
	mode=notes
	notes_version=${2:?usage: next-version.sh [--notes <version>]}
	;;
*)
	echo "usage: next-version.sh [--notes <version>]" >&2
	exit 2
	;;
esac

# Pre-release tags such as v0.2.0-rc1 are not release tags.
last_tag=$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname |
	grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -n 1 || true)

if [[ $mode == version && -z $last_tag ]]; then
	echo "0.1.0"
	exit 0
fi

range=HEAD
if [[ -n $last_tag ]]; then range="$last_tag..HEAD"; fi

# level: 0 nothing, 1 patch, 2 feature, 3 breaking.
level=0
notes_breaking='' notes_features='' notes_fixes='' notes_other=''

log=$(git log --format='%H%x1f%s%x1f%b%x1e' "$range")
while IFS= read -r -d $'\x1e' record || [[ -n $record ]]; do
	record=${record#$'\n'}
	if [[ -z ${record//[$'\n']/} ]]; then continue; fi
	sha=${record%%$'\x1f'*}
	rest=${record#*$'\x1f'}
	subject=${rest%%$'\x1f'*}
	body=${rest#*$'\x1f'}

	type=
	breaking=0
	desc=
	if cc_parse "$subject"; then
		type=$CC_TYPE
		breaking=$CC_BREAKING
		desc=${CC_SCOPE:+$CC_SCOPE: }$CC_DESC
	fi
	if cc_has_breaking_footer "$body"; then
		breaking=1
		if [[ -z $desc ]]; then desc=$subject; fi
	fi

	commit_level=0
	if ((breaking)); then
		commit_level=3
	else
		case $type in
		feat) commit_level=2 ;;
		fix | perf | revert) commit_level=1 ;;
		esac
	fi
	if ((commit_level > level)); then level=$commit_level; fi

	# Commits that are not Conventional Commits, such as merge commits, stay
	# out of the notes unless they carry a breaking footer.
	if [[ -z $desc ]]; then continue; fi
	line="- $desc (${sha:0:7})"$'\n'
	if ((breaking)); then
		notes_breaking+=$line
	else
		case $type in
		feat) notes_features+=$line ;;
		fix | perf | revert) notes_fixes+=$line ;;
		*) notes_other+=$line ;;
		esac
	fi
done <<<"$log"

if [[ $mode == notes ]]; then
	printf '## v%s\n' "$notes_version"
	section() {
		if [[ -n $2 ]]; then printf '\n### %s\n\n%s' "$1" "$2"; fi
	}
	section "Breaking changes" "$notes_breaking"
	section "Features" "$notes_features"
	section "Fixes" "$notes_fixes"
	section "Other changes" "$notes_other"
	exit 0
fi

if ((level == 0)); then
	echo "no releasable commits since $last_tag" >&2
	exit 0
fi

IFS=. read -r major minor patch <<<"${last_tag#v}"
if ((level == 3 && major >= 1)); then
	major=$((major + 1)) minor=0 patch=0
elif ((level >= 2)); then
	# Before 1.0 a breaking change raises the minor number, like a feature.
	minor=$((minor + 1)) patch=0
else
	patch=$((patch + 1))
fi
echo "$major.$minor.$patch"
