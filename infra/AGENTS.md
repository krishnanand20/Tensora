# AGENTS.md: infra/ (bare-metal bootstrap)

These rules add to the root [AGENTS.md](../AGENTS.md); read that first.

## Layout

```
ansible/site.yml               entry point: base → iommu → wireguard → k3s
ansible/inventory/             hosts.example.yml is committed; hosts.yml (real IPs) is gitignored
ansible/group_vars/all/        main.yml (non-secret); vault.yml (ansible-vault encrypted, gitignored)
ansible/roles/<role>/          tasks/, handlers/, templates/, defaults/
scripts/                       read-only diagnostics to run on a node (IOMMU groups, GPU inventory)
```

## Rules

- **Never run playbooks against real hosts.** Agents may run `ansible-lint`, `ansible-playbook --syntax-check`, and, if asked, `--check --diff`. Applying changes is for humans.
- **Idempotent tasks only.** Every task must be safe to re-run. Use `creates:`, `changed_when:`, or proper modules rather than bare `shell`.
- **Reboots are opt-in.** Kernel cmdline changes notify a handler that reboots only if `iommu_allow_reboot: true`.
- **Secrets** (k3s token, WireGuard private keys) live only in `group_vars/all/vault.yml`, encrypted with `ansible-vault`. Reference them as `vault_*` variables.
- Use fully qualified module names (`ansible.builtin.apt`, not `apt`).
- **Scripts** in `scripts/` are diagnostics only. Read-only, `set -euo pipefail`, and they must pass shellcheck.
- Keep k3s flags in sync with deploy/: Cilium needs flannel, network policy, and kube-proxy disabled, and MetalLB needs servicelb disabled.

## Commands

```bash
cd infra/ansible
uvx --from ansible-core ansible-playbook --syntax-check -i inventory/hosts.example.yml site.yml
uvx ansible-lint
shellcheck ../scripts/*.sh
```
