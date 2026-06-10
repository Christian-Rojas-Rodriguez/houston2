---
description: Draft an RFC (Request for Comments / Technical Design Doc) from an approved PRD. Conducts a targeted technical interview, decides granularity, decomposes into Tasks, and declares POA components. Use during Bootstrap (UC-1) after the PRD gate passes. Output feeds plan-workflow to derive workflow.md.
disable-model-invocation: true
allowed-tools: Read
---

# draft-rfc

Guide for identifying what to ask and how to compose the RFC. Planner reads this to decide which technical questions to generate.

---

## Phase 1 — Gap analysis (what to ask)

Read the approved PRD + researcher output. For each RFC section, decide: **infer or ask?**

| RFC section | Infer when... | Ask when... |
|---|---|---|
| §2 Context | Current state described in PRD §3 | Existing system is complex and unstated |
| §3 Goals / Non-goals | Directly derivable from PRD §5 + §6 | PRD goals could map to multiple incompatible architectures |
| §4 Proposed design | Stack is known and there's one obvious pattern | Multiple viable architectures exist; trade-offs matter |
| §5 Alternatives | One dominant approach in the space | Explicit trade-off decisions should be documented |
| §6 Cross-cutting impact | Standard stack with no unusual security/perf constraints | Encryption, multi-tenancy, compliance, or migration is involved |
| **§7 Granularity** | **Always ask** unless PRD already names components (e.g. "6 DB tables") | — |
| **§8 POA Declaration** | **Always ask** to confirm agent/skill/hook set | — |
| §9 Testing | Standard unit/integration/acceptance | Project has unusual test requirements (SQL, infra, e2e flows) |

**§7 and §8 are mandatory** — always surface at least one question for each. They are load-bearing: `plan-workflow` cannot derive `workflow.md` without them being complete.

---

## Phase 2 — Question quality bar

Same rules as draft-prd. Additionally for RFC:

- **§7 question format**: "What is the atomic unit of work for a Task? (1 Task = 1 what?) Example options: 1 DB migration, 1 API endpoint, 1 agent definition. Propose your preferred granularity and list the resulting Tasks with a one-line scope each."
- **§8 question format**: "Which agents, skills, hooks, and commands does this project need? Review the standard set from `propose-agents` and tell me which apply, which to remove, and which new ones to add."
- For §4 (design): provide your best proposal from researcher context and ask for confirmation rather than asking an open-ended question. Example: "Based on your stack, I'd propose [X architecture]. Does this match your intent, or is there a different pattern you have in mind?"

**Max 7 questions.** §7 and §8 count as mandatory slots. Fill the rest with the most impactful technical unknowns.

---

## Phase 3 — Composition rules

After receiving answers:

- §7 must name the granularity strategy + a complete task table (id, slug, one-line scope)
- §8 must have complete tables for agents, skills, hooks, commands (MCPs optional)
- Use `propose-agents` defaults for agent config (model, effort, maxTurns, permissionMode)
- Cite PRD sections when an RFC decision traces to a product requirement (`PRD §5`)
- The file is the review surface — write so a reader can understand the full architecture without the Q&A transcript

---

## RFC output format

```markdown
---
version: 0.1.0
status: draft
prd: .claude/specs/prd.md
---

# RFC: <title>

## §1. Resumen
## §2. Contexto y motivación
## §3. Goals / Non-goals
## §4. Diseño propuesto
## §5. Alternativas consideradas
## §6. Impacto transversal
## §7. Granularidad y descomposición en Tasks   ← load-bearing
## §8. Declaración POA                          ← load-bearing
## §9. Plan de testing
## §10. Riesgos y mitigaciones
## §11. Preguntas abiertas
```

**§7 and §8 must be complete before Specter writes the RFC draft.** If they have `TBD`, ask a follow-up before outputting.
