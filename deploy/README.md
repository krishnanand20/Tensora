# deploy (Datacenter profile)

Everything that runs on a Kubernetes cluster, as code. This is the **Datacenter profile**
([ADR-0012](../docs/adr/0012-deployment-profiles.md)). For one Mac or Linux machine, or a small Lab, see
[docs/runbooks/00-quickstart.md](../docs/runbooks/00-quickstart.md) instead; nothing here is needed.

Before deploying, bootstrap each server with `infra/` (Ubuntu, IOMMU, and k3s without flannel,
kube-proxy, traefik, or servicelb).

## Tools

`helm` (v3.14+ or v4), `helmfile`, `kustomize`, `kubectl`, `kubeconform`, and `virtctl` for VMs.

## Install (humans only)

```bash
cd deploy
cp environments/single-node.yaml environments/mylab.yaml   # edit IPs, GPUs, and domain, then add it to helmfile
helmfile -e single-node template | less                     # review the render
helmfile -e single-node apply                               # first run takes 15–30 minutes
```

The steps are walked through in [docs/runbooks/](../docs/runbooks/README.md) (01–03), including how to
label nodes for container or passthrough mode.

## Regions and environments

Each region is its own cluster with its own environment file. The `single-node` environment is a
one-region deployment, and its cluster also hosts the control plane. For more regions, add
`environments/<region-id>.yaml` per cluster, and run the control plane once (ADR-0009).

Planned additions (ADR-0010):

- CPU worker nodes
- the `tensora-vllm-cpu` ClusterServingRuntime
- the regional inference gateway
- `TENSORA_REGION` passed to each region's controller

## Secrets created out of band

None of these live in git. Create them before the first apply:

| Secret | Namespace | Keys | Used by |
|---|---|---|---|
| `tensora-db` | tensora-system | `DATABASE_URL` | tensora chart |
| `grafana-admin` | observability | `admin-user`, `admin-password` | kube-prometheus-stack |
| Keycloak DB credentials | keycloak | see platform/keycloak/README.md | Keycloak |
| Object store credentials | tensora-models | depends on choice | model ingestion and KServe |

## Versions

The pins in `helmfile.yaml.gotmpl` were set when the repo was scaffolded (2026-10) **and have not
been verified against current upstream releases.** Before the first install, check each one and
record what you verified here:

| Component | Pinned | Verified on |
|---|---|---|
| Gateway API CRDs | v1.3.0 | — |
| Cilium | 1.18.0 | — |
| MetalLB | 0.15.2 | — |
| cert-manager | v1.18.2 | — |
| NVIDIA GPU Operator | v25.3.1 | — |
| KubeVirt / CDI | v1.6.0 / v1.62.0 | — |
| Longhorn | 1.9.1 | — |
| Rook-Ceph | v1.17.7 | — |
| KServe | v0.15.2 | — |
| Keycloak Operator | 26.3.0 | — |
| kube-prometheus-stack | 75.15.1 | — |
| Loki / Alloy | 6.35.0 / 1.2.0 | — |
| vLLM image | v0.10.0 | — |

Check upgrade compatibility in pairs: Cilium with Gateway API, the GPU Operator with the driver
and kernel, KubeVirt with CDI, and KServe with cert-manager.
