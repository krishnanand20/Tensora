# Glossary

| Term | Meaning |
|---|---|
| **Admission** | The controller's decision to place a pending or queued workload onto free GPUs |
| **Allocation (billing)** | Time a GPU is reserved for a workload in a billable state, whatever its utilization (ADR-0006) |
| **CDI** | Containerized Data Importer: imports and clones VM disk images into PVCs (DataVolume, DataSource) |
| **Cell** | One region's own clusters, storage, gateway, and regional controller (ADR-0009) |
| **Cilium** | eBPF-based CNI providing networking, NetworkPolicy, kube-proxy replacement, and Gateway API |
| **DataSource** | A CDI object naming a "golden" disk that VM disks are cloned from (in `tensora-images`) |
| **DCGM** | NVIDIA Data Center GPU Manager. Its exporter publishes GPU metrics (utilization, XID, ECC, temperature). |
| **Fallback** | An opt-in substitute (another GPU class, CPU, replica, model, or region) used when the requested one is unavailable (ADR-0010) |
| **Gateway API** | The Kubernetes ingress API that succeeds Ingress: `Gateway`, `HTTPRoute`, and related types |
| **GPU class** | A catalog entry (`l40s-48gb`) mapping a sellable GPU type to cluster resource names |
| **GPU Operator** | NVIDIA operator that installs the driver, container toolkit, device plugins, vfio-manager, MIG manager, and DCGM |
| **Inference gateway** | Regional proxy in front of endpoints: API keys, token metering, rate limits, and request failover (ADR-0010) |
| **Intent** | What a customer asked for, recorded in Postgres. The controller makes the cluster match it. |
| **IOMMU** | CPU feature (Intel VT-d, AMD-Vi) that isolates device DMA; required for passthrough |
| **Isolation tier** | `trusted` (our team, runc allowed) or `untrusted` (customers, VMs or Kata only) |
| **Kata Containers** | Runs each pod inside a lightweight VM; gives containers a VM-grade boundary |
| **KServe** | Kubernetes model-serving layer. `InferenceService` is its main resource. RawDeployment mode avoids Knative. |
| **KubeVirt** | Runs VMs as Kubernetes objects (`VirtualMachine`), each inside a virt-launcher pod |
| **MetalLB** | Gives bare-metal clusters LoadBalancer IPs (L2/ARP or BGP) |
| **MIG** | Multi-Instance GPU: hardware partitioning of A100, H100, and later cards into isolated slices |
| **Park** | Release a workload's GPUs while keeping its disks (VM halted, notebook scaled to 0) |
| **Passthrough** | Giving a VM exclusive, direct access to a whole PCI device via VFIO |
| **Placement** | Where a workload actually runs: its region and `placed_class`, which can differ from the requested class when fallback is enabled |
| **PSA** | Pod Security Admission: per-namespace `privileged`, `baseline`, or `restricted` enforcement |
| **Reconcile** | One controller pass that makes cluster state match intent: admit, observe, park, remove |
| **Region** | One location with its own GPU clusters and storage. Every workload lives in exactly one (ADR-0009). |
| **safetensors** | A tensor file format that can't execute code when loaded, unlike pickle-based `.bin` and `.pt` files |
| **Server-side apply** | Kubernetes apply where the API server merges fields by manager; idempotent |
| **Tenant** | A customer account (organization). Its objects live in namespace `t-<slug>`. |
| **Time-slicing** | Several processes sharing one GPU with no memory isolation; never used across tenants |
| **vGPU** | NVIDIA's licensed mediated-device GPU sharing for VMs |
| **VFIO** | Linux framework that hands devices to userspace or VMs safely, using the IOMMU |
| **vLLM** | High-throughput LLM inference server with an OpenAI-compatible API |
| **Workload** | Something a customer launches: `vm`, `notebook`, or `endpoint` |
| **XID error** | NVIDIA driver error code. 79 means the GPU fell off the bus; 48 means a double-bit ECC error. |
