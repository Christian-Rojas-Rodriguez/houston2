# SPEC — open-factory-spec

> Spec maestra de la factory. Consolida los principios de [`constitution.md`](./constitution.md), el modelo POA de [`diagrams/poa-class.md`](./diagrams/poa-class.md), los casos de uso de [`diagrams/sequences.md`](./diagrams/sequences.md), las convenciones de invocación de [`diagrams/invocation-fixtures.md`](./diagrams/invocation-fixtures.md) y el plan de implementación de [`workflow.md`](./workflow.md).
>
> Versión: `0.1.1` (blueprint inicial). Bumpea según política §11.

---

## 1. Propósito y alcance

`open-factory-spec` es una factory de agentes para [Claude Code](https://code.claude.com/docs/) alineada a **SDD spec-as-source** (paper [arxiv:2602.00180](https://arxiv.org/abs/2602.00180) §II-C: "the specification is the only artifact humans edit directly; code is entirely generated from the spec"). Implementa la disciplina sobre tres invariantes:

1. Humanos editan specs y aprueban PRs; agentes escriben el código.
2. Toda diff de código exige una diff de Spec correspondiente.
3. El **Auditor** + el hook `pre-commit-contract` + los tests autoría **QA** garantizan equivalencia Spec↔Código sin requerir regeneración pura.

**Fuera de alcance**:

- Frameworks de testing específicos (cada proyecto elige el suyo; la factory impone solo la estructura de carpetas y el coverage gate).
- MCPs (la factory base no trae conectores; los Agents de dominio los declaran).
- UI/dashboards. La factory vive en archivos, terminales y editores.

**Distribución prevista** (constitution §4): Fase actual **Opción B** — paquete `opftr` (unscoped) que scaffoldea `.claude/` en proyectos target. Fase futura **Opción D** — monorepo bajo org `@open-factory` con `@open-factory/core`, `@open-factory/cli` y `@open-factory/plugin`.

---

## 2. Modelo POA

POA (Programación Orientada a Agentes) modela las primitivas ejecutables como objetos con `name`, `permissions` y `objective` compartidos. Ver diagrama completo en [`diagrams/poa-class.md`](./diagrams/poa-class.md).

### 2.1 Primitivas

| Primitiva | Archivo | Naturaleza |
|---|---|---|
| `Agent` | `.claude/agents/<n>.md` (markdown + YAML frontmatter) | Subagent de Claude Code |
| `Skill` | `.claude/skills/<n>/SKILL.md` (+ scripts opcionales) | Capacidad invocable por prompt o `/comando` |
| `Command` | `.claude/commands/<n>.md` | Punto de entrada explícito `/comando` |
| `Hook` | `.claude/hooks/<n>.{sh,py}` + entrada en `settings.json` | Disparador determinístico por evento |
| `MCP` | `.mcp.json` o inline en Agent | Conector externo |

### 2.2 Artefactos del contrato

| Artefacto | Archivo | Rol |
|---|---|---|
| `Bootstrap` | (operacional, vía Planner) | Fase única que produce PRD + RFC + Workflow derivado |
| `PRD` | `.claude/specs/prd.md` | What/Why del proyecto — revisado y aprobado por el User en Gate 1 |
| `RFC` | `.claude/specs/rfc.md` | How + granularidad + descomposición en Tasks + declaración POA — revisado y aprobado por el User en Gate 2 |
| `Constitution` | `.claude/specs/constitution.md` | Principios no negociables |
| `Workflow` | `.claude/specs/workflow.md` | Granularidad + Tasks + declaración POA **(derivado del RFC)** — declarante soberano en tiempo de ejecución |
| `Task` | `.claude/specs/tasks/<id>-<slug>.md` | Unidad atómica con Spec embebida |
| `Spec` | (dentro del archivo de Task) | Contrato What/Why/How versionado (What/Why trazan al PRD; How traza al RFC) |
| `TestSuite` | `tests/<level>/<id>__*.test.*` | Derivada del Spec, escrita por QA |

### 2.3 Invariantes derivados

Enforzados por hooks o por Auditor (ver constitution §2 y poa-class.md §3):

1. **`Workflow ⇒ Components`** — toda primitiva POA debe estar declarada en `Workflow.declared*`. Validado por `pre-spec-validate`.
2. **`Spec ⇒ Code diff`** — cada path tocado debe estar en scope de alguna Spec activa. Validado por `pre-commit-contract`.
3. **`Spec ⇒ Tests`** — cada acceptance criterion debe tener test correspondiente. Validado por `pre-commit-contract` + Auditor.
4. **`POAObject.permissions = minimal`** — least privilege. Validado por `permissions-guard` al boot.
5. **`Spec.version monotónico`** — bump solo por Auditor, nunca por humano. Validado por `post-merge-bump`.

---

## 3. Niveles del contrato

```
L-1 Bootstrap     fase única que produce L0+L1 y declara agents/skills/hooks/commands del proyecto
L0  Constitution  principios no negociables
L1  Workflow      granularidad + Tasks + declaración POA
L2  Task          unidad atómica; 1 Task = 1 Spec
L3  Spec          contrato versionado What/Why/How
```

**Bootstrap** es operacional: ocurre una vez al inicio del proyecto y se re-corre cuando el alcance cambia significativamente. Lo ejecuta `planner` con dos gates de review explícita del User (UC-1):

- **Gate 1 — PRD** (`prd.md`): What/Why del proyecto. Sin PRD aprobado no se empieza el RFC.
- **Gate 2 — RFC** (`rfc.md`): How + granularidad + Task decomposition + declaración POA. Sin RFC aprobado no se deriva el Workflow ni se escribe nada a disco.

Bootstrap produce tres artefactos que se commitean juntos: `prd.md`, `rfc.md`, `workflow.md` (derivado del RFC) + `constitution.md` + skeletons de Tasks/Agents/Skills/Hooks/Commands.

**El Workflow es soberano**: ningún Agent/Skill/Command/Hook puede existir si no está declarado allí. `workflow.md` es derivado del RFC, pero es el declarante en tiempo de ejecución — los hooks y agentes leen `workflow.md`, no el RFC directamente. Para agregar un componente nuevo se corre UC-4 (`/agent-new` o equivalente) que es un mini-Bootstrap incremental.

---

## 4. Catálogo de agentes base

Layout por capa y color. Para cada agent, su Task de creación está en [`workflow.md`](./workflow.md) §3.

### 4.1 Bootstrap

#### `planner` — `orange`

- **objective**: conducir el Bootstrap en dos etapas (PRD → RFC), derivar `workflow.md` y coordinar la materialización por `specter`.
- **tools**: `Read`, `Agent(researcher)`, `Skill(draft-prd)`, `Skill(draft-rfc)`, `Skill(plan-workflow)`, `Skill(propose-agents)`, `Skill(research-topic)`.
- **permissionMode**: `plan` (no escribe; solo propone).
- **hooks**: `Stop` con prompt-hook que valida que la propuesta tenga la review del User antes de cerrar.
- **memory**: `project`.
- **description** (borrador): `Use proactively when the user asks to plan or design a project workflow, define what/why/how, decide task granularity, or enumerate which agents/skills/hooks/commands a project needs. Bootstrap-layer agent.`

#### Skills Bootstrap

| Skill | Task | Rol |
|---|---|---|
| `draft-prd` | 0030 | Conduce el interview de PRD (What/Why) y renderiza `prd.md`. Gate 1 de Bootstrap. |
| `draft-rfc` | 0031 | Conduce el interview técnico (How + granularidad + POA) y renderiza `rfc.md`. Gate 2 de Bootstrap. |
| `plan-workflow` | 0008 | Deriva `workflow.md` (tablas POA + task list + grafo) desde el RFC aprobado. |
| `propose-agents` | 0009 | Catálogo de agents/skills/hooks/commands con modelo/esfuerzo/permisos; invocado desde `draft-rfc` para §8. |
| `research-topic` | 0010 | Survey de dominio + stack; invocado desde `researcher` y por `planner` antes del PRD interview. |

### 4.2 Specify (`blue`)

#### `curator`

- **objective**: pulir What/Why/How de una Task individual (granularidad ya heredada del Workflow).
- **tools**: `Read`, `Skill(polish-idea)`.
- **permissionMode**: `plan`.
- **hooks**: `pre-spec-validate` (PreToolUse sobre Write de specs).
- **description**: `Use proactively when the user asks to polish, refine, or clarify the idea behind a task before turning it into a spec. Specify-layer agent.`

#### `specter`

- **objective**: materializar la Spec versionada en disco con el template estándar.
- **tools**: `Write(specs/**)`, `Skill(write-spec)`, `Read`.
- **permissionMode**: `acceptEdits` solo para `specs/`.
- **description**: `Use proactively when the user asks to write, materialize, or save a spec to disk. Specify-layer agent.`

### 4.3 Plan (`purple`)

#### `qa`

- **objective**: (a) validar testeabilidad/ambigüedad de la Spec; (b) **autoría del test suite** en 3 niveles (unit, integration, acceptance).
- **tools**: `Read`, `Write(tests/**)`, `Bash(<test-runner> --dry-run)`, `Skill(spec-lint)`, `Skill(author-tests)`.
- **permissionMode**: `acceptEdits` para `tests/`.
- **description**: `Use proactively when the user asks to validate a spec or write tests from a spec. Generates unit, integration, and acceptance tests as failing-first contracts. Plan-layer agent.`

### 4.4 Implement (`green`)

#### `coder`

- **objective**: implementar la Task hasta que la suite de QA pase.
- **tools**: `Edit`, `Write`, `Read`, `Bash`.
- **permissionMode**: `acceptEdits` solo para paths declarados en Spec.
- **hooks**: `PreToolUse(Edit|Write)` que verifica scope (delegado a `pre-commit-contract` en modo dry).
- **description**: `Use proactively when the user asks to implement, code, or build a task whose spec already exists. Implement-layer agent.`

#### `reviewer`

- **objective**: revisar diff contra Spec antes de pasar a Validate.
- **tools**: `Read`, `Grep`, `Bash(git diff *)`, `Bash(git log *)`.
- **permissionMode**: `plan` (read-only).
- **description**: `Use proactively when the user asks for code review, to check a diff against the spec, or to inspect recent changes. Implement-layer agent.`

### 4.5 Validate (`red`)

#### `tester`

- **objective**: ejecutar la suite y reportar resultados.
- **tools**: `Bash(<test-runner> *)`, `Read`.
- **description**: `Use proactively when the user asks to run tests, check the test suite status, or report failing tests. Validate-layer agent.`

#### `pr`

- **objective**: abrir Pull Request siguiendo plantilla.
- **tools**: `Bash(gh pr *)`, `Bash(git push *)`, `Read`.
- **description**: `Use proactively when the user asks to open a pull request, create a PR, or push for review. Validate-layer agent.`

#### `auditor`

- **objective**: validar equivalencia Spec↔Código en el PR; al merge, bumpear `Spec.version`.
- **tools**: `Read`, `Bash(git *)`, `Skill(verify-contract)`.
- **permissionMode**: `default`.
- **description**: `Use proactively when the user asks to audit a PR, verify spec-code equivalence, check contract drift, or bump a spec version. Validate-layer agent.`

### 4.6 Transversales

#### `researcher` — `cyan`

- **objective**: investigación de contexto de dominio/stack/código existente.
- **tools**: `Read`, `Grep`, `Glob`, `WebFetch`.
- **permissionMode**: `plan` (read-only).
- **memory**: `project`.
- **description**: `Use proactively when the user asks to research, investigate, or analyze a domain, codebase, or stack before planning or speccing. Transversal agent.`

#### `tl` — `pink`

- **objective**: orquestar agentes para correr los UCs; decisiones no críticas.
- **tools**: `Agent(*)`, `Read`.
- **permissionMode**: `default`.
- **memory**: `project`.
- **description**: `Use proactively when the user asks to orchestrate the full cycle of a task, coordinate multiple agents end-to-end, or run a workflow. Cross-cutting super-agent.`

> Detalles completos de cada agent (model, hooks por agent, memory scope, color exacto) se especifican en su Task individual del Workflow.

---

## 5. Convenciones de invocación por prompt

Recordatorio: Agents y Skills se eligen por matching natural del prompt con su campo `description`. Commands y Hooks NO participan del matching.

### 5.1 Reglas para `description`

1. **Caso de uso primero, luego trigger phrases, luego detalles.** El cap de 1536 chars trunca desde el final.
2. **Empezar con "Use proactively when ..."** para forzar delegación automática (patrón documentado en [subagents](https://code.claude.com/docs/en/sub-agents#understand-automatic-delegation)).
3. **Incluir frases textuales del User**, no parafrasearlas. Si el User suele decir "abrí el PR", la description debe contener "open a pull request" o "abrí el PR".
4. **Sufijo de capa**: cerrar con "Specify-layer agent" / "Plan-layer agent" / etc. ayuda al matching cuando dos agents tienen verbs similares.

### 5.2 Skills con efectos

Toda Skill que escribe archivos, llama APIs externas o muta estado debe declarar `disable-model-invocation: true`. Esto previene que Claude la dispare por su cuenta y la fuerza a invocarse vía `/comando` o desde un Agent que la preloadea.

Skills sin efectos (referenciales: `research-topic`, `polish-idea`) pueden ser auto-invocables.

### 5.3 Smoke-test obligatorio

Cada PR que toca un `description` debe re-correr el subset relevante de [`diagrams/invocation-fixtures.md`](./diagrams/invocation-fixtures.md). Si un fixture falla, ajustar la description hasta que pase. Anti-patrones detectados se loguean en la §6 del mismo archivo para evitar reincidencia.

---

## 6. Hooks normativos

Los 4 hooks de la factory base, con su contrato:

### 6.1 `permissions-guard`

- **Evento**: `SessionStart`.
- **Matcher**: ninguno (siempre dispara).
- **Input**: JSON con `session_id`, `cwd`.
- **Comportamiento**: recorre `.claude/agents/`, `.claude/skills/`, `.claude/commands/`, valida que cada componente declare `permissions` consistentes con su `objective`. Imprime warnings a stderr para violaciones (no bloquea).
- **Exit codes**: 0 siempre.

### 6.2 `pre-spec-validate`

- **Evento**: `PreToolUse`.
- **Matcher**: `Write` con `if: Write(.claude/specs/tasks/**)`.
- **Input**: JSON con `tool_input.file_path`, `tool_input.content`.
- **Comportamiento**: parsea el frontmatter/markdown destino, exige presencia de secciones `## What`, `## Why`, `## How` y bloque `acceptanceCriteria:` con ≥1 entrada. También exige que el componente esté declarado en `workflow.md` `Workflow.declared*` (si es un componente nuevo).
- **Exit codes**: 0 ok, 2 block con mensaje específico en stderr.

### 6.3 `pre-commit-contract`

- **Evento**: git pre-commit hook (no Claude Code event; vive en `.git/hooks/pre-commit` instalado vía Task 0003).
- **Input**: lee `git diff --cached --name-only`.
- **Comportamiento**: invoca `verify-contract` skill en modo `--check`. Mapea cada path tocado a Tasks activas vía Workflow; si algún path queda fuera de scope, bloquea. Si Spec tiene acceptance criteria sin test correspondiente en `tests/<level>/<id>__*.test.*`, bloquea.
- **Exit codes**: 0 ok, 2 block.

### 6.4 `post-merge-bump`

- **Evento**: git post-merge hook.
- **Input**: lee el último commit y los archivos cambiados.
- **Comportamiento**: detecta qué Tasks fueron afectadas, deduce el nivel de bump (patch/minor/major) según las reglas de §11, invoca `verify-contract` skill en modo `--bump` que actualiza el frontmatter de la Spec. Hace commit auto con mensaje `chore(spec): bump <task-id> to <version>`.
- **Exit codes**: 0 siempre (efecto secundario, no bloquea merge).

---

## 7. Testing en spec-as-source — 3 niveles obligatorios

QA materializa el contrato de cada Spec en una `TestSuite` con tres archivos por Task:

### 7.1 `unit/`

- Uno por cada **acceptance criterion atómico** de la Spec.
- Verifica funciones/clases aisladas con mocks de I/O.
- Path: `tests/unit/<task-id>__<slug>.test.*`.

### 7.2 `integration/`

- Uno por cada **interacción declarada en el Workflow** entre la Task y otra.
- Verifica contratos entre módulos sin mockear los I/O internos (sí mockear externos).
- Path: `tests/integration/<task-id>__<slug>.test.*`.

### 7.3 `acceptance/` (alias e2e)

- Uno por cada **escenario "What" del User** en la Spec.
- Verifica el flujo end-to-end: HTTP request real, click real en Playwright, comando real en CLI, etc.
- Path: `tests/acceptance/<task-id>__<slug>.test.*`.

### 7.4 Coverage gate

`verify-contract` skill rechaza el merge si:

- Alguna `Spec.acceptanceCriteria[i]` no tiene test `unit` correspondiente.
- Alguna interacción del Workflow no tiene test `integration` correspondiente.
- Algún escenario "What" de la Spec no tiene test `acceptance` correspondiente.

Frameworks de testing los elige cada proyecto (la factory no impone). La convención de paths es lo único impuesto, para que el mapeo automático funcione.

---

## 8. Diagramas de secuencia

Ver [`diagrams/sequences.md`](./diagrams/sequences.md) para los 4 casos de uso:

- **UC-1** — Bootstrap del proyecto (con review del User).
- **UC-2** — Implementar una Task (loop SDD por task).
- **UC-3** — Spec drift detectado (hook bloquea, se re-curate).
- **UC-4** — Crear agente de dominio (Bootstrap incremental).

---

## 9. Workflow per-dominio (playbooks)

La granularidad se decide por proyecto (constitution §2.3.9). Estos playbooks son sugerencias por dominio, no obligatorios.

### 9.1 ML — pipelines de entrenamiento

- **Granularidad típica**: 1 Task = 1 etapa del pipeline (data loading, preprocessing, model definition, training loop, evaluation, checkpointing).
- **Agents de dominio típicos**: `data-loader`, `trainer`, `evaluator`.
- **MCPs útiles**: MLflow, W&B, S3/GCS.
- **Tests**: unit con mocks de dataset; integration con dataset chico real; acceptance con run completo de pocos steps validando métricas mínimas.

### 9.2 Data — pipelines ETL

- **Granularidad típica**: 1 Task = 1 etapa (extract, transform, load por destino).
- **Agents de dominio típicos**: `extractor`, `transformer`, `loader`.
- **MCPs útiles**: BigQuery, Snowflake, dbt, Airbyte.
- **Tests**: unit por función de transform; integration con muestras representativas; acceptance con DAG completo.

### 9.3 Backend — endpoints o capas

- **Granularidad fina**: 1 Task = 1 endpoint (REST/gRPC).
- **Granularidad gruesa**: 1 Task = 1 capa de responsabilidad (controller, service, repository).
- **Agents de dominio típicos**: `endpoint-author`, `migration-author`.
- **MCPs útiles**: GitHub, Postgres, Stripe (si aplica).
- **Tests**: unit por handler/service; integration con DB real (testcontainers); acceptance con HTTP real.

### 9.4 Frontend — páginas o componentes

- **Granularidad fina**: 1 Task = 1 componente.
- **Granularidad gruesa**: 1 Task = 1 página o ruta.
- **Agents de dominio típicos**: `nextjs-page`, `component-author`.
- **MCPs útiles**: Figma, Playwright, Storybook.
- **Tests**: unit con React Testing Library; integration con Storybook play functions; acceptance con Playwright.

---

## 10. Reglas de extensión

Cómo agregar un Agent de dominio sin romper la factory (ver UC-4):

1. **Pasa por Planner** (`/agent-new <name>`). Curator no crea agents.
2. **Declarar en `Workflow.declaredAgents`**. Sin declaración, `pre-spec-validate` rechaza la creación del archivo.
3. **Reutilizar Skills base** cuando aplique. Si un `nextjs-page` puede usar las mismas Skills de git/diff que `coder`, lo hace por composición; no se duplica lógica.
4. **Solo agregar lo específico**. Lo único nuevo (ej. `tsx-conventions`, MCP Figma) se declara explícitamente.
5. **Validar con fixtures**. Antes de cerrar el PR del agent nuevo, ejecutar los smoke-tests de `invocation-fixtures.md` para confirmar que dispara con prompts naturales.
6. **Permisos mínimos**. Si el agent nuevo pide más de lo necesario, `permissions-guard` warnea al boot y Reviewer rechaza el PR.

**Anti-pattern**: un Agent de dominio que termina con más de 5 Skills propias probablemente está mal granularizado y debería partirse en 2.

---

## 11. Versionado del contrato

SemVer por Spec, bump dispara solo el Auditor al merge.

| Tipo | Cuándo |
|---|---|
| **Patch** (x.y.Z) | Clarificación de texto sin cambio de comportamiento; refactor del Coder que mantiene los acceptance criteria invariantes. |
| **Minor** (x.Y.0) | Nuevo acceptance criterion compatible con los anteriores; extensión del scope (paths nuevos cubiertos) sin romper consumidores. |
| **Major** (X.0.0) | Cambio incompatible en What/Why; eliminación o reescritura de acceptance criterion; cambio que rompe consumidores aguas abajo. |

### 11.1 Algoritmo de bump

`verify-contract --bump` corre al merge:

1. Compara `Spec` actual vs versión previa (`git show HEAD~1:./Spec.md`).
2. Diff de `acceptanceCriteria[]`:
   - Si solo se cambió texto → **patch**.
   - Si se agregaron criterios → **minor**.
   - Si se quitaron o cambió `What`/`Why` significativo → **major**.
3. Escribe el nuevo `version` en el frontmatter, commitea `chore(spec): bump <id> to <version>`.

### 11.2 Sin auto-merge

Si el Auditor detecta drift entre Spec y código durante el review del PR, **bloquea el merge**. El bump nunca ocurre antes del merge; nunca se delega a humanos.

---

## 12. Roadmap de implementación

Orden recomendado (ver [`workflow.md`](./workflow.md) §3 para detalle por Task):

1. **Foundation** (0001-0004) — settings + 4 hooks. Sin esto la factory no aplica nada.
2. **Transversales** (0005-0006) — researcher + tl. Sin tl no se puede orquestar; sin researcher no hay input para los demás.
3. **Bootstrap layer** (0007-0010, 0030-0031) — planner + 5 skills (`draft-prd`, `draft-rfc`, `plan-workflow`, `propose-agents`, `research-topic`). Habilita correr UC-1 con flujo PRD → RFC → Workflow derivado.
4. **Specify layer** (0011-0014) — curator + specter + 2 skills. Habilita correr el inicio de UC-2.
5. **Plan layer** (0015-0017) — qa + 2 skills. Sin qa no hay tests, sin tests Coder no puede empezar.
6. **Implement layer** (0018-0019) — coder + reviewer.
7. **Validate layer** (0020-0023) — tester + pr + auditor + verify-contract skill.
8. **Commands** (0024-0029) — entry points `/`.

Hitos:

- **Hito A**: Foundation + Transversales (0001-0006) listos a mano. **Recursión rota**: la factory ya puede empezar a usarse a sí misma desde acá.
- **Hito B**: Hito A + Bootstrap + Specify (hasta 0014, más 0030-0031). Se puede correr UC-1 completo con flujo PRD → RFC → Workflow derivado y crear Specs reales.
- **Hito C**: Hito B + Plan + Implement (hasta 0019). Se puede correr UC-2 hasta el código.
- **Hito D**: Hito C + Validate + Commands (hasta 0029). UC-1..UC-4 todos funcionales. Factory completa.

---

---

## 13. Capa de empaquetado (no POA)

La factory se distribuye como paquete npm `opftr` (unscoped, constitution §4, Opción B). El paquete vive en `packages/cli/` del monorepo y NO es un componente POA: es **infraestructura de distribución**, no se declara en `Workflow.declared*`. Sus reglas:

- **Dogfooding**: `.claude/` y `CLAUDE.md` en la raíz del repo SON el template. `packages/cli/scripts/sync-templates.mjs` copia ese contenido a `packages/cli/templates/` antes de cada build/publish. Hay un único source-of-truth editable.
- **Cero deps de runtime** en `opftr`. Solo Node builtins (`node:fs`, `node:path`, `node:util`). Esto minimiza la superficie de supply chain y hace `npx` instantáneo.
- **Versionado del paquete**: SemVer independiente de las Specs internas. El paquete versiona el snapshot del template; cada agent/skill nuevo sumado al `.claude/` raíz produce un minor del paquete.
- **Guarda anti self-bootstrap**: el comando `init` rechaza correr contra el propio repo del monorepo (detecta que el target contiene el `templates/`); el flag `--force` permite override consciente.
- **Comandos planeados**:
  - `init [dir]` — fase B v0.1 (implementado).
  - `upgrade [dir]` — fase B v0.2 (TODO): merge no-destructivo de un template más nuevo en una `.claude/` existente; backup automático.
  - `validate [dir]` — fase B v0.3 (TODO): corre `permissions-guard` + `pre-spec-validate` offline sin abrir Claude Code.
  - `agent new <name>` — fase B v0.4 (TODO): wrapper headless que invoca al Planner via Claude Code Agent SDK para UC-4.

Cuando lleguemos a la fase D, este package se renombra de `opftr` a `@open-factory/cli` (al registrar la org en npm) y se suman `@open-factory/core` (tipos + validadores reutilizables) y `@open-factory/plugin` (plugin nativo de Claude Code para lo que tolera vivir como plugin). La factory en `.claude/` no se ve afectada. `opftr` queda como alias de retrocompatibilidad si hace falta.

---

## Anexos

- [constitution.md](./constitution.md) — principios no negociables.
- [workflow.md](./workflow.md) — Tasks que construyen esta factory.
- [diagrams/poa-class.md](./diagrams/poa-class.md) — modelo de clases POA.
- [diagrams/sequences.md](./diagrams/sequences.md) — UC-1..UC-4.
- [diagrams/invocation-fixtures.md](./diagrams/invocation-fixtures.md) — smoke-test de matching.
- [`packages/cli/README.md`](../../packages/cli/README.md) — capa de empaquetado.
