---
description: Derive workflow.md (granularity strategy, task list, and declared POA components) from an approved RFC. Use during Bootstrap (UC-1) after the RFC gate passes, and during Workflow revision (/workflow-review).
disable-model-invocation: true
allowed-tools: Read
---

# plan-workflow

Derive the canonical `workflow.md` from an approved RFC. `workflow.md` is a machine-readable declaration;
the prose rationale lives in the PRD and RFC.

## Inputs

- Approved RFC (`.claude/specs/rfc.md`) — especially **§7** (granularity + task list) and **§8** (POA declaration).
- User's high-level intent (for intro framing).

## Output

A Markdown document with the following sections, ready to be inserted as `.claude/specs/workflow.md`:

1. `## 1. Granularity` — the chosen granularity strategy and the justification (one paragraph). **Derived from RFC §7.**
2. `## 2. Declared components` — tables for agents / skills / hooks / commands / MCPs. **Derived from RFC §8.**
3. `## 3. Tasks` — ordered list with `id`, `slug`, scope. **Derived from RFC §7.**
4. `## 4. Dependencies` — mermaid graph of Task dependencies.
5. `## 5. Definition of Done` — generic DoD plus task-specific overrides if needed.
6. `## 6. Risks` — known risks and their mitigations. **Derived from RFC §10.**

## Operating rules

- Keep the output **terse**. The Workflow is a machine-readable declaration; verbosity belongs in the RFC.
- One Task = one Spec. If you cannot describe a Task in one sentence, raise an `Open question` — do not silently merge tasks.
- Do not invent components. Faithfully translate RFC §8; if the RFC is incomplete, leave a `?` and surface it.
- `workflow.md` is **derived** from the RFC; it does not replace it. The RFC is the human-reviewed document; `workflow.md` is the sovereign POA declaration used at runtime by hooks and agents.
