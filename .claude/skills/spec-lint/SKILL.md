---
description: Lint a Spec file for testability, ambiguity, and structural compliance. Returns OK or a list of fixes. Used by QA before authoring tests so that any ambiguity is resolved at the Spec layer instead of leaking into tests.
disable-model-invocation: true
allowed-tools: Read
---

# spec-lint

Heuristic check that a Spec is testable and unambiguous.

## Inputs

- Path to a single Spec file under `.claude/specs/tasks/`.

## Checks

1. **Structure** — required sections present: frontmatter, `## What`, `## Why`, `## How`, `## Acceptance criteria`, `## Out of scope`.
2. **Frontmatter** — `task`, `slug`, `granularity`, `version`, `status`, `declares`, `scope` all set; `version` parses as SemVer.
3. **Acceptance criteria** — at least one entry; each entry is a single sentence, atomic, with no `and` / `or` chains; each entry references concrete observables (file paths, values, behaviors).
4. **Ambiguity tokens** — flag words like `properly`, `correctly`, `seamlessly`, `appropriate`, `etc.` — these signal vague criteria.
5. **Scope sanity** — every path in frontmatter `scope` exists or is a sibling of an existing path (no typos).
6. **Cross-link** — the Workflow declares this Task; if not, that is a blocking error.

## Output

```
verdict: OK | needs-revision
findings:
  - severity: block | warn
    location: <line> or <field>
    message: <one sentence>
    suggestion: <one sentence>
```

## Operating rules

- Be strict on `block` severity. Block on any structural failure or ambiguity token in an acceptance criterion.
- Never modify the Spec; only report. QA / Curator handles the revision.
