#!/usr/bin/env bash
# permissions-guard — SessionStart hook (skeleton, Task 0001).
# Walks .claude/{agents,skills,commands} and warns on least-privilege violations.
# Never blocks: prints warnings to stderr and exits 0.
#
# Hook input (stdin, JSON): { "session_id": "...", "cwd": "..." }
# Hook output: stderr warnings; exit 0.

set -u
INPUT=$(cat)
CLAUDE_DIR="${CLAUDE_PROJECT_DIR:-$(pwd)}/.claude"

if [ ! -d "$CLAUDE_DIR" ]; then
  exit 0
fi

# TODO(0001): inspect frontmatter of agents/, skills/, commands/ and flag:
#   - agents declaring tools beyond their declared objective
#   - skills with effects but missing disable-model-invocation: true
#   - hooks registered in settings.json without a matching script file
# Skeleton: print a single boot message and exit.
printf "[permissions-guard] skeleton — Task 0001 will implement full audit.\n" >&2
exit 0
