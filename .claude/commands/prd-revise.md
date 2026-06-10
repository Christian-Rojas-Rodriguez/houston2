---
description: Revise the approved PRD (.claude/specs/prd.md) mid-project. Planner reads the current PRD, asks what needs to change, proposes a diff, and Specter applies it after approval. Warns if the change may affect the RFC.
argument-hint: "[what you want to change — optional, will ask interactively if omitted]"
disable-model-invocation: true
---

# /prd-revise

Revise the approved PRD. Change context: `$ARGUMENTS`.

## What to do

Invoke `Agent(architect)` with:

> "Run /prd-revise. Change context: $ARGUMENTS
> Follow these steps exactly."

---

## Steps

```
Step 0.  Ensure clean git state on develop:
         git status  → must be clean. If dirty: STOP, tell user to commit or stash first.
         git checkout develop
         git pull origin develop

Step 1.  Invoke Agent(planner):
         "Read .claude/specs/prd.md in full.
          Change context from user: '$ARGUMENTS'

          If change context is empty or vague:
            Ask the user: 'What do you want to change in the PRD?
            Be specific: which section, and what should it say instead?'
            Wait for the answer before continuing.

          Once you know what to change:
          - Identify which PRD sections are affected
          - Ask 1-3 targeted clarifying questions IF the change is ambiguous
            (e.g. 'You want to change the target user — should the RFC personas
            also change, or just the PRD?')
          - Propose the revised section(s) as a diff:

            CURRENT §X:
            [current text]

            PROPOSED §X:
            [new text]

          - If the change touches §5 (Objetivos), §6 (Requisitos), or §4 (Usuarios):
            add a warning: 'This change may affect RFC §X — review rfc.md after approving.'

          Output the diff proposal."

Step 2.  Show the user the diff.
         Ask: "Apply this PRD revision? Reply 'yes', 'revise: <feedback>', or 'cancel'."
         — Wait for explicit approval —
         On 'revise': relay feedback to planner, get updated proposal, repeat.

Step 3.  On 'yes': Invoke Agent(specter):
         "Update .claude/specs/prd.md applying this diff:
          [paste full diff from planner output]
          Preserve all other sections unchanged. Keep frontmatter (version, status)."

Step 4.  Run git commit:
         git add .claude/specs/prd.md
         git commit -m "docs(prd): revise [affected section(s)]"

Step 5.  Report to user: what changed and whether RFC review is recommended.
```

## Operating rules

- PRD is the What/Why layer — do not let implementation decisions bleed in.
- The constitution prohibits agents from bumping Spec versions; that happens at merge via auditor.
- If the user wants to change something that belongs in the RFC (architecture, granularity, POA), redirect them to `/rfc-revise`.
