---
description: Polish the raw idea of a single Task into a structured What / Why / How triple, suitable for Specter to materialize as a Spec. Use during the Specify phase of UC-2 right after researcher hands off task-level context.
disable-model-invocation: true
allowed-tools: Read
---

# polish-idea

Quality checklist for What / Why / How / Acceptance criteria. Read this before producing any spec section.

## What — quality bar

| Check | Pass | Fail |
|---|---|---|
| User-visible | Describes what a user or downstream system *observes* | Describes internal implementation steps |
| Business terms | No code, no SQL, no function names | "calls `createOrg()` with..." |
| Tight scope | Covers exactly this task, not adjacent ones | Bleeds into next task's scope |
| Complete | No `TBD` unless paired with an open question | Left blank |

## Why — quality bar

- Must cite at least one PRD section (`see PRD §X`)
- Must answer "why *now*" — not just "because it's needed"
- Must state the consequence of skipping: what breaks, what stays blocked, what can't be built

## How — quality bar

- Lists **exact file paths** that will be created or modified (not "some files in src/")
- Names key interfaces, schemas, or function signatures at a sketch level
- Cites RFC sections for design decisions (`see RFC §4`)
- Notes cross-task dependencies by task id (`depends on Task 0001`)
- Does NOT contain full code — only structural sketches

## Acceptance criteria — checklist (apply to every criterion)

```
[ ] Atomic — one single observable thing (no "and", no "or")
[ ] Testable — can be verified by running a command, reading a file, or checking a value
[ ] Unambiguous — no weasel words: properly, correctly, seamlessly, appropriate, as expected
[ ] Concrete — names specific files, tables, endpoints, values, or behaviors
[ ] Scoped — within this task's declared scope, not another task's
```

**Examples**

Bad (vague):
- "The database schema is correctly set up."
- "Authentication works properly."

Good (concrete):
- "Running `supabase db push` applies without errors; `organizations`, `memberships`, `agents` tables exist with the columns declared in RFC §4.2."
- "POST `/auth/login` with valid credentials returns HTTP 200 and a JWT with `sub` equal to the user's UUID."
- "File `src/lib/db.ts` exports a `drizzle` client instance; TypeScript compilation passes with `tsc --noEmit`."

## Out of scope — quality bar

- At least 2 explicit bullets
- Derived from RFC §3 (non-goals) or adjacent task boundaries
- Each bullet names what is *not* done, not what *is* done

## Open questions — when to use

Add an open question when:
- A How decision depends on information not in the RFC or codebase
- An acceptance criterion cannot be made concrete without user input
- Scope is genuinely ambiguous between this task and an adjacent one

Format: `**Open question**: <one sentence describing what is unknown and who can resolve it>`
