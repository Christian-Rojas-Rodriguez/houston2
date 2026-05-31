---
description: Draft a PRD (Product Requirements Document) for a new project by conducting a targeted product interview. Use during Bootstrap (UC-1) after researcher has surveyed the domain. Input: description + researcher findings. Output: filled PRD using the canonical template at .claude/specs/templates/prd.md.
disable-model-invocation: true
allowed-tools: Read
---

# draft-prd

Guide for identifying what to ask and how to compose the PRD. Planner reads this to decide which questions to generate.

---

## Phase 1 — Gap analysis (what to ask)

Read the project description + researcher output. For each PRD section, decide: **infer or ask?**

| PRD section | Infer when... | Ask when... |
|---|---|---|
| §1 TL;DR | Description names the product and audience | Core value prop is vague or multi-product |
| §2 Vision / Strategy | Roadmap context is stated or obvious from domain | Multiple plausible strategic directions exist |
| §3 Problem / Context | Pain is clearly described | Current state is ambiguous or problem needs prioritization |
| §4 Target users | User is named in the description | Multiple plausible user types compete |
| §5 Objectives / Metrics | Success is implicit from the problem statement | Success criteria could mean very different things |
| §6 Requirements | Core feature set is obvious from description | P0/P1/P2 split is unclear; what launches vs. what waits |
| §7 UX | No UI involved, or flows are trivial | UI flows exist but aren't described; reference products matter |
| §8 Dependencies | Stack is described by researcher and has no ambiguity | External systems are named but ownership/contracts unclear |
| §9 Rollout | Single-tenant or obvious launch strategy | Multi-phase or flag-gated rollout could vary significantly |

**Rule**: if you can infer a section with high confidence from description + research, do so. Only ask when the answer would meaningfully change the PRD content.

---

## Phase 2 — Question quality bar

Each question you generate must pass all of these:

- **Specific**: references THIS project's domain, not a generic category ("What are your metrics?" → bad; "You mention a leaderboard — should the north-star metric be DAU or session length?" → good)
- **Single-focus**: one ambiguity per question, no compound questions
- **High-impact**: the answer would change at least one PRD section meaningfully
- **Not derivable**: cannot be answered by re-reading the description + researcher output

**Max 7 questions.** If you have more than 7 gaps, prioritize the ones that affect §6 (Requirements) and §4 (Users) first — those drive the most downstream decisions.

---

## Phase 3 — Composition rules

After receiving answers, compose the PRD:

- Synthesize description + research + answers into prose — do not paste answers verbatim
- Cite the description when inferring (`from description: "..."`)
- Every §6 P0 item must be something the user or answers confirmed explicitly
- §10 Open questions: only for things still unresolved after the Q&A
- The file is the review surface — write so a reader unfamiliar with the conversation understands it fully

---

## PRD output format

```markdown
---
version: 0.1.0
status: draft
---

# PRD: <title>

| Campo | Valor |
|---|---|
| Autor | |
| Target release | |

## §1. TL;DR
## §2. Vision y Estrategia
## §3. Contexto y Problema
## §4. Usuarios Target
## §5. Objetivos y Métricas
## §6. Requisitos (P0 / P1 / P2)
## §7. Experiencia de Usuario
## §8. Dependencias
## §9. Rollout
## §10. Preguntas Abiertas
```
