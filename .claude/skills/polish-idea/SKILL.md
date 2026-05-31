---
description: Polish the raw idea of a single Task into a structured What / Why / How triple, suitable for Specter to materialize as a Spec. Use during the Specify phase of UC-2 right after researcher hands off task-level context.
allowed-tools: Read
---

> Skeleton — full behavior is implemented in Task 0012.

# polish-idea

Turn an unstructured task description into a three-dimensional idea ready for spec materialization.

## Inputs

- Task id and slug (granularity already fixed by Workflow).
- Raw description (one paragraph from the Workflow's task list).
- Researcher findings (citations, surrounding context).

## Output

```markdown
## What
<user-facing behavior in business terms; no implementation>

## Why
<business reason; why now, why this scope>

## How
<technical sketch; paths, interfaces, trade-offs>

## Open questions
<anything that needs User clarification or further research>
```

## Operating rules

- The three sections are mandatory; if any is empty, return `Open questions` instead of fabricating content.
- Granularity is **inherited** — do not re-scope the Task. If the Task feels mis-sized, return a single `Open question` recommending Workflow revision.
- Cite researcher findings inline; never make claims without a source.
- This Skill is auto-invocable (no `disable-model-invocation`) because it has no side effects; it returns structured text only.
