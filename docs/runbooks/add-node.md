# Add a node

1. **Before racking:** firmware updates and BIOS settings as in [01, step 1](01-node-bootstrap.md#1-biosuefi).
2. **Install Ubuntu** as in 01, step 2.
3. **Inventory:** add the host to `infra/ansible/inventory/hosts.yml`:
   - under `k3s_servers` if you're growing to 3 servers for HA, otherwise under `k3s_agents`
   - set `gpu_workload_config` for its role (container or vm-passthrough)
   - add its WireGuard key to the vault
4. **Ansible**, limited to the new host:
   ```bash
   ansible-playbook site.yml --limit <new-host> --check --diff --ask-vault-pass
   ansible-playbook site.yml --limit <new-host> --ask-vault-pass -e iommu_allow_reboot=true
   ```
5. **Join check:** `kubectl get nodes` shows the node Ready, with GPU Operator pods running on it.
6. **Burn-in** on every GPU ([02, burn-in](02-gpu-passthrough.md#burn-in-required-before-offering-any-gpu)). A failing GPU means the node is not offered: cordon it and open an RMA.
7. **Capacity:** if the node has a new GPU model, add a GPU class to `images/catalog.yaml` and the device to `gpuPassthrough.devices`.
   The controller picks up capacity automatically. Verify that its next admission pass uses the new GPUs.
8. **Storage:** with Longhorn, raise `storage.replicas` once there are at least 3 nodes, and check that existing volumes rebuild. With Ceph, check that new OSDs joined and wait for `HEALTH_OK`.
9. **Watch the first week:** XID and ECC errors, temperatures under real load, and power draw against the circuit budget.

## Removing a node

1. `kubectl cordon <node>`, then stop or migrate tenant workloads. Tell the affected customers first.
2. `kubectl drain <node> --ignore-daemonsets --delete-emptydir-data`
3. Ceph: mark its OSDs out and wait for rebalance. Longhorn: evict its replicas.
4. `kubectl delete node <node>`, and remove the host from inventory.
