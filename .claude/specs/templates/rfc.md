---
version: 0.1.0
status: draft
prd: .claude/specs/prd.md
---

# RFC: [Título técnico]

| Campo | Valor |
|---|---|
| **Autor(es)** | |
| **Revisores** | |
| **PRD relacionado** | [.claude/specs/prd.md](.claude/specs/prd.md) |

## §1. Resumen

Qué propone este documento en pocas frases.

## §2. Contexto y motivación

- Estado actual del sistema.
- Por qué necesitamos este cambio (link al PRD §2/§3).

## §3. Goals / Non-goals

**Goals:** qué resuelve este diseño.

**Non-goals:** qué queda fuera.

## §4. Diseño propuesto

- Arquitectura (diagramas, componentes).
- Modelo de datos / esquema.
- APIs / contratos / interfaces.

## §5. Alternativas consideradas

| Opción | Pros | Contras | Decisión |
|---|---|---|---|

## §6. Impacto transversal

- **Seguridad / privacidad:**
- **Performance / escalabilidad:**
- **Observabilidad:** logs, métricas, alertas.
- **Migración / backward compatibility:**

## §7. Granularidad y descomposición en Tasks

Estrategia de granularidad elegida: `[descripción — ej. "1 Task = 1 componente POA"]`

Justificación: [por qué esta granularidad es la correcta para este proyecto]

| id | slug | scope (una línea) |
|---|---|---|

## §8. Declaración POA

> Estas tablas son la fuente que `plan-workflow` usa para derivar `workflow.md`. Deben estar completas antes de la aprobación del RFC.

### Agents

| # | Agent | Capa | Color | Task |
|---|---|---|---|---|

### Skills

| # | Skill | Usada por | Task | `disable-model-invocation` |
|---|---|---|---|---|

### Hooks

| # | Hook | Evento | Task | Bloqueante |
|---|---|---|---|---|

### Commands

| # | Command | Use case | Task |
|---|---|---|---|

### MCPs

`Ninguno` _(los MCPs son responsabilidad de los Agents de dominio)_

## §9. Plan de testing

- **Unit:** un test por acceptance criterion atómico.
- **Integration:** un test por interacción entre Tasks declarada en el Workflow.
- **Acceptance:** un test por escenario "What" de la Spec.

## §10. Riesgos y mitigaciones

| Riesgo | Probabilidad | Impacto | Mitigación |
|---|---|---|---|

## §11. Preguntas abiertas

- [ ]
