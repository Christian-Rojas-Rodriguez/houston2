---
description: Verify the spec-as-source contract — every changed path traces to a Spec scope, every acceptance criterion has a passing test, no human-edited code without a Spec diff. Drives the `pre-commit-contract` hook and the Auditor's PR gate, and bumps Spec versions at post-merge time.
disable-model-invocation: true
allowed-tools: Read, Bash(git *)
---

> Skeleton — full behavior is implemented in Task 0023.

# verify-contract

The single source of truth for spec-as-source invariants. Invoked by `pre-commit-contract` (block-style check) and by Auditor (PR gate + bump).

## Modes

- `--check` — read-only. Validate the current diff (`HEAD..` staged or PR base..head) against active Specs. Exit `0` on pass, `2` on block. Print findings to stderr.
- `--bump` — write-style. After a successful merge, compute the SemVer bump for each affected Spec and rewrite its `version` field. Exit `0` always; failures go to stderr.

## Inputs

- The active diff (provided by the caller as either a list of paths or by invoking `git diff` internally).
- The set of Specs under `.claude/specs/tasks/`.

## Checks (in `--check` mode)

1. **Scope check** — every changed code path is inside the `scope` of at least one Spec whose status is `in-progress` or `done`.
2. **Coverage check** — every `acceptanceCriteria` entry of every touched Spec has a corresponding test under `tests/<level>/<task-id>__*.test.*`.
3. **Test-result check** — when invoked from `pre-commit-contract`, run the test suite for affected Tasks and fail if any test fails.
4. **Constitution untouched** — `.claude/specs/constitution.md` was not modified unless this is an explicit constitution-revision commit (detected by commit message tag `[constitution]`).

## Bump policy (in `--bump` mode)

| Change | Bump |
|---|---|
| Textual clarification, no acceptanceCriteria change | `patch` |
| Added acceptanceCriteria, scope expanded, fully backward-compatible | `minor` |
| Removed/changed acceptanceCriteria, What/Why changed, breaking | `major` |

Output: a JSON object per affected Spec, e.g. `{ "id": "0042", "old": "0.3.1", "new": "0.4.0", "reason": "added 2 criteria" }`.

## Operating rules

- Never modify code or tests. Bumps modify only `version` in Spec frontmatter.
- Only the Auditor (via this Skill) is allowed to call `--bump`. Humans never bump.
- Failures in `--check` mode must produce actionable messages: which path, which Spec, which criterion, what to do.
