---
description: Survey a domain, stack, library, or codebase area and return a citation-rich summary. Use during Bootstrap research (called by Planner) and per-task research (called by Curator/Specter). Loaded by the researcher agent.
allowed-tools: Read, Grep, Glob, WebFetch
---

> Skeleton — full behavior is implemented in Task 0010.

# research-topic

Structured research template used by the `researcher` agent.

## Inputs

- A research question, in one or two sentences.
- The invocation context: `bootstrap`, `per-task`, or `ad-hoc`.

## Output

Use the canonical researcher output shape:

```markdown
## Findings
- <claim> (<path:line> or <URL>)
- ...

## Open questions
- <question> — <who can answer>

## Recommended next agent
<one sentence>
```

## Operating rules

- Every Findings bullet **must** end with a citation. No citation, no bullet.
- Prefer reading project files over fetching the web. Only fetch URLs the user provided or the project references.
- Stay within researcher's `maxTurns` budget (20). Return a tight summary rather than going deep.
- Auto-invocable (no `disable-model-invocation`) — has no side effects, only reads.
- After completing, append a one-line note to MEMORY.md with the topic, the strongest citation, and the takeaway.
