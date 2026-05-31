---
description: Revise an existing task spec (.claude/specs/tasks/<id>-<slug>.md) — update What/Why/How, add or refine acceptance criteria, adjust scope. Curator reads the current spec, proposes changes, Specter applies after approval.
argument-hint: "<task-id> [what you want to change — optional]"
disable-model-invocation: true
---

# /spec-revise

Revise task spec `$ARGUMENTS`.

## What to do

Parse `$ARGUMENTS`: first token is the task id, remainder is the change context.
Example: `/spec-revise 0003 tighten the acceptance criteria` → id=0003, context="tighten the acceptance criteria".

Invoke `Agent(architect)` with:

> "Run /spec-revise for task [id]. Change context: [context]
> Follow these steps exactly."

---

## Steps

```
Step 0.  Ensure clean git state on the task branch:
         git status  → must be clean. If dirty: STOP, tell user to commit or stash first.
         Read .claude/specs/tasks/<id>-*.md → get `branch` field from frontmatter.
         git checkout <branch>   (the task's own branch, not develop)
         git pull origin <branch> 2>/dev/null || true   (ok if branch doesn't exist remotely yet)

Step 1.  Invoke Agent(curator):
         "Read .claude/specs/tasks/<id>-*.md in full.
          Also read .claude/specs/prd.md and .claude/specs/rfc.md for traceability context.
          Change context from user: '[context]'

          If change context is empty:
            Ask the user: 'What do you want to change in the spec for task <id>?
            Examples: refine the What, fill in the How, tighten acceptance criteria,
            adjust scope, add out-of-scope items.'
            Wait for the answer.

          Once you know what to change:
          - Identify which spec sections are affected
          - Ask 1-2 clarifying questions if the change is ambiguous
          - Check traceability: the change must still trace to PRD and RFC
            (What/Why → PRD; How → RFC). Flag if it drifts.
          - Propose the revised section(s) as a diff:

            CURRENT ## What:
            [current text]

            PROPOSED ## What:
            [new text]

          - Flag if acceptance criteria changes affect already-written tests:
            '⚠ QA wrote tests for criteria [X] — if this criterion changes,
            those tests need to be updated too.'

          Output the diff proposal."

Step 2.  Show the user the diff.
         Ask: "Apply this spec revision? Reply 'yes', 'revise: <feedback>', or 'cancel'."
         — Wait for explicit approval —

Step 3.  On 'yes': Invoke Agent(specter):
         "Update .claude/specs/tasks/<id>-<slug>.md applying this diff:
          [paste full diff]
          Preserve frontmatter fields (task, slug, granularity, version, status,
          declares, scope). Do not bump the version — that is the auditor's job at merge."

Step 4.  Run git commit:
         git add .claude/specs/tasks/<id>-*.md
         git commit -m "docs(spec-<id>): revise [affected section(s)]"

Step 5.  Report: what changed. If acceptance criteria changed, remind user:
         "QA tests may need updating — run /task-run <id> Phase B again or
         ask qa to update the affected test files."
```

## Operating rules

- Never bump `version` in the frontmatter — that is the auditor's job at merge.
- Never change `scope` to include paths not declared in workflow.md for this task.
- If the change makes the spec inconsistent with the RFC, flag it and suggest running /rfc-revise first.
- If the task is already `done`, warn the user: revising a done spec requires a new PR.
