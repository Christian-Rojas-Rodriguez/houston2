# CLAUDE.md — memoria de open-factory-spec

> Este archivo se inyecta al contexto de Claude Code en cada sesión. Mantenerlo **conciso**: la documentación detallada vive en `.claude/specs/`.

## Qué es este repo

Una **factory de agentes para Claude Code** alineada a SDD spec-as-source. Implementación de POA (Programación Orientada a Agentes). Distribución actual: paquete npm `opftr` (Opción B). Distribución futura: monorepo híbrido CLI + plugin Claude Code (Opción D).

## Reglas no negociables

1. **Humanos editan specs y aprueban PRs.** Los agentes escriben el código. Toda diff de código exige diff de Spec.
2. **Permisos mínimos por defecto.** Cada Agent/Skill/Command/Hook declara solo lo necesario para su `objective`.
3. **Una Task = una Spec.** Si necesita más, está mal granularizada.
4. **El Workflow es soberano**: ningún componente POA existe si no está declarado en `.claude/specs/workflow.md`.
5. **QA escribe los tests antes que Coder implemente.** TDD por defecto. 3 niveles: unit / integration / acceptance.
6. **El Auditor es el gate**: valida equivalencia Spec↔Código en PR y bumpea `Spec.version` al merge.

Lectura completa: [`.claude/specs/constitution.md`](.claude/specs/constitution.md).

## Dónde vive cada cosa

| Tipo | Ubicación |
|---|---|
| Agents | `.claude/agents/<n>.md` |
| Skills | `.claude/skills/<n>/SKILL.md` (+ scripts) |
| Commands | `.claude/commands/<n>.md` |
| Hooks (scripts) | `.claude/hooks/<n>.{sh,py}` |
| Hooks (registro) | `.claude/settings.json` |
| Constitution | `.claude/specs/constitution.md` |
| Workflow | `.claude/specs/workflow.md` |
| Tasks + Specs | `.claude/specs/tasks/<id>-<slug>.md` |
| Spec maestra | `.claude/specs/SPEC.md` |
| Diagramas (POA + UCs) | `.claude/specs/diagrams/` |
| Tests | `tests/<level>/<task-id>__<slug>.test.*` |

## Estado actual de la factory

**Blueprint inicial** (v0.1.0). Los archivos `.claude/specs/*.md` están escritos; las Tasks 0001-0029 todavía no tienen `.claude/specs/tasks/<id>-<slug>.md` materializadas ni componentes POA implementados.

**Próximo hito**: Hito A (Foundation + Transversales, Tasks 0001-0006) hecho a mano para romper la recursión y permitir que la factory empiece a usarse a sí misma. Ver [`.claude/specs/workflow.md`](.claude/specs/workflow.md) §3 y [`.claude/specs/SPEC.md`](.claude/specs/SPEC.md) §12.

## Cuándo delegar a qué agente

Cuando trabajes en este repo, **usá los agentes proactivamente** según el matching natural del prompt:

- "Investigá / analizá el dominio" → `researcher`
- "Definí el workflow / diseñá las tasks" → `planner`
- "Pulí la idea / aclará el what/why/how" → `curator`
- "Escribí / materializá la spec" → `specter`
- "Validá la spec / escribí los tests" → `qa`
- "Implementá la task / hacé el código" → `coder`
- "Revisá el diff / code review" → `reviewer`
- "Corré los tests" → `tester`
- "Abrí el PR" → `pr`
- "Auditá / verificá el contrato" → `auditor`
- "Orquestá el ciclo completo" → `tl`

Si un agent base aún no está implementado (ver §"Estado actual"), Cursor o el usuario humano cubre ese rol manualmente.

Fixtures completos de matching: [`.claude/specs/diagrams/invocation-fixtures.md`](.claude/specs/diagrams/invocation-fixtures.md).

## Convenciones de código

- **Markdown + YAML frontmatter** para todas las definiciones POA.
- **Bash + jq** para hooks simples; **Python 3** para hooks complejos y para scripts de skills con efectos.
- **kebab-case** para nombres de Agents/Skills/Commands/Hooks (constitution §5).
- **Specs con SemVer**: `version: x.y.z` en el frontmatter. Bump solo por Auditor.

## Para futuros contribuidores

Antes de modificar cualquier cosa en `.claude/`:

1. Leer [`.claude/specs/constitution.md`](.claude/specs/constitution.md) y [`.claude/specs/SPEC.md`](.claude/specs/SPEC.md).
2. Confirmar que el cambio esté declarado en `workflow.md` (si no, hay que correr `/agent-new` o `/spec-new` primero).
3. Para tocar la constitution: justificar cláusula por cláusula y bumpear majors en las Specs afectadas.
