---
name: commit
description: Handles git commit operations. Use when user explicitly asks to commit code changes.
---

## Rules

### Principles
- **Only commit when the user literally says "commit"** (or gives an unambiguous commit instruction like "提交" / "commit this"). Approving a plan, choosing a value, or saying "ok / 就这样 / go ahead / do it" about a *code change* is NOT commit permission — apply the edit, then stop and wait for the explicit word.
- Never auto-commit.
- When in doubt, make the edit and ask before committing.
- When user says "commit all", stage and commit all modified files together in one commit.
- Do NOT push after commit. Leave that to the user.

### Requirements

#### title
- title of the commit MUST Use English
- Follow GitHub conventional commit prefixes:
  - `feat:`, `fix:`, `docs:`, `style:`, `refactor:`, `perf:`, `test:`, `chore:`, etc.


#### message body
- Mix Chinese and English
- Technical terms in English (e.g., `PR`, `branch`, `commit`, `merge`)

