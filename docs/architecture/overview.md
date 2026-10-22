# Architecture overview

Tensora is a global, multi-region GPU cloud. Customers launch three kinds of workload on GPUs we own,
in the region they choose:

| Kind | What the customer gets | Built on |
|---|---|---|
| **VM** | A full Linux VM with whole GPUs passed through, accessed over SSH | KubeVirt + VFIO |
| **Notebook** | JupyterLab with a GPU and a persistent work directory | StatefulSet in a Kata sandbox |
| **Endpoint** | An OpenAI-compatible API serving a safetensors model | KServe (RawDeployment) + vLLM |

We assemble proven open-source infrastructure and write the product layer: the control plane (Go),
the ML tooling (Python), and the dashboard (TypeScript).

## Design assumptions

These assumptions shape most decisions. **Confirm or correct them.** They are tracked in [roadmap.md](../roadmap.md#open-decisions).

| Assumption | Consequence | If it's wrong |
|---|---|---|
| Customers are **external and untrusted** | VMs or Kata for their code, strict network policy, metering-grade billing, abuse handling | If it's internal-only, a `trusted` tier with runc containers and even time-slicing becomes acceptable, and billing becomes showback |
| We start with **one GPU node** and grow to about 3–10 | k3s, Longhorn first, Ceph later; no multi-node training fabric yet | At 20+ nodes from day one, start with Ceph and consider full Kubernetes with HA from the beginning |
| **Worldwide customers; India is the first region** (decided 2026-10-23) | Regions are first-class ([ADR-0009](../adr/0009-regions-global-control-plane-regional-cells.md)); USD pricing; global tax, privacy, and export-control compliance | — |
| **Opt-in fallbacks** (decided 2026-10-23) | CPU nodes and CPU classes; placement fallback; a regional inference gateway with failover ([ADR-0010](../adr/0010-fallbacks-cpu-placement-and-inference-failover.md)) | — |
| The workload mix is VMs, notebooks, and LLM inference | Whole-GPU allocation; MIG later for small inference | If training-heavy, prioritize 8-GPU NVLink nodes and an InfiniBand or RoCE fabric |

## Layers and components

| # | Layer | Choice | Phase | Where |
|---|---|---|---|---|
| 1 | Hardware | Datacenter GPUs (L40S, A100, H100) in servers with BMC, NVMe, and 25 GbE; UPS | 1 | [hardware/](../hardware/bill-of-materials.md) |
| 2 | OS and virtualization | Ubuntu 24.04, KVM via KubeVirt, IOMMU/VFIO | 1 | `infra/ansible` (base, iommu) |
| 2 | GPU management | NVIDIA GPU Operator: driver, toolkit, device plugins, vfio-manager, MIG manager, DCGM | 1 | `deploy/platform/gpu-operator` |
| 3 | Orchestration | k3s with embedded etcd, KubeVirt + CDI ([ADR-0001](../adr/0001-kubernetes-kubevirt-gpu-operator.md), [ADR-0002](../adr/0002-k3s-single-node-first.md)) | 1 | `infra/ansible/roles/k3s`, `deploy/platform/kubevirt` |
| — | Regions | One global control plane, plus one independent cell per region ([ADR-0009](../adr/0009-regions-global-control-plane-regional-cells.md)) | 1 → 4 | [control-plane.md](control-plane.md#regions-and-fallback-placement) |
| 4 | Model serving | KServe RawDeployment + the pinned `tensora-vllm` runtime (`tensora-vllm-cpu` for CPU fallback); MLflow later | 1 / 3 | `deploy/platform/kserve`, `cluster-config` |
| 4 | Inference gateway | Regional proxy for API keys, token metering, rate limits, and per-request failover ([ADR-0010](../adr/0010-fallbacks-cpu-placement-and-inference-failover.md)) | 3 | not built yet |
| 5 | Storage | Longhorn and S3 (choice open) → Rook-Ceph (RBD, RGW, CephFS) ([ADR-0005](../adr/0005-storage-longhorn-then-ceph.md)) | 1 → 2 | [storage.md](storage.md) |
| 6 | Networking | Cilium (CNI, kube-proxy replacement, policy, Gateway API), MetalLB, cert-manager, WireGuard ([ADR-0007](../adr/0007-gateway-api-on-cilium.md)) | 1 | [networking.md](networking.md) |
| 7 | Product | Tensora API, controller, and metering (Go); dashboard (TS); ML tooling (Python); Keycloak ([ADR-0003](../adr/0003-language-split.md)) | 3 | `controlplane/`, `web/`, `ml/` |
| 7 | Queueing and placement | Tensora admission: quota, capacity, region, and opt-in fallback to another GPU class or CPU. Kueue later for batch jobs. | 3 / 4 | [control-plane.md](control-plane.md) |
| 8 | Observability | kube-prometheus-stack, DCGM exporter, Loki + Alloy | 2 | [observability.md](observability.md) |
| 8 | Isolation | VMs or Kata, default-deny networking, no tenant Kubernetes API access ([ADR-0004](../adr/0004-untrusted-code-in-vms-or-kata.md)) | 1–3 | [tenancy-and-isolation.md](tenancy-and-isolation.md) |

## System diagram

```mermaid
flowchart TB
  cust([Customer])
  subgraph Global["Global control plane (one)"]
    ggw[Gateway<br/>api. / app. / auth.]
    api[tensora api]
    web[dashboard]
    meter[tensora metering]
    kc[Keycloak]
    pg[(Postgres<br/>accounts · intent · events · usage)]
  end
  subgraph Cell["Regional cell (one per region, e.g. in-mum-1)"]
    rgw[Region gateway<br/>id.region.ep.domain]
    igw[Inference gateway<br/>keys · token metering · failover]
    ctrl[tensora controller<br/>TENSORA_REGION]
    s3[(Object store)]
    obs[Prometheus · Loki · Grafana]
    subgraph Tenant["t-&lt;tenant&gt; namespaces"]
      vm[VirtualMachine]
      nb[Notebook]
      ep[InferenceService<br/>GPU, or CPU fallback]
    end
    subgraph Nodes[Nodes]
      gpus[[GPU nodes: container or vm-passthrough]]
      cpus[[CPU nodes]]
    end
  end
  cust --> ggw --> api & web & kc
  cust -- workload traffic --> rgw --> igw --> ep
  api --> pg
  meter --> pg
  ctrl --> pg
  ctrl -- server-side apply --> Tenant
  ep -- storage initializer --> s3
  Tenant --> Nodes
  obs -. scrapes .- Nodes
```

Until the second region opens, the global plane and the first region's cell run in **the same
cluster**. They split when region two arrives ([ADR-0009](../adr/0009-regions-global-control-plane-regional-cells.md)).
The inference gateway, CPU nodes, and fallback placement are designed but not built yet ([ADR-0010](../adr/0010-fallbacks-cpu-placement-and-inference-failover.md)).

## Request flow: launching an endpoint

```mermaid
sequenceDiagram
  actor U as Customer
  participant A as API (global)
  participant DB as Postgres (global)
  participant C as Controller (region)
  participant K as Kubernetes (region)
  participant G as Inference gateway (region)
  U->>A: POST /v1/workloads {kind: endpoint, region, gpu_class, fallback: [cpu]}
  A->>DB: lock quota row, check GPUs held, insert workload (pending, requested_class) + event
  A-->>U: 201 {state: pending}
  loop every 5s
    C->>DB: pending/queued workloads in my region, GPUs allocated
    C->>K: capacity per class (GPU and CPU)
    alt requested class has room
      C->>K: apply tenant bundle + InferenceService on GPU
    else sold out past fallback_after (opted in)
      C->>K: apply tenant bundle + InferenceService on CPU (tensora-vllm-cpu)
    end
    C->>DB: pending → provisioning, placed_class recorded (+event)
    C->>K: observe InferenceService Ready?
    C->>DB: provisioning → running (+event); billing starts at the placed class
  end
  U->>A: GET /v1/workloads (dashboard polls)
  U->>G: POST /v1/chat/completions (API key)
  G-->>U: answer + X-Tensora-Served-By / X-Tensora-Fallback headers
```

Today's code runs this flow without `region`, fallback placement, or the inference gateway. Those
are designed but not built yet (see the [roadmap task breakdown](../roadmap.md#task-breakdown-regions-and-fallback-placement)).

## Deliberately out of scope (for now)

- Multi-node distributed training (InfiniBand/RoCE, NCCL tuning, gang scheduling)
- Customer access to the Kubernetes API, and bring-your-own container images
- Spot or preemptible capacity, and reservations
- Workloads that span regions or move between them while running (each workload lives in exactly one region)
- Scale-to-zero endpoints (this needs Knative serverless mode)
