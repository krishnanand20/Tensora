---
name: verify
description: Run the lint and test commands for every component touched by the current change and report the real results. Use before saying any code change is done.
---

Follow [docs/playbooks/verify-changes.md](../../../docs/playbooks/verify-changes.md).

1. Find touched paths with `git status --porcelain` and `git diff --name-only HEAD`. Include untracked files.
2. Map the paths to commands using the playbook's table, and run every command that applies.
3. Report each command as pass or fail. For failures, show the relevant output.
4. If a tool is missing, say which one and how to install it. **Never report a skipped check as passing.**
5. If isolation-sensitive paths changed (see CLAUDE.md), suggest running the `tenancy-reviewer` subagent.
   If `deploy/` or `infra/` changed, suggest `platform-reviewer`.
