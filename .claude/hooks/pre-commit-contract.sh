#!/usr/bin/env bash
# pre-commit-contract — git pre-commit hook (skeleton, Task 0003).
# Delegates to the `verify-contract` skill in --check mode. Rejects any commit whose
# diff touches paths outside the scope of all active Specs, or that leaves an
# acceptance criterion without a corresponding test.
#
# Hook input: none (reads `git diff --cached --name-only`).
# Hook output: exit 0 on pass, exit 2 on block (with reason on stderr).

set -u
REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
cd "$REPO_ROOT" || exit 0

CHANGED=$(git diff --cached --name-only --diff-filter=ACMRTUXB)
if [ -z "$CHANGED" ]; then
  exit 0
fi

# TODO(0003): full implementation.
# Skeleton behavior: print a notice and allow. Real behavior delegates to verify-contract:
#
#   node packages/cli/dist/index.js audit --check \
#     --diff <(git diff --cached) \
#     --specs .claude/specs/tasks/ \
#   || exit 2
#
# Until verify-contract exists, this hook stays advisory.
printf "[pre-commit-contract] skeleton — Task 0003 will enforce scope+coverage.\n" >&2
printf "[pre-commit-contract] %d file(s) changed.\n" "$(printf "%s" "$CHANGED" | wc -l | tr -d ' ')" >&2
exit 0
