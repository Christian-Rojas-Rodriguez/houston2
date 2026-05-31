---
description: Materialize a Spec on disk at `.claude/specs/tasks/<id>-<slug>.md` from a polished What/Why/How idea. Renders the canonical template with frontmatter, scope, acceptanceCriteria, and initial version 0.1.0. Used by Specter during UC-1 (Bootstrap output) and UC-2 (per-Task spec).
disable-model-invocation: true
allowed-tools: Read, Write
---

> Skeleton — full behavior is implemented in Task 0014.

# write-spec

Render and write a Spec file in the canonical format expected by `pre-spec-validate`.

## Inputs

- Task id (4-digit string).
- Task slug (kebab-case).
- Granularity (inherited from Workflow).
- Polished What / Why / How (from Curator via Skill `polish-idea`).
- Scope: list of paths the implementation may touch.

## Output (template)

```markdown
---
task: "<id>"
slug: <slug>
granularity: <component|task|feature|...>
version: 0.1.0
status: in-progress
declares:
  - type: <agent|skill|hook|command|other>
    name: <name>
    path: <relative path>
scope:
  - <path-or-glob>
---

# Task <id> — <human title>

## What
<from polish-idea>

## Why
<from polish-idea>

## How
<from polish-idea>

## Acceptance criteria
1. <atomic, testable claim>
2. ...

## Out of scope
- <explicit non-goals>
```

## Operating rules

- Writes are restricted to `.claude/specs/tasks/**`. Reject any path outside.
- New Specs always start at `version: 0.1.0`. Never bump here — that is Auditor's job.
- Refuse to materialize if What / Why / How / acceptanceCriteria are not all present. The `pre-spec-validate` hook will also reject, but failing earlier is cheaper.
- Never editorialize the content received from Curator/Planner.
