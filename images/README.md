# images

What users can launch.

| Path | What |
|---|---|
| `catalog.yaml` | The launch catalog: GPU classes and images. The control plane loads it, and its tests validate it. |
| `containers/pytorch-notebook/` | JupyterLab with PyTorch and CUDA (catalog id `pytorch-notebook`), for Linux |
| `vm/ubuntu-24.04-cuda/` | Datacenter VM base image, as a CDI DataSource (catalog id `ubuntu-24.04-cuda`) |

Endpoint images aren't built here. In a datacenter, the `vllm` catalog entry points to the
`tensora-vllm` ClusterServingRuntime in `deploy/charts/cluster-config`, which pins an upstream vLLM image.

## Planned catalog changes (ADR-0009, 0010, 0013)

- A `regions:` list. Solo has one region, `local`.
- Each class gains an `accelerator` (`nvidia`, `apple`, `amd`, `cpu`) and the `regions` where it's offered.
- **Apple Silicon classes**, identified by chip family and unified memory (for example `apple-m4-max-128gb`), and **CPU classes** (for example `cpu-16c-64g`).
- Images list a runtime per platform:
  - endpoints: vLLM (NVIDIA), MLX (Apple), llama.cpp (any, including CPU)
  - notebooks: a CUDA container on Linux, and a uv-managed environment with PyTorch (MPS) and MLX on Macs
  - VMs: qcow2 images (libvirt, KubeVirt) and macOS guest images (Virtualization framework)

The catalog loader rejects unknown fields today, so these arrive together with the code change.

## Adding an image

1. Build it: add a directory under `containers/` or `vm/`.
2. Pin the base image by digest for releases. Scan it, for example with `trivy image`.
3. Add it to `catalog.yaml` with the right `kind` and `ref`.
4. Run `make test-cp`. The control plane tests load the catalog and fail on mistakes.

Rules for images in user workloads:

- Run as non-root (uid 1000).
- No baked-in credentials.
- No SSH server in container images.
- Expose the documented port only.
- Free to use and redistribute: no paid or licence-key software (rule 12).
