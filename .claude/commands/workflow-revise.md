---
description: Revise workflow.md mid-project — add or remove tasks, change granularity, update declared agents/skills/hooks/commands. Planner reads the current workflow, proposes a diff, Specter applies after approval. Creates task skeletons for new tasks.
argument-hint: "[what you want to change — optional, will ask interactively if omitted]"
disable-model-invocation: true
---

# /workflow-revise

Revise the project Workflow. Change context: `$ARGUMENTS`.

## What to do

Invoke `Agent(architect)` with:

> "Run /workflow-revise. Change context: $ARGUMENTS
> Follow these steps exactly."

---

## Steps

```
Step 0.  Ensure clean git state on develop:
         git status  → must be clean. If dirty: STOP, tell user to commit or stash first.
         git checkout develop
         git pull origin develop

Step 1.  Invoke Agent(planner):
         "Read .claude/specs/workflow.md, .claude/specs/rfc.md, and
          .claude/specs/tasks/ (list all task specs and their status).
          Change context from user: '$ARGUMENTS'

          If change context is empty or vague:
            Ask the user: 'What do you want to change in the Workflow?
            Examples: add a new task, remove a task, change task order,
            add/remove a declared agent or skill, adjust granularity.'
            Wait for the answer.

          Once you know what to change:
          - Identify the affected sections (task list, declared components, dependencies)
          - Check constraints:
            * Cannot remove a task that is in-progress or done
            * Cannot remove a declared component used by a done task
            * New tasks must follow the id+slug naming convention
          - Ask 1-2 clarifying questions if scope of new task(s) is unclear
          - Propose the diff:

            CURRENT workflow.md §X:
            [current text]

            PROPOSED workflow.md §X:
            [new text]

          - List new task skeletons to be created (if any).
          - Note if the change contradicts rfc.md §7 or §8 and whether /rfc-revise should run first.

          Output the diff proposal."

Step 2.  Show the user the diff.
         Ask: "Apply this Workflow revision? Reply 'yes', 'revise: <feedback>', or 'cancel'."
         — Wait for explicit approval —

Step 3.  On 'yes': Invoke Agent(specter):
         "Apply this diff to .claude/specs/workflow.md:
          [paste full diff]
          Preserve all unchanged sections and frontmatter.
          For each NEW task in the diff, create a skeleton spec at
          .claude/specs/tasks/<id>-<slug>.md with status: skeleton."

Step 4.  Run git commit:
         git add .claude/specs/workflow.md .claude/specs/tasks/
         git commit -m "chore(workflow): revise — [summary of change]"

Step 5.  Report: what changed, new skeleton tasks created (if any), recommended next steps.
         If new tasks were added: suggest running /task-run <new-id> for each.
```

## Operating rules

- Constitution is never touched here. If a constitution change is needed, the user does it manually.
- Removing a task that is `in-progress` or `done` is a blocker — surface it and stop.
- Workflow is the sovereign declarant — any new component MUST be declared here before it is implemented.
