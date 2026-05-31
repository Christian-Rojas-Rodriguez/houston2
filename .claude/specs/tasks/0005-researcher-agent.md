---
task: "0005"
slug: researcher-agent
granularity: component
version: 0.1.0
status: done
declares:
  - type: agent
    name: researcher
    path: .claude/agents/researcher.md
scope:
  - .claude/agents/researcher.md
  - tests/unit/0005__researcher-agent.test.mjs
---

# Task 0005 — `researcher` agent

> The transversal read-only research agent. Invoked by Planner (Bootstrap layer) for domain/stack discovery and by Curator/Specter (Specify layer) for per-task context gathering.

## What

A Claude Code subagent named `researcher` defined at `.claude/agents/researcher.md` that:

- Reads files, searches the codebase, and fetches web docs to gather context.
- Returns concise, citation-rich summaries to the calling agent.
- Never modifies files. Never opens PRs. Never runs destructive commands.
- Accumulates project-scoped memory (`memory: project`) so insights persist across sessions.

## Why

Every other agent in the factory needs grounded context to operate well:

- Planner must understand domain + stack before proposing a Workflow.
- Curator must understand the task's surroundings before polishing What/Why/How.
- Specter must reference existing conventions when materializing a Spec.

Having one transversal `researcher` avoids duplicating exploration logic in each consumer and keeps the read-only invariant in a single, auditable place.

This is the simplest agent in the factory (no skills, no MCPs, no hooks of its own), so it doubles as the canonical example for new contributors.

## How

Single file: `.claude/agents/researcher.md` with YAML frontmatter and a markdown body that serves as the system prompt.

### Frontmatter contract

```yaml
---
name: researcher
description: <see acceptanceCriteria #2>
model: haiku
effort: medium
maxTurns: 20
permissionMode: plan
memory: project
color: cyan
tools: Read, Grep, Glob, WebFetch
---
```

### Config rationale

| Field | Value | Why |
|---|---|---|
| `model` | `haiku` | Read-heavy, summarization-oriented work. Cheapest+fastest tier that handles long-context retrieval well. |
| `effort` | `medium` | Default reasoning depth — exploration doesn't need deep chain-of-thought. |
| `maxTurns` | `20` | Caps agentic exploration. Most research tasks resolve in <10 turns; 20 leaves headroom. |
| `permissionMode` | `plan` | Read-only enforcement at the Claude Code permission layer. |
| `memory` | `project` | Insights about THIS project's codebase persist across sessions and travel with the repo. |
| `color` | `cyan` | Per workflow.md §2.1, transversal agents are cyan. |
| `tools` | `Read, Grep, Glob, WebFetch` | Minimal allowlist for read-only research. No `Edit`/`Write`/`Bash`. |

### Body (system prompt) contract

The markdown body MUST:

1. Open with a one-line identity ("You are a research specialist…").
2. Describe the three invocation contexts: Bootstrap research, per-Task research, ad-hoc User questions.
3. Enforce output shape: `## Findings` (bulleted, citation-rich) → `## Open questions` (if any) → `## Recommended next agent` (which agent should pick this up).
4. Forbid speculation: any claim must be backed by a file path or URL.
5. Reference memory: instruct the agent to consult `MEMORY.md` first and update it after notable findings.

## Acceptance criteria

1. File `.claude/agents/researcher.md` exists and parses as valid YAML frontmatter + markdown body.
2. `description` field contains the substring `"Use proactively when"` AND at least one of: `"research"`, `"investigate"`, `"analyze"`, `"explore"`. (Required for prompt-matching per SPEC §5.)
3. `tools` is exactly the allowlist `Read, Grep, Glob, WebFetch` — no extras, no missing.
4. `model` is `haiku`, `effort` is `medium`, `permissionMode` is `plan`, `memory` is `project`, `color` is `cyan`, `maxTurns` is `20`.
5. The markdown body (after frontmatter) is non-empty and contains the literal headings `## Findings`, `## Open questions`, `## Recommended next agent`.
6. The body contains the word `MEMORY.md` (forces the memory contract to be in the system prompt).
7. The body does not contain any of: `Edit`, `Write`, `Bash`, `npm`, `pnpm` — the agent should never reference write tools or package managers.

All acceptance criteria are verified by `tests/unit/0005__researcher-agent.test.mjs`. No integration or acceptance tests apply at this granularity (researcher has no inter-task interactions to exercise yet; that comes when Planner consumes it in Task 0007).

## Out of scope

- Skills for researcher (e.g. `research-topic` is Task 0010, separate).
- Web search integration beyond `WebFetch` (researcher relies on the User/calling agent providing URLs).
- Background research mode (foreground only for now).
