# infra

Takes a freshly installed Ubuntu 24.04 GPU server to a k3s node that is ready for `deploy/`.

| Role | Does |
|---|---|
| `base` | Installs packages (including the iSCSI and NFS clients Longhorn needs), disables swap, sets sysctls and kernel modules, hardens SSH, and enables unattended security upgrades |
| `iommu` | Turns on IOMMU (`intel_iommu=on`/`amd_iommu=on`, `iommu=pt`), loads the VFIO modules, and blacklists nouveau. It does **not** bind GPUs to vfio-pci; the GPU Operator does that per node. |
| `wireguard` | Sets up the admin VPN interface (`wg0`). The Kubernetes API and SSH are meant to be reachable only over it, but **no firewall rule enforces that yet** (TODO in the roadmap, Phase 2). |
| `k3s` | Installs k3s with embedded etcd and with flannel, network policy, kube-proxy, traefik, and servicelb disabled, plus secrets encryption |

**Regions:** keep one inventory per region (each region is its own cluster, ADR-0009).
**CPU nodes** for CPU instances, fallback (ADR-0010), and the control plane need only `base`, `wireguard`, and `k3s`.
A `cpu_nodes` inventory group is planned for them. All hosts run on UTC.

## Use

```bash
cd infra/ansible
cp inventory/hosts.example.yml inventory/hosts.yml          # real IPs (gitignored)
cp group_vars/all/vault.yml.example group_vars/all/vault.yml
ansible-vault encrypt group_vars/all/vault.yml
ansible-playbook -i inventory/hosts.yml site.yml --check --diff   # dry run
ansible-playbook -i inventory/hosts.yml site.yml -e iommu_allow_reboot=true
```

Then check the node with the scripts and follow [docs/runbooks/01-node-bootstrap.md](../docs/runbooks/01-node-bootstrap.md):

```bash
sudo ./scripts/check-iommu.sh
./scripts/gpu-inventory.sh
```
