---
description: Author the failing-first test suite for a Task from its Spec. Generates unit tests per acceptance criterion, integration tests per declared interaction, and acceptance tests per user-facing scenario. Used by QA after spec-lint passes and before Coder starts.
disable-model-invocation: true
allowed-tools: Read, Write
---

> Skeleton — full behavior is implemented in Task 0017.

# author-tests

Generate the test files that encode a Spec's acceptanceCriteria as executable contracts.

## Inputs

- Path to a Spec file.
- The test runner already used by the project (detected by QA; defaults to `node --test` if absent).

## Output

For a Task `0042-feature-foo`, write up to three files (only those that apply):

- `tests/unit/0042__feature-foo.test.<ext>` — one test per atomic acceptance criterion.
- `tests/integration/0042__feature-foo.test.<ext>` — one test per declared interaction with another Task.
- `tests/acceptance/0042__feature-foo.test.<ext>` — one test per user-facing scenario in `## What`.

## Rules

- Tests are **failing on first run**. No skips. No `xtest`.
- Each test name **quotes the acceptance criterion verbatim** (or a faithful summary if length forbids).
- Tests have **no implementation logic** of the system under test in them — they only observe.
- Choose the simplest runner that already exists. Do not introduce a new dependency.
- Always end the suite with a final assertion that the Spec version matches the version the tests were authored against — this fails fast if Spec is bumped without re-authoring tests.

## Coverage gate

Before exiting, emit a coverage table mapping each acceptance criterion to its test name. If any criterion is uncovered, return a `block` and abort writes — better no tests than partial tests.

## Operating rules

- Never modify implementation files. Only write under `tests/**`.
- Never modify a Spec. If a Spec has untestable criteria, return control to `spec-lint` / Curator.
