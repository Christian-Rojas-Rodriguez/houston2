---
description: Bootstrap a new project with POA + spec-as-source — runs UC-1 end-to-end (researcher → planner PRD interview → PRD review gate → RFC interview → RFC review gate → specter materializes). Use when starting a brand-new project.
argument-hint: "<one-paragraph project description>"
disable-model-invocation: true
---

# /factory-init

Bootstrap this project end-to-end using the two-gate UC-1 flow. Project description: `$ARGUMENTS`.

## What to do

Before invoking architect, ensure a clean git state on develop:
```
git status  → must be clean. If dirty: STOP, tell user to commit or stash first.
git checkout develop
git pull origin develop
```
All PRD/RFC/workflow bootstrap commits go directly to `develop`.

Invoke `Agent(architect)` with the following instruction:

> "Run UC-1 factory bootstrap for this project. Project description: $ARGUMENTS
> Follow the UC-1 two-gate sequence from your instructions exactly:
> researcher → planner PRD interview → Gate 1 (write drafts/prd.md, wait for approval) → planner RFC interview → Gate 2 (write drafts/rfc.md, wait for approval) → promote drafts → derive workflow.md → materialize all skeletons."

Then wait. TL will orchestrate the full bootstrap sequence, stopping at each gate for user review.

## Gate behavior

- **Gate 1**: TL will stop after Specter writes `.claude/specs/drafts/prd.md` and ask you to review it. Reply with `approve prd`, `revise: <feedback>`, or `cancel`.
- **Gate 2**: TL will stop after Specter writes `.claude/specs/drafts/rfc.md` and ask you to review it (§7 Granularity and §8 POA Declaration must be complete). Reply with `approve rfc`, `revise: <feedback>`, or `cancel`.

After both approvals, TL will materialize `workflow.md`, update `constitution.md`, and create task skeleton specs. It will then show you the list of files created and suggest running `/task-run <first-id>`.

## Acceptance

- Before Gate 1 approval: only `.claude/specs/drafts/prd.md` is written.
- Before Gate 2 approval: only `.claude/specs/drafts/rfc.md` is written.
- `workflow.md`, `constitution.md`, and all task specs are written ONLY after both gates pass.
- Granularity is fixed in RFC §7 and not re-decided by downstream agents.
- RFC §8 (POA Declaration) is complete before `plan-workflow` runs.
