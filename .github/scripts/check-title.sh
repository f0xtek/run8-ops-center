#!/usr/bin/env bash
# Checks that a pull request title is a Conventional Commit subject.
#
#   check-title.sh "feat(web): add order filters"
#
# Exits 0 for a valid title and 1 with a one-line message otherwise. The title
# is only ever read as data, never evaluated.
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=conventional.sh
. "$here/conventional.sh"

title=${1-}
if cc_parse "$title"; then
	exit 0
fi
echo "Pull request title must follow Conventional Commits, type(scope)!: description, with type one of: ${CC_TYPES// /, }." >&2
exit 1
