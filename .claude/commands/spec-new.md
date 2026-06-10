---
description: Create a Spec skeleton at `.claude/specs/tasks/<id>-<slug>.md` for a Task that is already declared in the Workflow. Use when a Task exists in the Workflow but its Spec file has not been materialized yet.
argument-hint: <id> <slug>
disable-model-invocation: true
---

> Skeleton — full behavior is implemented in Task 0028.

# /spec-new

Create a fresh Spec skeleton for Task id `$0`, slug `$1`.

## Plan

1. `tl` confirms that Task `$0` is declared in `workflow.md`. If not, abort and recommend `/agent-new` or `/workflow-review`.
2. `tl` invokes `researcher` for per-task context.
3. `tl` invokes `curator` to polish What/Why/How.
4. `tl` invokes `specter` to render the canonical template via Skill `write-spec` at `.claude/specs/tasks/<id>-<slug>.md`, version `0.1.0`, status `in-progress`.
5. Return the file path and recommended next step (`/task-run <id>`).

## Operating rules

- The Spec starts at `0.1.0` and `status: in-progress`. It never starts at `done`.
- If the Task already has a Spec, surface the existing file path and ask if the user wants `/workflow-review` instead.
