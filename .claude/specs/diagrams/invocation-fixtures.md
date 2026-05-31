# Invocation fixtures — smoke-test de matching prompt → agent/skill

> Claude Code dispara un Agent o Skill **comparando el prompt del User con el campo `description`**. Estos fixtures son el smoke-test manual: cada vez que se escribe o modifica un Agent/Skill, ejecutar los prompts de abajo y verificar que dispare el target esperado. Si no dispara, ajustar `description`.

## 1. Cómo se usa este archivo

1. Después de crear/editar un Agent o Skill, abrir Claude Code en el repo.
2. Para cada fila relevante de las tablas de abajo, tipear el "Prompt típico" tal cual.
3. Confirmar en el transcript de Claude Code que el "Target esperado" se haya disparado (vía Agent tool o vía Skill tool).
4. Si no disparó, revisar el `description` del target. Recordar:
   - Empezar con el caso de uso, luego trigger phrases, luego detalles (truncamiento a 1536 chars).
   - Incluir frases textuales del User cuando sea posible.
   - Para forzar delegación automática, usar la frase "Use proactively when ...".

## 2. Agents

| Prompt típico del User | Target esperado | Razón / trigger phrase clave |
|---|---|---|
| `Investigá qué stack usan los otros pipelines de ML del equipo` | `researcher` | "investigá", "qué stack" |
| `Hacé un análisis del dominio antes de planificar` | `researcher` | "análisis del dominio" |
| `Definí el workflow del proyecto y qué tasks necesitamos` | `planner` | "definí el workflow", "qué tasks" |
| `Diseñá cómo descomponemos esto en tasks` | `planner` | "diseñá", "descomponemos en tasks" |
| `Necesito agregar un agent nuevo para frontend` | `planner` | "agregar un agent nuevo" (UC-4 → Planner) |
| `Pulí la idea de la task 0003 antes de escribirla como spec` | `curator` | "pulí la idea", "antes de escribirla como spec" |
| `Aclará el what/why/how de esta task` | `curator` | "what/why/how" |
| `Materializá la spec de la task 0003 en disco` | `specter` | "materializá la spec" |
| `Escribí la spec de la task 0003 a archivo` | `specter` | "escribí la spec a archivo" |
| `Validá que la spec 0003 sea testeable y escribí los tests` | `qa` | "validá que la spec sea testeable", "escribí los tests" |
| `Generá la suite de tests unit/integration/e2e desde la spec 0003` | `qa` | "generá la suite de tests" |
| `Implementá la task 0003 hasta que pasen los tests` | `coder` | "implementá la task" |
| `Hacé el código de la task 0003` | `coder` | "hacé el código" |
| `Revisá el diff contra la spec 0003` | `reviewer` | "revisá el diff", "contra la spec" |
| `Code review de los últimos cambios` | `reviewer` | "code review" |
| `Corré los tests` | `tester` | "corré los tests" |
| `Reportá el estado del test suite` | `tester` | "reportá el estado del test suite" |
| `Abrí el PR para la task 0003` | `pr` | "abrí el PR" |
| `Creá el pull request con el template del repo` | `pr` | "creá el pull request" |
| `Auditá que el código coincida con la spec antes de mergear` | `auditor` | "auditá que el código coincida con la spec" |
| `Verificá el contrato spec-código del PR` | `auditor` | "verificá el contrato spec-código" |
| `Orquestá el ciclo completo de la task 0003 de punta a punta` | `tl` | "orquestá el ciclo completo" |
| `Coordiná los agentes para implementar la task` | `tl` | "coordiná los agentes" |

## 3. Skills (auto-invocables — sin `disable-model-invocation`)

| Prompt típico | Target esperado | Notas |
|---|---|---|
| `Investigá el dominio de pipelines de entrenamiento` | Skill `research-topic` (a través de `researcher`) | Se preloadea en `researcher`; el matching es del agent, la skill sale en su prompt |
| `Pulí esta idea con what/why/how` | Skill `polish-idea` (a través de `curator`) | Igual que arriba |

> Las skills con efectos (`write-spec`, `author-tests`, `verify-contract`, `plan-workflow`, `propose-agents`, `spec-lint`) **NO se invocan por prompt natural**. Llevan `disable-model-invocation: true` y se disparan vía `/comando` o desde el agent que las preloadea. No aparecen en este fixture.

## 4. Commands (deterministas — sin matching)

Los commands se invocan con `/comando` y no participan del matching. Listados acá solo para completitud:

| Command | Trigger | Dispara |
|---|---|---|
| `/factory-init` | manual | UC-1 (Bootstrap) |
| `/workflow-review` | manual | re-abre Workflow para ajustar granularidad |
| `/task-run <id>` | manual | UC-2 (loop SDD por task) |
| `/agent-new <name>` | manual | UC-4 (Bootstrap incremental de agent) |
| `/spec-new <id>` | manual | crea esqueleto de Spec |
| `/spec-bump <id> <patch\|minor\|major>` | manual / Auditor | bumpea versión de la Spec |

## 5. Hooks (eventos — sin matching de prompt)

Los hooks corren por evento de lifecycle de Claude Code o por evento de git. Solo listados acá para que el smoke-test sea completo; su validación es por ejecución del evento, no por prompt:

| Hook | Evento | Validación |
|---|---|---|
| `pre-spec-validate` | `PreToolUse(Write)` sobre `.claude/specs/tasks/**` | Intentar crear una spec sin What/Why/How: debe bloquear exit 2 |
| `pre-commit-contract` | git pre-commit | Hacer un commit con código que toca path fuera de scope: debe bloquear |
| `post-merge-bump` | git post-merge | Mergear PR de Task: la spec debe quedar con version++ |
| `permissions-guard` | `SessionStart` | Boot con un agent que pida tools fuera de su scope: debe warnings en stderr |

## 6. Anti-patrones detectados (mantener actualizado)

> Cada vez que un fixture falle por mal matching, anotar acá el ajuste para que sirva de aprendizaje.

- *(vacío — agregar a medida que aparezcan)*
