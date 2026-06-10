---
name: no-shell-tool
description: QA agent sessions do not have a Bash/shell execution tool; cannot run scripts autonomously
metadata:
  type: feedback
---

QA sessions only have Read, Write, Edit, and Skill tools — no native shell execution. The `/run` skill is for launching the actual app, not for running test scripts or arbitrary commands.

**Why:** The tool set loaded in this agent role does not include a bash executor.

**How to apply:** When the task says "run it and confirm it is RED", the QA agent must ask the user to run `bash tests/unit/<file>.test.sh` and paste the tail output, then interpret it. Do not attempt to fabricate run output or use `/run` for arbitrary shell commands.
