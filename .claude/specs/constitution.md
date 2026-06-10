# Constitution — open-factory-spec

> Principios no negociables del proyecto. Cualquier Spec, Agent, Skill, Command o Hook que contradiga esta constitution debe rechazarse en review.

## 1. Identidad

`open-factory-spec` es una **factory de agentes para Claude Code** que materializa dos ideas combinadas:

- **SDD spec-as-source** (Spec-Driven Development, paper arxiv:2602.00180 §II-C): la spec es el único artefacto que los humanos editan; el código es derivado.
- **POA** (Programación Orientada a Agentes): los componentes ejecutables (Agent, Skill, Command, Hook, MCP) se modelan como objetos de primera clase con `name`, `permissions` mínimas y `objective` explícito.

## 2. Reglas inviolables

### 2.1 Spec-as-source

1. **Los humanos solo editan specs y aprueban PRs.** Cualquier edición humana directa a código se considera un bug del flujo, no un atajo válido.
2. **Toda diff de código requiere una diff de Spec correspondiente.** El hook `pre-commit-contract` rechaza commits cuyo diff de código no esté cubierto por la versión actual de alguna Spec en scope.
3. **El Auditor es el gate final.** Valida la equivalencia Spec↔Código en cada PR y, al merge, bumpea `Spec.version` según el alcance del cambio.
4. **El Coder edita incrementalmente** (no regenera desde cero). El invariante de equivalencia con la Spec se garantiza mediante Auditor + tests autoría QA, no mediante regeneración pura.

### 2.2 POA — Programación Orientada a Agentes

5. **Todo componente ejecutable es un `POAObject`** con `name`, `permissions` (mínimas), `objective`. Sin excepciones.
6. **Permisos mínimos por defecto.** Un Agent/Skill/Command/Hook que requiera más permisos que los estrictamente necesarios para cumplir su `objective` debe rechazarse.
7. **Encapsulación por archivo.** Cada Agent vive en `.claude/agents/<name>.md`, cada Skill en `.claude/skills/<name>/SKILL.md`, cada Command en `.claude/commands/<name>.md`, cada Hook como script en `.claude/hooks/` + registro en `.claude/settings.json`.
8. **Composición sobre herencia.** Un Agent compone Skills + Commands + MCPs + Hooks; no hereda comportamiento de otro Agent. La reutilización entre agents pasa por compartir Skills.

### 2.3 Granularidad y workflow

9. **La granularidad se decide en Bootstrap, no en spec-cycle.** Específicamente, se fija en el **RFC** (§7 Granularidad), y el User la aprueba en el gate de RFC antes de que `planner` derive el Workflow. Curator/Specter NO la deciden; la heredan del Workflow derivado.
10. **Bootstrap se conduce como interview en dos etapas: PRD → RFC. Cada etapa escribe un borrador en disco para revisión en editor.**
    - El **PRD borrador** se escribe en `.claude/specs/drafts/prd.md` (`status: draft`). El User lo abre en su editor, lo edita directamente y aprueba con "approve prd" (Gate 1).
    - El **RFC borrador** se escribe en `.claude/specs/drafts/rfc.md` (`status: draft`). El User lo abre, lo revisa (§7 Granularidad y §8 Declaración POA deben estar completos) y aprueba con "approve rfc" (Gate 2).
    - Al aprobar cada gate, el borrador se **promueve** a `.claude/specs/prd.md` / `rfc.md` (`status: approved`).
    - `workflow.md` es **derivado del RFC aprobado** por `plan-workflow`. Sigue siendo el único declarante soberano en tiempo de ejecución.
    - **Hasta que ambos gates pasen**: lo único escrito son los borradores en `drafts/`. `workflow.md`, `constitution.md` y toda task spec se escriben SOLO después de Gate 2.
    - Los artefactos finales (`prd.md`, `rfc.md`, `workflow.md`, `constitution.md`) se commitean juntos al cierre del Bootstrap.
11. **Una Task = una Spec.** Si una Task pide múltiples specs, está mal granularizada y debe descomponerse.
12. **Workflow es el único artefacto que declara qué agents/skills/hooks/commands/MCPs necesita el proyecto.** Crear un componente nuevo sin pasar por Workflow es deuda técnica. (`workflow.md` es derivado del RFC; la fuente canónica de revisión humana es el RFC, pero el declarante en tiempo de ejecución es siempre `workflow.md`.)

### 2.4 Invocación y descubrimiento

12. **Agents y Skills se invocan por matching de prompt con `description`.** Toda `description` debe redactarse pensando en las frases naturales del User. Se valida con fixtures en `.claude/specs/diagrams/invocation-fixtures.md`.
13. **Commands y Hooks son determinísticos.** Para acciones con efectos (escribir spec, bumpear versión, abrir PR) se prefieren Commands (`/comando`) o Hooks (evento) por sobre invocación por prompt.
14. **Skills con efectos llevan `disable-model-invocation: true`** para evitar que Claude las dispare por su cuenta.

