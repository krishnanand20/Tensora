# GPU allocation and sharing

## The options

| Method | Isolation | Cards | Licensing | Tensora policy |
|---|---|---|---|---|
| **Whole-GPU passthrough** (VFIO) | Strong: the VM owns the device | Almost any; consumer cards have caveats | None | **Default for VMs** |
| **Whole GPU to a container** | Good with Kata; weak with runc | Any | None | Default for notebooks and endpoints |
| **MIG** (Multi-Instance GPU) | Hardware-partitioned memory and SMs; a shared driver | A100, H100, H200, B200 (and some others) | None for containers. MIG-backed vGPU needs a vGPU licence. | Phase 5 (M4), for small endpoints, behind Kata |
| **vGPU** | Strong (mediated device per VM) | Datacenter cards | NVIDIA vGPU/AI Enterprise licence per GPU | Not planned. Revisit if fractional VMs are in demand. |
| **Time-slicing** | **None**: shared memory and no fault isolation | Any | None | **Never across tenants**, and only for internal dev |

## When GPUs are sold out

Customers may opt in to **placement fallback**: another GPU class, or CPU, after a wait they choose
([ADR-0010](../adr/0010-fallbacks-cpu-placement-and-inference-failover.md)). Fallback never means
sharing a GPU. Time-slicing stays forbidden across tenants even under capacity pressure.

## GPU Operator node modes

The GPU Operator decides **per node** how all of its GPUs are used, through a label:

```
nvidia.com/gpu.workload.config=container        # NVIDIA driver on host → nvidia.com/gpu for pods
nvidia.com/gpu.workload.config=vm-passthrough   # vfio-pci bound → nvidia.com/<DEVICE> for KubeVirt
```

Consequences:

- **One node can't serve VMs and containers at the same time.** With a single node in
  Phase 1, you test container mode (endpoint and burn-in) and passthrough mode (VM) one after the other.
  Relabeling rebinds every GPU, so drain GPU workloads first.
- From two nodes onward, dedicate nodes to a mode and let the catalog route each workload kind to the right one.
- Node capacity per GPU class is computed this way (`cluster/kube`): passthrough nodes are counted by `vm_device_name`,
  and container nodes by `container_resource` plus `node_selector`.

## Consumer GPUs (RTX 4090, 5090)

- Cheap per FLOP, with no MIG or vGPU. Passthrough generally works, but some boards have reset quirks, so test stop/start cycles.
- **Licensing:** NVIDIA's GeForce driver licence has historically excluded datacenter deployment.
  Read the current EULA and get legal advice before selling consumer-GPU compute. Until then, the catalog
  marks `rtx4090-24gb` as `internal_only`, so only `trusted` tenants can use it.
- Their thermals assume desktop airflow. Blower-style or server-validated variants are needed in racks.

## MIG notes (Phase 5)

- MIG profiles (for example `1g.10gb` and `3g.40gb` on A100 80GB) appear as separate resources
  (`nvidia.com/mig-1g.10gb`) with `mig.strategy: mixed`. Each profile becomes its own GPU class in the catalog.
- MIG gives memory and fault isolation, but tenants still share a host kernel unless they run in Kata.
- Repartitioning needs an idle GPU. Plan fixed layouts per node rather than layouts that change on demand.
