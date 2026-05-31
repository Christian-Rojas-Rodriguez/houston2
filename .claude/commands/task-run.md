---
description: Drive a single Task through the full spec-cycle (UC-2) — researcher → curator → specter → qa → coder → reviewer → tester → pr → auditor. Use to implement one Task at a time from the Workflow.
argument-hint: <task-id>
disable-model-invocation: true
---

> Skeleton — full behavior is implemented in Task 0026.

# /task-run

Run UC-2 for Task `$ARGUMENTS`.

## Plan

1. `tl` reads `.claude/specs/tasks/<task-id>-*.md` to load the Task.
2. If the Spec is missing or incomplete, hand off to `researcher` → `curator` → `specter` to build it. The user reviews and approves before proceeding.
3. Once the Spec is materialized, hand off to `qa` to lint and author the failing test suite.
4. Hand off to `coder` to implement until tests pass.
5. Hand off to `reviewer` for a final read-only pass.
6. Hand off to `tester` to run the suite in a clean state.
7. Hand off to `pr` to open the pull request.
8. Hand off to `auditor` to verify Spec↔Code equivalence and propose a SemVer bump.
9. Return the PR URL and the bump proposal to the user.

## Operating rules

- Every handoff is logged in `tl`'s output as a `## Current step` line.
- The user is the merge approver — `auditor` only proposes the bump, it never merges.
- If any agent returns `block`, `tl` stops and surfaces the blocker; it does not auto-retry.