### 2.5 Testing — 3 niveles obligatorios

15. **QA escribe los tests antes que Coder implemente.** TDD por defecto, derivado del contrato de la Spec.
16. **Cobertura mínima en 3 niveles**: `unit` (por cada acceptance criterion), `integration` (por cada interacción entre Tasks declarada en el Workflow), `acceptance` (por cada escenario "What" del User).
17. **Paths convencionados**: `tests/<level>/<task-id>__*.test.*` para que `pre-commit-contract` mapee diff → Task → tests requeridos.

### 2.6 Versionado del contrato

18. **SemVer por Spec.** Patch = clarificación sin cambio de comportamiento. Minor = nuevo acceptance criterion compatible. Major = cambio incompatible en What/Why o eliminación de criterio.
19. **El bump lo dispara el Auditor al merge**, nunca antes.

## 3. Stack técnico

- **Definiciones**: Markdown con YAML frontmatter (formato nativo de Claude Code para subagents/skills/commands).
- **Hooks**: bash + `jq` para parseo de stdin JSON. Python opcional para hooks complejos.
- **Skill scripts**: Python 3 preferido (stdlib cuando alcance, requirements declarados cuando no). Bash para scripts cortos. Node solo si se justifica.
- **Tests**: depende del dominio del proyecto target — la factory no impone framework, sino la estructura de carpetas y el coverage gate.
- **Distribución**: ver §4.

## 4. Distribución (Opción B → D)

Decisión aprobada (May 17, 2026):

- **Fase inicial (B)**: paquete npm `opftr` (unscoped) que scaffoldea `.claude/` + `CLAUDE.md` en el proyecto target. Este repo es a la vez factory funcional y fuente del template. Nombre corto y unscoped para que `npx opftr init` funcione sin org registrada.
- **Fase futura (D)**: monorepo pnpm bajo la org `@open-factory` con `@open-factory/core` (tipos + validadores), `@open-factory/cli` (scaffolding + headless ops, sucesor de `opftr`) y `@open-factory/plugin` (Claude Code plugin). El plugin distribuye lo que tolera ser plugin-level; el CLI sigue generando lo que necesita campos por-agent restringidos en plugins (`hooks`, `mcpServers`, `permissionMode`). La migración de `opftr` → `@open-factory/cli` se hace cuando se registre la org en npm.

**Implicancia para el layout actual**: nada cambia. Los archivos en `.claude/` son a la vez el dogfooding de la factory y los templates que el CLI publicará. Cero re-trabajo al migrar.

## 5. Convenciones de naming

- **Agents** en kebab-case sin namespace (ej. `planner`, `nextjs-page`).
- **Skills** en kebab-case con verbo-sustantivo (ej. `polish-idea`, `author-tests`).
- **Commands** en kebab-case con verbo o verbo-sustantivo (ej. `factory-init`, `task-run`, `spec-bump`).
- **Hooks** en kebab-case con `pre-`/`post-` + objetivo (ej. `pre-spec-validate`, `post-merge-bump`).
- **Tasks** con id numérico de 4 dígitos + slug descriptivo (ej. `0007-figma-design-system-import`).
- **Specs**: nombre del archivo = id de Task (`tasks/0007-…md`).

## 6. Memoria viva

Los agentes con `memory: project` escriben aprendizajes en `.claude/agent-memory/<agent>/MEMORY.md`. Este directorio se commitea para que el conocimiento institucional viaje con el repo.

Los agentes con `memory: local` usan `.claude/agent-memory-local/` (gitignored) para experimentos sin compartir.

## 7. Cambios a esta constitution

Cambiar este archivo requiere:

1. PR con justificación explícita por cada cláusula tocada.
2. Aprobación humana (no se delega a Auditor — la constitution está por encima del contrato Spec).
3. Si la cláusula afecta a algún Agent/Skill/Hook existente, las Specs correspondientes deben bumpear major.

### Enmienda 2026-05-30 — Bootstrap PRD → RFC

**Cláusulas modificadas**: §2.3.9, §2.3.10, §2.3.11 (renumerada a 12).

**Justificación**: el Bootstrap original producía un único `workflow.md` que mezclaba rationale de producto, diseño técnico, granularidad y declaración POA — difícil de revisar y sin separación de concerns. La enmienda introduce dos artefactos de revisión humana (PRD y RFC) antes de derivar `workflow.md`, manteniendo la soberanía del Workflow en tiempo de ejecución intacta (cláusula 12 = ex-11).

**Impacto en specs**: Tasks 0007-0009 (planner, plan-workflow, propose-agents) y Task 0024 (factory-init) requieren bump major al materializarse. Se agregan Tasks 0030 (draft-prd) y 0031 (draft-rfc) al Workflow.
