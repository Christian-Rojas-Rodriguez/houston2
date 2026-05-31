#!/usr/bin/env bash
# pre-spec-validate — PreToolUse hook on Write under .claude/specs/tasks/** (skeleton, Task 0002).
# Rejects Spec writes that miss What / Why / How / acceptanceCriteria, or that declare
# a component not present in Workflow.declared*.
#
# Hook input (stdin, JSON): { "tool_input": { "file_path": "...", "content": "..." }, ... }
# Hook output: exit 0 to allow, exit 2 to block (with reason on stderr).

set -u
INPUT=$(cat)

# Extract fields without jq if jq is absent.
extract_json_field() {
  # $1 = key
  printf "%s" "$INPUT" | python3 -c "import sys, json; d=json.load(sys.stdin); print(d.get('tool_input', {}).get('$1', ''))" 2>/dev/null
}

FILE_PATH=$(extract_json_field file_path)
CONTENT=$(extract_json_field content)

case "$FILE_PATH" in
  *".claude/specs/tasks/"*".md") ;;
  *)
    # Not a Spec write — pass through.
    exit 0
    ;;
esac

# TODO(0002): full implementation.
# Skeleton behavior: warn that the hook is a stub and allow the write.
# Full behavior must:
#   1. Reject if any of `## What`, `## Why`, `## How`, `## Acceptance criteria` missing.
#   2. Reject if frontmatter lacks task / slug / granularity / version / status / scope.
#   3. Reject if the declared component (agent/skill/hook/command) is not declared in Workflow.
printf "[pre-spec-validate] skeleton — Task 0002 will enforce What/Why/How/criteria.\n" >&2
exit 0
