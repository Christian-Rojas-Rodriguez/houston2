---
description: Draft an RFC (Request for Comments / Technical Design Doc) from an approved PRD. Conducts a technical interview, decides granularity, decomposes into Tasks, and declares POA components (agents/skills/hooks/commands/MCPs). Use during Bootstrap (UC-1) after the PRD gate passes. Output feeds plan-workflow to derive workflow.md.
disable-model-invocation: true
allowed-tools: Read
---

# draft-rfc

Conduct a technical interview from an approved PRD and render a filled RFC — including granularity strategy, Task decomposition, and POA declaration (§7 and §8 are load-bearing for `plan-workflow`).

## Inputs

- Approved PRD (`.claude/specs/prd.md`).
- Researcher findings (domain + stack).
- Template: `.claude/specs/templates/rfc.md`.

## Interview guide

| Section | Key question to resolve |
|---|---|
| §2 Contexto | ¿Cuál es el estado actual del sistema que esto cambia? |
| §3 Goals | ¿Qué goals del PRD sirve directamente esta arquitectura? |
| §4 Diseño | ¿Cuál es el patrón arquitectural central (capas, flujo de datos)? |
| §5 Alternativas | ¿Qué opciones se descartaron y por qué? |
| §6 Impacto | ¿Hay concerns de seguridad, migración u observabilidad? |
| §7 Granularidad | ¿Cuál es la unidad atómica de trabajo? (1 Task = ¿1 qué?) |
| §8 Declaración POA | ¿Qué primitivas POA se necesitan: agents, skills, hooks, commands, MCPs? |
| §9 Testing | ¿Cuáles son los boundaries de unit / integration / acceptance? |

## Output

A filled `.claude/specs/templates/rfc.md`-shaped Markdown document. **§7 and §8 are mandatory and must be complete** — they are consumed by `plan-workflow` to derive `workflow.md`.

```markdown
---
version: 0.1.0
status: draft
prd: .claude/specs/prd.md
---

# RFC: <title>
...
## §11. Preguntas abiertas
- [ ] <anything unresolved>
```

## Operating rules

- The RFC captures **How** — architecture, trade-offs, granularity, and POA component declarations. Not What/Why (those live in the PRD).
- §7 (Granularidad) and §8 (Declaración POA) are load-bearing: `plan-workflow` reads these to derive `workflow.md`. Do not leave them as `TBD`.
- Invoke `propose-agents` when populating §8 to ensure correct model/effort/maxTurns/permissionMode defaults per agent.
- One Task = one Spec. If a scope item cannot be described in one sentence, decompose it further.
- Cite PRD sections (`PRD §X`) when an RFC decision traces to a product requirement.
- **Output goes to specter**, which writes it to `.claude/specs/drafts/rfc.md` with `status: draft` for the user to open, review, and iterate on. The file is the review surface — not the console output.
- This skill is `disable-model-invocation: true` because the draft must be written to disk and reviewed by the User before `plan-workflow` derives the Workflow.
