---
description: Add a new domain agent to the project, running UC-4 (mini-Bootstrap). Routes through Planner, requires explicit user approval, then materializes via Specter and updates the Workflow declaration.
argument-hint: <agent-name>
disable-model-invocation: true
---

> Skeleton — full behavior is implemented in Task 0027.

# /agent-new

Add a domain agent named `$ARGUMENTS`.

## Plan

1. `tl` invokes `planner` to propose the new agent: `objective`, `model`, `effort`, `maxTurns`, `permissionMode`, `memory`, `color`, minimum-privilege `tools`, plus any skills/hooks/MCPs it needs.
2. `tl` surfaces the proposal to the user. Ask: `Approve agent <name>? (yes / revise / cancel)`.
3. If `yes`, `specter` writes `.claude/agents/<name>.md` and updates `workflow.md` to add the agent (and any new skills/hooks/MCPs) to `Workflow.declared*`.
4. Re-sync templates so the new agent ships in the next CLI release.

## Operating rules

- Reuse before invent. Planner must justify why an existing Skill/Hook cannot be reused.
- A domain agent with more than 5 unique Skills is mis-granular; Planner must split it or stop and ask.
- `pre-spec-validate` will block any new component file that is not declared in `Workflow.declared*`. Update the Workflow first, materialize second.
