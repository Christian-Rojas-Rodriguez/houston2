---
description: Catalog the agents, skills, hooks, commands, and MCPs needed for a project, with per-agent model/effort/maxTurns/permissionMode recommendations and minimum-privilege tool allowlists. Used by Planner via draft-rfc when populating RFC §8 (Declaración POA) during Bootstrap (UC-1), and during UC-4 (`/agent-new`).
disable-model-invocation: true
allowed-tools: Read
---

# propose-agents

Produce the per-agent, per-skill, per-hook, per-command configuration that Planner emits as part of a Workflow proposal.

## Inputs

- Granularity strategy chosen by Planner.
- Task list with scopes.

## Output

For each agent, a YAML block with:

```yaml
name: <kebab-case>
description: Use proactively when ...
model: <haiku|sonnet|opus>
effort: <low|medium|high>
maxTurns: <integer>
permissionMode: <plan|acceptEdits|default>
memory: <project|local|none>
color: <one of: red blue green yellow purple orange pink cyan>
tools: <comma-separated minimum allowlist>
```

For each skill, a YAML block with `description`, `disable-model-invocation`, and `allowed-tools`.

For each hook, the event, matcher, and the script path that would live at `.claude/hooks/<n>.{sh,py}`.

For each command, the slug, the use case it serves, and any positional arguments.

## Recommendation table (defaults)

| Role kind | model | effort | maxTurns | permissionMode |
|---|---|---|---|---|
| Read-heavy researcher | haiku | medium | 20 | plan |
| Orchestrator / TL | sonnet | high | 30 | default |
| Heavy-reasoning planner | opus | high | 40 | plan |
| Idea polisher | sonnet | medium | 15 | plan |
| Spec/template writer | sonnet | low | 10 | acceptEdits |
| QA (lint + author tests) | sonnet | high | 25 | acceptEdits |
| Coder | sonnet | medium | 50 | acceptEdits |
| Reviewer | sonnet | medium | 15 | plan |
| Test runner | haiku | low | 10 | default |
| PR opener | haiku | low | 5 | default |
| Auditor | opus | high | 30 | default |

## Operating rules

- Always start from the table above; only deviate when the Task scope justifies it.
- Permissions are an allowlist, never a denylist.
- Every agent gets a `description` that starts with `Use proactively when` and contains at least one trigger phrase the user would type.
