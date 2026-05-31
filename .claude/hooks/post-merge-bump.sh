#!/usr/bin/env bash
# post-merge-bump — git post-merge hook (skeleton, Task 0004).
# After a merge, invokes Auditor's verify-contract --bump mode to recompute the SemVer
# version of every Spec affected by the merge and commit the bump as a separate commit
# (`chore(spec): bump <task-id> to <version>`).
#
# Hook input: none (reads `git log -1 --name-only`).
# Hook output: exit 0 always (side effect, never blocks).

set -u
REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
cd "$REPO_ROOT" || exit 0

# TODO(0004): full implementation.
# Skeleton behavior: print a notice and exit. Real behavior delegates to verify-contract:
#
#   node packages/cli/dist/index.js audit --bump \
#     --since HEAD~1 \
#     --specs .claude/specs/tasks/ \
#   && git add .claude/specs/tasks/ \
#   && git commit -m "chore(spec): bump versions after merge $(git rev-parse --short HEAD)"
#
# Until verify-contract exists, this hook stays a no-op.
printf "[post-merge-bump] skeleton — Task 0004 will bump Spec.version on merge.\n" >&2
exit 0
