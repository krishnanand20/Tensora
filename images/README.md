# images

What customers can launch.

| Path | What |
|---|---|
| `catalog.yaml` | The launch catalog: GPU classes and images. The control plane loads it, and its tests validate it. |
| `containers/pytorch-notebook/` | JupyterLab with PyTorch and CUDA (catalog id `pytorch-notebook`) |
| `vm/ubuntu-24.04-cuda/` | VM base image as a CDI DataSource (catalog id `ubuntu-24.04-cuda`) |

Endpoint images aren't built here. The `vllm` catalog entry points to the `tensora-vllm`
ClusterServingRuntime in `deploy/charts/cluster-config`, which pins an upstream vLLM image.

## Adding an image

1. Build it: add a directory under `containers/` or `vm/`.
2. Pin the base image by digest for production builds. Scan it, for example with `trivy image`.
3. Add it to `catalog.yaml` with the right `kind` and `ref`.
4. Run `make test-cp`. The control plane tests load the catalog and fail on mistakes.

Rules for images in tenant workloads:

- Run as non-root (uid 1000).
- No baked-in credentials.
- No SSH server in container images.
- Expose the documented port only.
