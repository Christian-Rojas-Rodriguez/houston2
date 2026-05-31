---
description: Drive a single Task through the full spec-cycle (UC-2) — researcher → curator → specter → gate → qa → coder → reviewer → tester → pr → auditor. Use to implement one Task at a time from the Workflow.
argument-hint: <task-id>
disable-model-invocation: true
---

# /task-run

Run UC-2 for Task `$ARGUMENTS`. This command has two phases separated by a human review gate.

## What to do

Invoke `Agent(architect)` with the following instruction:

> "Run UC-2 for task $ARGUMENTS. Follow the two-phase sequence exactly as described below. Stop at the spec gate and wait for explicit approval before starting Phase B."

---

## Phase A — Spec (no code written yet)

```
Step 1.  Invoke Agent(researcher):
         "Survey the codebase and relevant libraries for task $ARGUMENTS.
          Read workflow.md, prd.md, rfc.md, and the existing task spec if it exists.
          Return: what exists at the declared scope paths, key interfaces, prior art."

Step 2.  Invoke Agent(curator):
         "Produce complete What/Why/How + acceptance criteria for task $ARGUMENTS.
          Use researcher output: [paste step 1 output].
          If the spec already has skeleton placeholders, fill them — preserve the frontmatter."

Step 3.  Invoke Agent(specter):
         "Write/update .claude/specs/tasks/$ARGUMENTS-<slug>.md with this content:
          [paste full curator output]
          Preserve existing frontmatter fields (id, slug, version, status, declares, scope)."

Step 4.  Run git commit:
         git add .claude/specs/tasks/$ARGUMENTS-*.md
         git commit -m "spec($ARGUMENTS): materialize <slug>"

         *** SPEC GATE — STOP ***
         Show the user the path of the spec just written.
         Say: "Spec written to .claude/specs/tasks/$ARGUMENTS-<slug>.md
               Open it, review and edit if needed.
               Reply 'approve spec', 'revise: <feedback>', or 'cancel'."
         — DO NOT start Phase B until explicit approval —
         On "revise": relay feedback to curator, get updated content, specter rewrites, repeat step 4.
```

---

## Phase B — Implementation (only after spec approved)

```
Step 5.  Invoke Agent(qa):
         "Read .claude/specs/tasks/$ARGUMENTS-<slug>.md.
          Run Skill(spec-lint) and fix any blocking issues first.
          Then author the failing test suite: unit tests (one per acceptance criterion),
          integration tests (per workflow dependencies), acceptance tests (per What scenarios).
          Return the list of test files written."

Step 6.  Run git commit:
         git add tests/
         git commit -m "test($ARGUMENTS): author failing suite for <slug>"

Step 7.  Invoke Agent(coder):
         "Implement task $ARGUMENTS. Read the spec at .claude/specs/tasks/$ARGUMENTS-<slug>.md.
          Implement incrementally until all tests in tests/**/$ARGUMENTS__*.test.* pass.
          Stay within the declared scope paths. Do not modify test files."

Step 8.  Invoke Agent(reviewer):
         "Review the diff for task $ARGUMENTS against .claude/specs/tasks/$ARGUMENTS-<slug>.md.
          Check: all scope paths touched, no out-of-scope changes, no spec drift."

Step 9.  Invoke Agent(tester):
         "Run the full test suite for task $ARGUMENTS. Return pass/fail per level."

Step 10. If tester reports failures: return to coder (step 7) with the failure report.
         If tester is green:

Step 11. Run git commit:
         git add <all changed source files>
         git commit -m "feat($ARGUMENTS): implement <slug>"

Step 12. Invoke Agent(pr):
         "Open a pull request for task $ARGUMENTS.
          Title: '$ARGUMENTS: <slug>'
          Body must reference: spec path, test count, reviewer sign-off."

Step 13. Invoke Agent(auditor):
         "Validate Spec↔Code equivalence for task $ARGUMENTS.
          Read .claude/specs/tasks/$ARGUMENTS-<slug>.md and the diff.
          Propose a SemVer bump (patch/minor/major) with justification."

Step 14. Report to user: PR URL + bump proposal. User approves the merge.
```

---

## Operating rules

- Every step is logged as a `## Current step` line in architect's output.
- Commits are made by architect using Bash after each major artefact (spec, tests, implementation).
- If any agent returns a `block`, architect stops and surfaces the blocker — no auto-retry.
- The user is always the merge approver. Auditor only proposes; never merges.
- Never skip the spec gate. A skeleton spec is not an approved spec.
