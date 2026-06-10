---
description: Reopen the Workflow to revise granularity, add or remove Tasks, or update declared agents/skills/hooks/commands/MCPs. Use mid-project when scope or organization changes.
disable-model-invocation: true
---

> Skeleton — full behavior is implemented in Task 0025.

# /workflow-review

Run a partial UC-1 focused on Workflow revision (no Constitution change).

## Plan

1. Invoke `tl` to orchestrate.
2. `tl` reads the current `workflow.md` and the open Tasks under `.claude/specs/tasks/`.
3. `tl` invokes `researcher` if external context has changed (new stack piece, new domain dependency).
4. `tl` invokes `planner` to produce a **diff proposal** — what to add, modify, or remove in the Workflow.
5. **Stop and surface the diff to the user.** Ask: `Apply workflow diff? (yes / revise / cancel)`.
6. If `yes`, `specter` applies the diff: updates `workflow.md`, adds new Task skeletons, and (only if necessary) adds new agent/skill/hook/command skeletons.

## Operating rules

- Constitution is never touched here. If a constitution change is needed, the user must do it manually (see constitution §7).
- Removing a declared component is allowed only if no Task in `done` status uses it. Otherwise, the diff is rejected and surfaced as a blocker.
