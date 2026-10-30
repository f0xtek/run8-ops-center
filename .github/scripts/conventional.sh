#!/usr/bin/env bash
# shellcheck disable=SC2034 # the CC_* variables are the output for the sourcing script
# Shared Conventional Commits grammar. Source this file; do not run it.
#
# The allowed types live here so the version script and the pull request title
# check agree on them.

CC_TYPES="feat fix perf refactor docs test build ci chore style revert"

# cc_parse SUBJECT
#
# Parses "type(scope)!: description". On success it returns 0 and sets
# CC_TYPE, CC_SCOPE (may be empty), CC_BREAKING (0 or 1) and CC_DESC. On
# failure it returns 1 and leaves those variables empty.
cc_parse() {
	CC_TYPE='' CC_SCOPE='' CC_BREAKING=0 CC_DESC=''
	local subject=$1
	local re='^([a-z]+)(\(([^)]+)\))?(!)?: ([^[:space:]].*)$'
	# A newline would let a later line pass the regular expression.
	case $subject in *$'\n'*) return 1 ;; esac
	[[ $subject =~ $re ]] || return 1
	case " $CC_TYPES " in
	*" ${BASH_REMATCH[1]} "*) ;;
	*) return 1 ;;
	esac
	CC_TYPE=${BASH_REMATCH[1]}
	CC_SCOPE=${BASH_REMATCH[3]}
	if [[ -n ${BASH_REMATCH[4]} ]]; then CC_BREAKING=1; fi
	CC_DESC=${BASH_REMATCH[5]}
	return 0
}

# cc_has_breaking_footer BODY
#
# Returns 0 when a line of BODY starts with "BREAKING CHANGE:" or
# "BREAKING-CHANGE:".
cc_has_breaking_footer() {
	local line
	while IFS= read -r line; do
		case $line in
		"BREAKING CHANGE:"* | "BREAKING-CHANGE:"*) return 0 ;;
		esac
	done <<<"$1"
	return 1
}
