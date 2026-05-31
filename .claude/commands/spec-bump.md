---
description: Bump a Spec's SemVer version. Normally Auditor calls this automatically via `post-merge-bump`. Use the dry-run flag to preview a bump before merge. Direct invocation by humans is allowed only for `--dry-run`.
argument-hint: <task-id> <patch|minor|major> [--dry-run]
disable-model-invocation: true
---

> Skeleton — full behavior is implemented in Task 0029.

# /spec-bump

Bump the SemVer version of the Spec for Task `$0` by level `$1`. Optional flag: `$2` (typically `--dry-run`).

## Plan

1. `tl` invokes `auditor`.
2. `auditor` invokes Skill `verify-contract` with `--check` to confirm the bump is justified.
3. If `--dry-run` flag is present, return the proposed new version without writing.
4. Otherwise, `auditor` writes the new `version` field to the Spec frontmatter.

## Operating rules

- Direct human invocation is **only** valid with `--dry-run`. Bumps without dry-run from a human are rejected — bumps go through Auditor via the `post-merge-bump` hook.
- Bump levels follow constitution §2.6 and SPEC §11:
  - **patch** — text clarification only.
  - **minor** — added acceptanceCriteria, scope expansion, fully backward compatible.
  - **major** — removed/changed criteria, What/Why change, or breaking consumers.
- The version field is the only thing that changes. Acceptance criteria, scope, etc. require a separate Spec revision.
