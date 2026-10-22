# CLAUDE.md

@AGENTS.md

## Claude Code specifics

- **Skills** (`.claude/skills/`): `/verify`, `/new-adr`, `/add-api-resource`, `/add-platform-component`.
  Each wraps a playbook in `docs/playbooks/` so Claude, Codex, and humans follow the same steps.
- **Subagents** (`.claude/agents/`):
  - `tenancy-reviewer` — run after any change to `controlplane/internal/{api,service,store,cluster}`, `deploy/`, or `ml/src/tensora_ml/validate.py`. Looks for cross-tenant leaks, isolation regressions, and billing-integrity bugs.
  - `platform-reviewer` — run after changes to `deploy/` or `infra/`. Checks pinning, secrets, ordering, and blast radius.
- **Permissions** (`.claude/settings.json`): secret files are denied; cluster-mutating commands
  require explicit approval. Personal overrides go in `.claude/settings.local.json` (gitignored).
- **Component rules** load from nested `CLAUDE.md` files, which import the nested `AGENTS.md`.
- Personal, uncommitted notes: `CLAUDE.local.md` (gitignored).
