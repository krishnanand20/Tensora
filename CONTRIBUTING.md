# Contributing

## Setup

```bash
make setup        # Go modules, uv sync for ml/, npm install for web/, pre-commit hooks
make dev          # local all-in-one stack (no GPU or cluster required)
make lint test    # what CI runs
```

## Workflow

1. Branch from `main`: `feat/<short-name>`, `fix/<short-name>`, `docs/<short-name>`.
2. Keep each PR to one concern. Infra changes (`deploy/`, `infra/`) go in separate PRs from application code.
3. Use [Conventional Commits](https://www.conventionalcommits.org/): `feat(api): add model resource`.
4. Fill in the PR template, including the **tenant isolation** and **billing impact** sections.
5. CI must pass. At least one human reviews every PR, including those written by an agent.

## Decisions

Anything that changes an architecture rule in [AGENTS.md](AGENTS.md), adds a platform
component, or replaces one needs an ADR. See [docs/playbooks/write-an-adr.md](docs/playbooks/write-an-adr.md).

## Changes that need extra care

Changes to these paths need a reviewer who knows the isolation model:

- `controlplane/internal/{api,service,store,cluster}`: tenant scoping, lifecycle, and manifests
- `controlplane/internal/metering`: billing correctness
- `ml/src/tensora_ml/validate.py`: the rule that tenant models must be safetensors with no remote code
- `deploy/charts/cluster-config`, plus anything touching NetworkPolicy, RBAC, or RuntimeClass

## AI agents

Agents follow [AGENTS.md](AGENTS.md). Review their changes as you would any contributor's,
with extra attention to the paths listed above.
