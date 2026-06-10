---
description: Revise the approved RFC (.claude/specs/rfc.md) mid-project. Planner reads the current RFC, asks what needs to change, proposes a diff, and Specter applies it after approval. If §7 (Granularity) or §8 (POA Declaration) change, workflow.md is re-derived automatically.
argument-hint: "[what you want to change — optional, will ask interactively if omitted]"
disable-model-invocation: true
---

# /rfc-revise

Revise the approved RFC. Change context: `$ARGUMENTS`.

## What to do

Invoke `Agent(architect)` with:

> "Run /rfc-revise. Change context: $ARGUMENTS
> Follow these steps exactly."

---

## Steps

```
Step 0.  Ensure clean git state on develop:
         git status  → must be clean. If dirty: STOP, tell user to commit or stash first.
         git checkout develop
         git pull origin develop

Step 1.  Invoke Agent(planner):
         "Read .claude/specs/rfc.md and .claude/specs/prd.md in full.
          Change context from user: '$ARGUMENTS'

          If change context is empty or vague:
            Ask the user: 'What do you want to change in the RFC?
            Examples: adjust the architecture in §4, change granularity in §7,
            add/remove an agent in §8, update task scope.'
            Wait for the answer.

          Once you know what to change:
          - Identify which RFC sections are affected
          - Ask 1-3 targeted clarifying questions if the change is ambiguous
          - Propose the revised section(s) as a diff:

            CURRENT §X:
            [current text]

            PROPOSED §X:
            [new text]

          - Flag explicitly if the diff touches §7 or §8:
            '⚠ §7/§8 changed — workflow.md will be re-derived after approval.'

          - If the change contradicts the PRD, flag it:
            '⚠ This changes [X] which traces to PRD §Y — confirm this is intentional.'

          Output the diff proposal."

Step 2.  Show the user the diff.
         Ask: "Apply this RFC revision? Reply 'yes', 'revise: <feedback>', or 'cancel'."
         — Wait for explicit approval —
         On 'revise': relay feedback to planner, get updated proposal, repeat.

Step 3.  On 'yes': Invoke Agent(specter):
         "Update .claude/specs/rfc.md applying this diff:
          [paste full diff from planner output]
          Preserve all other sections unchanged. Keep frontmatter."

Step 4.  Run git commit:
         git add .claude/specs/rfc.md
         git commit -m "docs(rfc): revise [affected section(s)]"

Step 5.  IF the diff touched §7 (Granularity) or §8 (POA Declaration):
         Invoke Agent(planner):
         "Re-derive workflow.md from the updated .claude/specs/rfc.md §7 and §8.
          Use Skill(plan-workflow). Return the complete updated workflow.md."

         Then Invoke Agent(specter):
         "Update .claude/specs/workflow.md with this content:
          [paste planner output]"

         Run git commit:
         git add .claude/specs/workflow.md
         git commit -m "chore(workflow): re-derive from updated RFC §7/§8"

         Report: "workflow.md re-derived. Review new task list and run /task-run <id> for any new tasks."

Step 6.  Report to user: what changed, whether workflow was re-derived, recommended next steps.
```

## Operating rules

- §7 and §8 are load-bearing — any change to them MUST trigger a workflow re-derivation (Step 5).
- If removing a declared component (agent/skill/hook) that already has a task in `done` status, flag it as a blocker and do not apply.
- RFC is the How layer — do not let product decisions bleed in. Redirect to `/prd-revise` if needed.
